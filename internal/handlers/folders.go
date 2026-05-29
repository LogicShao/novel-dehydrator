package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/db"
	"github.com/LogicShao/novel-dehydrator/internal/logger"
	"github.com/LogicShao/novel-dehydrator/internal/models"
)

type FoldersHandler struct {
	pool *pgxpool.Pool
}

func NewFoldersHandler(pool *pgxpool.Pool) *FoldersHandler {
	return &FoldersHandler{pool: pool}
}

const listFoldersSQL = `SELECT f.id, f.name, f.created_at,
	COUNT(bf.book_id) AS book_count
FROM folders f
LEFT JOIN book_folders bf ON f.id = bf.folder_id
GROUP BY f.id, f.name, f.created_at
ORDER BY f.created_at DESC`

func (h *FoldersHandler) HandleListFolders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := db.Query(ctx, h.pool, listFoldersSQL)
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}
	defer rows.Close()

	folders := make([]models.Folder, 0)
	for rows.Next() {
		var f models.Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt, &f.BookCount); err != nil {
			logger.WriteError(w, logger.InternalError(err.Error()))
			return
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, folders)
}

func (h *FoldersHandler) HandleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req models.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WriteError(w, logger.BadRequest("无效的请求体"))
		return
	}
	if req.Name == "" {
		logger.WriteError(w, logger.BadRequest("文件夹名称不能为空"))
		return
	}

	ctx := r.Context()
	var folder models.Folder
	err := db.QueryRow(ctx, h.pool,
		"INSERT INTO folders (name) VALUES ($1) RETURNING id, name, created_at",
		req.Name,
	).Scan(&folder.ID, &folder.Name, &folder.CreatedAt)
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusCreated, folder)
}

func (h *FoldersHandler) HandleRenameFolder(w http.ResponseWriter, r *http.Request) {
	folderID, err := strconv.ParseInt(chi.URLParam(r, "folderID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的文件夹ID"))
		return
	}

	var req models.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WriteError(w, logger.BadRequest("无效的请求体"))
		return
	}
	if req.Name == "" {
		logger.WriteError(w, logger.BadRequest("文件夹名称不能为空"))
		return
	}

	ctx := r.Context()
	tag, err := db.Exec(ctx, h.pool, "UPDATE folders SET name=$1 WHERE id=$2", req.Name, folderID)
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}
	if tag.RowsAffected() == 0 {
		logger.WriteError(w, logger.NotFound("文件夹不存在"))
		return
	}

	logger.WriteJSON(w, http.StatusOK, map[string]string{"name": req.Name})
}

func (h *FoldersHandler) HandleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	folderID, err := strconv.ParseInt(chi.URLParam(r, "folderID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的文件夹ID"))
		return
	}

	ctx := r.Context()
	tag, err := db.Exec(ctx, h.pool, "DELETE FROM folders WHERE id=$1", folderID)
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}
	if tag.RowsAffected() == 0 {
		logger.WriteError(w, logger.NotFound("文件夹不存在"))
		return
	}

	logger.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *FoldersHandler) HandleAddBooks(w http.ResponseWriter, r *http.Request) {
	folderID, err := strconv.ParseInt(chi.URLParam(r, "folderID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的文件夹ID"))
		return
	}

	var req models.AddBooksToFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WriteError(w, logger.BadRequest("无效的请求体"))
		return
	}
	if len(req.BookIDs) == 0 {
		logger.WriteError(w, logger.BadRequest("book_ids 不能为空"))
		return
	}

	ctx := r.Context()
	err = db.WithTransaction(ctx, h.pool, func(tx pgx.Tx) error {
		for _, bookID := range req.BookIDs {
			_, err := tx.Exec(ctx,
				"INSERT INTO book_folders (book_id, folder_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				bookID, folderID,
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *FoldersHandler) HandleRemoveBook(w http.ResponseWriter, r *http.Request) {
	folderID, err := strconv.ParseInt(chi.URLParam(r, "folderID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的文件夹ID"))
		return
	}
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的书籍ID"))
		return
	}

	ctx := r.Context()
	_, err = db.Exec(ctx, h.pool, "DELETE FROM book_folders WHERE folder_id=$1 AND book_id=$2", folderID, bookID)
	if err != nil {
		logger.WriteError(w, logger.InternalError(err.Error()))
		return
	}

	logger.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *FoldersHandler) HandleListFolderBooks(w http.ResponseWriter, r *http.Request) {
	folderID, err := strconv.ParseInt(chi.URLParam(r, "folderID"), 10, 64)
	if err != nil {
		logger.WriteError(w, logger.BadRequest("无效的文件夹ID"))
		return
	}

	ctx := r.Context()
	rows, err := db.Query(ctx, h.pool,
		listBooksSQL+` INNER JOIN book_folders bf ON b.id = bf.book_id WHERE bf.folder_id = $1 ORDER BY b.created_at DESC`,
		folderID,
	)
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
