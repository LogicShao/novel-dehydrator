package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/db"
	"github.com/LogicShao/novel-dehydrator/internal/logger"
	"github.com/LogicShao/novel-dehydrator/internal/models"
	"github.com/LogicShao/novel-dehydrator/internal/services/parser"
	"github.com/LogicShao/novel-dehydrator/internal/storage"
)

const listBooksSQL = `SELECT b.id, b.title, b.author, b.source_format, b.total_chapters,
	   b.has_volumes, b.parse_status, b.parse_error, b.created_at,
	   (SELECT COUNT(*) FROM chapters c WHERE c.book_id=b.id AND c.dehydrate_status='done') as dehydrated_count
FROM books b`

var allowedBookExtensions = map[string]struct{}{
	"epub": {},
	"txt":  {},
}

type BooksHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewBooksHandler(pool *pgxpool.Pool, cfg *config.Config) *BooksHandler {
	return &BooksHandler{pool: pool, cfg: cfg}
}

func (h *BooksHandler) HandleListBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Build dynamic ORDER BY from query params
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	orderClause := buildOrderClause(sortBy, order)

	query := listBooksSQL

	// Optional folder_id filter for Task 42
	folderIDStr := r.URL.Query().Get("folder_id")
	var args []any
	if folderIDStr != "" {
		if folderID, err := strconv.ParseInt(folderIDStr, 10, 64); err == nil {
			query += " INNER JOIN book_folders bf ON b.id = bf.book_id WHERE bf.folder_id = $1"
			args = append(args, folderID)
		}
	}

	query += " " + orderClause

	var rows pgx.Rows
	var err error
	if len(args) > 0 {
		rows, err = db.Query(ctx, h.pool, query, args...)
	} else {
		rows, err = db.Query(ctx, h.pool, query)
	}
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}
	defer rows.Close()

	books := make([]models.Book, 0)
	for rows.Next() {
		book, scanErr := scanBook(rows)
		if scanErr != nil {
			logger.WriteError(w, logger.InternalError(scanErr.Error()))
			return
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, books)
}

func (h *BooksHandler) HandleUploadBooks(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		logger.WriteError(w, logger.BadRequest(err.Error()))
		return
	}

	results := make([]map[string]any, 0)
	files := r.MultipartForm.File["files"]
	for _, header := range files {
		result := map[string]any{"filename": header.Filename}
		ext := fileExtension(header.Filename)
		if _, ok := allowedBookExtensions[ext]; !ok {
			result["error"] = fmt.Sprintf("不支持的文件格式 .%s", ext)
			results = append(results, result)
			continue
		}

		bookID, err := h.insertPendingBook(r.Context(), header.Filename, ext)
		if err != nil {
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		savePath := filepath.Join(h.cfg.DataDir, "uploads", fmt.Sprintf("%d.%s", bookID, ext))
		if err := saveUploadedFile(header, savePath); err != nil {
			if markErr := markBookFailed(r.Context(), h.pool, bookID, err); markErr != nil {
				log.Printf("markBookFailed: %v", markErr)
			}
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		if _, err := db.Exec(r.Context(), h.pool, "UPDATE books SET source_path=$1, parse_status='parsing', parse_error=NULL, updated_at=NOW() WHERE id=$2", savePath, bookID); err != nil {
			if markErr := markBookFailed(r.Context(), h.pool, bookID, err); markErr != nil {
				log.Printf("markBookFailed: %v", markErr)
			}
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		go parseBook(h.pool, h.cfg, bookID, savePath, ext)

		result["book_id"] = bookID
		result["parse_status"] = "parsing"
		results = append(results, result)
	}

	logger.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (h *BooksHandler) HandleGetBook(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathBookID(r)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效书籍ID"))
		return
	}

	row := db.QueryRow(r.Context(), h.pool, listBooksSQL+" WHERE b.id=$1", bookID)
	book, err := scanBook(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.WriteError(w, logger.NotFound("书籍不存在"))
			return
		}
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, book)
}

func (h *BooksHandler) HandleDeleteBook(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathBookID(r)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效书籍ID"))
		return
	}

	ctx := r.Context()
	var sourcePath string
	if err := db.QueryRow(ctx, h.pool, "SELECT source_path FROM books WHERE id=$1", bookID).Scan(&sourcePath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.WriteError(w, logger.NotFound("书籍不存在"))
			return
		}
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	if sourcePath != "" {
		if err := os.Remove(sourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.WriteError(w, logger.InternalError(err.Error()))
			return
		}
	}

	if err := storage.DeleteBookFiles(h.cfg.DataDir, bookID); err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	if _, err := db.Exec(ctx, h.pool, "DELETE FROM books WHERE id=$1", bookID); err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type BatchDeleteRequest struct {
	BookIDs []int64 `json:"book_ids"`
}

type BatchDeleteResult struct {
	Deleted []int64            `json:"deleted"`
	Failed  []BatchDeleteError `json:"failed"`
}

type BatchDeleteError struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
}

func (h *BooksHandler) HandleBatchDeleteBooks(w http.ResponseWriter, r *http.Request) {
	var req BatchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WriteError(w, logger.BadRequest("请求体格式错误"))
		return
	}

	if len(req.BookIDs) == 0 {
		logger.WriteError(w, logger.BadRequest("book_ids 不能为空"))
		return
	}
	if len(req.BookIDs) > 100 {
		logger.WriteError(w, logger.BadRequest("单次最多删除100本"))
		return
	}

	ctx := r.Context()
	seen := make(map[int64]struct{}, len(req.BookIDs))
	deletedIDs := make([]int64, 0, len(req.BookIDs))
	failedItems := make([]BatchDeleteError, 0)

	for _, bookID := range req.BookIDs {
		if _, ok := seen[bookID]; ok {
			continue
		}
		seen[bookID] = struct{}{}

		var sourcePath string
		if err := db.QueryRow(ctx, h.pool, "SELECT source_path FROM books WHERE id=$1", bookID).Scan(&sourcePath); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			failedItems = append(failedItems, BatchDeleteError{ID: bookID, Error: err.Error()})
			continue
		}

		if sourcePath != "" {
			if err := os.Remove(sourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failedItems = append(failedItems, BatchDeleteError{ID: bookID, Error: err.Error()})
				continue
			}
		}

		if err := storage.DeleteBookFiles(h.cfg.DataDir, bookID); err != nil {
			failedItems = append(failedItems, BatchDeleteError{ID: bookID, Error: err.Error()})
			continue
		}

		deletedIDs = append(deletedIDs, bookID)
	}

	if len(deletedIDs) > 0 {
		if _, err := db.Exec(ctx, h.pool, "DELETE FROM books WHERE id = ANY($1)", deletedIDs); err != nil {
			logger.WriteError(w, logger.InternalError(err.Error()))
			return
		}
	}

	logger.WriteJSON(w, http.StatusOK, BatchDeleteResult{Deleted: deletedIDs, Failed: failedItems})
}

func parseBook(pool *pgxpool.Pool, cfg *config.Config, bookID int64, filePath, ext string) {
	ctx := context.Background()

	parsed, err := parseBookFile(filePath, ext)
	if err != nil {
		if markErr := markBookFailed(ctx, pool, bookID, err); markErr != nil {
			log.Printf("markBookFailed: %v", markErr)
		}
		return
	}

	volumes, chapters := parser.DetectStructure(parsed.Chapters, parsed.HasNestedTOC)
	err = db.WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE books SET title=$1, author=$2, updated_at=NOW() WHERE id=$3", parsed.Title, parsed.Author, bookID); err != nil {
			return err
		}

		volumeSeqToID := make(map[int]int64, len(volumes))
		for _, volume := range volumes {
			var volumeID int64
			if err := tx.QueryRow(ctx,
				"INSERT INTO volumes (book_id, title, seq, detect_source) VALUES ($1, $2, $3, $4) RETURNING id",
				bookID, volume.Title, volume.Seq, volume.DetectSource,
			).Scan(&volumeID); err != nil {
				return err
			}
			volumeSeqToID[volume.Seq] = volumeID
		}

		for seq, chapter := range chapters {
			var volumeID *int64
			if chapter.VolumeSeq > 0 {
				if id, ok := volumeSeqToID[chapter.VolumeSeq]; ok {
					volumeID = &id
				}
			}

			var chapterID int64
			if err := tx.QueryRow(ctx,
				"INSERT INTO chapters (book_id, volume_id, title, seq, raw_path, raw_char_count) VALUES ($1, $2, $3, $4, '', $5) RETURNING id",
				bookID, volumeID, chapter.Title, seq+1, utf8.RuneCountInString(chapter.Content),
			).Scan(&chapterID); err != nil {
				return err
			}

			rawPath, err := storage.WriteRawChapter(cfg.DataDir, bookID, chapterID, chapter.Content)
			if err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, "UPDATE chapters SET raw_path=$1 WHERE id=$2", rawPath, chapterID); err != nil {
				return err
			}
		}

		hasVolumes := 0
		if len(volumes) > 0 {
			hasVolumes = 1
		}
		_, err := tx.Exec(ctx,
			"UPDATE books SET title=$1, author=$2, total_chapters=$3, has_volumes=$4, parse_status='done', parse_error=NULL, updated_at=NOW() WHERE id=$5",
			parsed.Title, parsed.Author, len(chapters), hasVolumes, bookID,
		)
		return err
	})
	if err != nil {
		if markErr := markBookFailed(ctx, pool, bookID, err); markErr != nil {
			log.Printf("markBookFailed: %v", markErr)
		}
	}
}

type normalizedParseResult struct {
	Title        string
	Author       string
	Chapters     []parser.RawChapter
	HasNestedTOC bool
}

func parseBookFile(filePath, ext string) (*normalizedParseResult, error) {
	switch ext {
	case "txt":
		result, err := parser.ParseTXT(filePath)
		if err != nil {
			return nil, err
		}
		return &normalizedParseResult{
			Title:        result.Title,
			Author:       result.Author,
			Chapters:     result.Chapters,
			HasNestedTOC: result.HasNestedTOC,
		}, nil
	case "epub":
		result, err := parser.ParseEPUB(filePath)
		if err != nil {
			return nil, err
		}
		chapters := make([]parser.RawChapter, 0, len(result.Chapters))
		for _, chapter := range result.Chapters {
			chapters = append(chapters, parser.RawChapter{
				Title:       chapter.Title,
				Content:     chapter.Text,
				VolumeTitle: chapter.Volume,
			})
		}
		return &normalizedParseResult{
			Title:        result.Title,
			Author:       result.Author,
			Chapters:     chapters,
			HasNestedTOC: result.HasNestedTOC,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func (h *BooksHandler) insertPendingBook(ctx context.Context, filename, ext string) (int64, error) {
	var bookID int64
	err := db.QueryRow(ctx, h.pool,
		"INSERT INTO books (title, source_format, source_path, parse_status) VALUES ($1, $2, '', 'pending') RETURNING id",
		strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)), ext,
	).Scan(&bookID)
	if err != nil {
		return 0, err
	}
	return bookID, nil
}

func saveUploadedFile(header *multipart.FileHeader, savePath string) error {
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return err
	}

	dst, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	return err
}

func markBookFailed(ctx context.Context, pool *pgxpool.Pool, bookID int64, err error) error {
	message := err.Error()
	if utf8.RuneCountInString(message) > 500 {
		runes := []rune(message)
		message = string(runes[:500])
	}
	_, execErr := db.Exec(ctx, pool, "UPDATE books SET parse_status='failed', parse_error=$1, updated_at=NOW() WHERE id=$2", message, bookID)
	return execErr
}

func pathBookID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBook(row rowScanner) (models.Book, error) {
	var book models.Book
	var hasVolumes int
	err := row.Scan(
		&book.ID,
		&book.Title,
		&book.Author,
		&book.SourceFormat,
		&book.TotalChapters,
		&hasVolumes,
		&book.ParseStatus,
		&book.ParseError,
		&book.CreatedAt,
		&book.DehydratedCount,
	)
	if err != nil {
		return models.Book{}, err
	}
	book.HasVolumes = hasVolumes != 0
	return book, nil
}

func fileExtension(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

func buildOrderClause(sortBy, order string) string {
	// Whitelist column names to prevent SQL injection
	column := "b.created_at"
	switch sortBy {
	case "title":
		column = "b.title"
	case "created_at":
		column = "b.created_at"
	}

	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}

	return "ORDER BY " + column + " " + direction
}
