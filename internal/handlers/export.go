package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/services/exporter"
	"github.com/LogicShao/novel-dehydrator/internal/storage"
)

type exportChapterStore interface {
	GetBookTitle(ctx context.Context, bookID int64) (string, error)
	GetChapterPaths(ctx context.Context, bookID, chapterID int64) (rawPath string, dehydratedPath *string, status string, err error)
}

type exportService interface {
	ExportTXT(ctx context.Context, bookID int64) ([]byte, error)
	ExportEPUB(ctx context.Context, bookID int64) ([]byte, error)
}

type chapterReader func(path string) (string, error)

type postgresExportStore struct{ pool *pgxpool.Pool }

type exporterService struct {
	pool    *pgxpool.Pool
	dataDir string
}

func HandleExport(pool *pgxpool.Pool, dataDir string) http.HandlerFunc {
	return handleExport(&postgresExportStore{pool: pool}, &exporterService{pool: pool, dataDir: dataDir})
}

func HandleChapterContent(pool *pgxpool.Pool) http.HandlerFunc {
	return handleChapterContent(&postgresExportStore{pool: pool}, storage.ReadChapter)
}

func handleExport(store exportChapterStore, service exportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "txt"
		}
		title, err := store.GetBookTitle(r.Context(), bookID)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeJSONError(w, http.StatusNotFound, "书籍不存在")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "查询书籍失败")
			return
		}

		var (
			data        []byte
			contentType string
		)
		switch format {
		case "txt":
			data, err = service.ExportTXT(r.Context(), bookID)
			contentType = "text/plain; charset=utf-8"
		case "epub":
			data, err = service.ExportEPUB(r.Context(), bookID)
			contentType = "application/epub+zip"
		default:
			writeJSONError(w, http.StatusBadRequest, "invalid format")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "导出失败")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_脱水版.%s"`, title, format))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			slog.Warn("export write failed", "bookID", bookID, "error", err)
		}
	}
}

func handleChapterContent(store exportChapterStore, readChapter chapterReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}
		chapterID, ok := parseInt64Param(w, r, "chapterID")
		if !ok {
			return
		}
		version := r.URL.Query().Get("version")
		if version == "" {
			version = "dehydrated"
		}
		if version != "raw" && version != "dehydrated" {
			writeJSONError(w, http.StatusBadRequest, "invalid version")
			return
		}

		rawPath, dehydratedPath, status, err := store.GetChapterPaths(r.Context(), bookID, chapterID)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeJSONError(w, http.StatusNotFound, "章节不存在")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "查询章节失败")
			return
		}

		path := rawPath
		if version == "dehydrated" && status == "done" && dehydratedPath != nil && *dehydratedPath != "" {
			path = *dehydratedPath
		}
		text, err := readChapter(path)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "章节文件读取失败")
			return
		}
		if version == "dehydrated" {
			text = exporter.StripMeta(text)
		}

		writeJSON(w, http.StatusOK, map[string]string{"text": text, "version": version})
	}
}

func (s *postgresExportStore) GetBookTitle(ctx context.Context, bookID int64) (string, error) {
	var title string
	err := s.pool.QueryRow(ctx, `SELECT title FROM books WHERE id=$1`, bookID).Scan(&title)
	return title, err
}

func (s *postgresExportStore) GetChapterPaths(ctx context.Context, bookID, chapterID int64) (string, *string, string, error) {
	var rawPath string
	var dehydratedPath *string
	var status string
	err := s.pool.QueryRow(ctx, `SELECT raw_path, dehydrated_path, dehydrate_status FROM chapters WHERE id=$1 AND book_id=$2`, chapterID, bookID).
		Scan(&rawPath, &dehydratedPath, &status)
	return rawPath, dehydratedPath, status, err
}

func (s *exporterService) ExportTXT(ctx context.Context, bookID int64) ([]byte, error) {
	return exporter.ExportTXT(ctx, s.pool, bookID, s.dataDir)
}

func (s *exporterService) ExportEPUB(ctx context.Context, bookID int64) ([]byte, error) {
	return exporter.ExportEPUB(ctx, s.pool, bookID, s.dataDir)
}


