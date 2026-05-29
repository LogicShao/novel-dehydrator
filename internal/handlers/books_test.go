package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/db"
	"github.com/LogicShao/novel-dehydrator/internal/storage"
)

func TestBooksList(t *testing.T) {
	pool, handler, _, cleanup := setupBooksTest(t)
	defer cleanup()

	ctx := context.Background()
	var olderID, newerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO books (title, author, source_format, source_path, total_chapters, has_volumes, parse_status)
		VALUES ('旧书', '作者甲', 'txt', '/tmp/old.txt', 3, 0, 'done') RETURNING id`).Scan(&olderID); err != nil {
		t.Fatalf("insert older book: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO books (title, author, source_format, source_path, total_chapters, has_volumes, parse_status)
		VALUES ('新书', '作者乙', 'epub', '/tmp/new.epub', 5, 1, 'done') RETURNING id`).Scan(&newerID); err != nil {
		t.Fatalf("insert newer book: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chapters (book_id, title, seq, raw_path, raw_char_count, dehydrate_status)
		VALUES ($1, '第一章', 1, '/tmp/1.txt', 10, 'done'),
		       ($1, '第二章', 2, '/tmp/2.txt', 20, 'pending'),
		       ($2, '第三章', 1, '/tmp/3.txt', 30, 'done')`, olderID, newerID); err != nil {
		t.Fatalf("insert chapters: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	handler.HandleListBooks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var books []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &books); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	if int64(books[0]["id"].(float64)) != newerID {
		t.Fatalf("first id = %v, want %d", books[0]["id"], newerID)
	}
	if books[0]["title"] != "新书" {
		t.Fatalf("first title = %v, want 新书", books[0]["title"])
	}
	if int(books[0]["dehydrated_count"].(float64)) != 1 {
		t.Fatalf("first dehydrated_count = %v, want 1", books[0]["dehydrated_count"])
	}
	if int64(books[1]["id"].(float64)) != olderID {
		t.Fatalf("second id = %v, want %d", books[1]["id"], olderID)
	}
	if int(books[1]["dehydrated_count"].(float64)) != 1 {
		t.Fatalf("second dehydrated_count = %v, want 1", books[1]["dehydrated_count"])
	}
}

func TestBooksUploadTXT(t *testing.T) {
	pool, handler, cfg, cleanup := setupBooksTest(t)
	defer cleanup()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "fanren.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	content := "第一章 山边小村\n韩立出生在一个小山村。\n第二章 青牛镇\n韩立跟随三叔前往七玄门。\n"
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write multipart content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/books/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	handler.HandleUploadBooks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			Filename    string `json:"filename"`
			BookID      int64  `json:"book_id"`
			ParseStatus string `json:"parse_status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(resp.Results))
	}
	result := resp.Results[0]
	if result.Filename != "fanren.txt" {
		t.Fatalf("filename = %q, want fanren.txt", result.Filename)
	}
	if result.BookID <= 0 {
		t.Fatalf("book_id = %d, want > 0", result.BookID)
	}
	if result.ParseStatus != "parsing" {
		t.Fatalf("parse_status = %q, want parsing", result.ParseStatus)
	}

	waitForBookParseDone(t, pool, result.BookID)

	var title, sourcePath, parseStatus string
	var totalChapters int
	if err := pool.QueryRow(context.Background(), `SELECT title, source_path, parse_status, total_chapters FROM books WHERE id=$1`, result.BookID).
		Scan(&title, &sourcePath, &parseStatus, &totalChapters); err != nil {
		t.Fatalf("query uploaded book: %v", err)
	}
	if title != "fanren" {
		t.Fatalf("title = %q, want fanren", title)
	}
	if parseStatus != "done" {
		t.Fatalf("parse_status = %q, want done", parseStatus)
	}
	if totalChapters != 2 {
		t.Fatalf("total_chapters = %d, want 2", totalChapters)
	}
	expectedPath := filepath.Join(cfg.DataDir, "uploads", "1.txt")
	if sourcePath != expectedPath {
		t.Fatalf("source_path = %q, want %q", sourcePath, expectedPath)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}

	var rawPath string
	if err := pool.QueryRow(context.Background(), `SELECT raw_path FROM chapters WHERE book_id=$1 AND seq=1`, result.BookID).Scan(&rawPath); err != nil {
		t.Fatalf("query raw_path: %v", err)
	}
	if _, err := os.Stat(rawPath); err != nil {
		t.Fatalf("raw chapter missing: %v", err)
	}
}

func TestBooksGetBook(t *testing.T) {
	pool, handler, _, cleanup := setupBooksTest(t)
	defer cleanup()

	ctx := context.Background()
	var bookID int64
	if err := pool.QueryRow(ctx, `INSERT INTO books (title, author, source_format, source_path, total_chapters, has_volumes, parse_status)
		VALUES ('测试书', '测试作者', 'txt', '/tmp/book.txt', 2, 1, 'done') RETURNING id`).Scan(&bookID); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chapters (book_id, title, seq, raw_path, raw_char_count, dehydrate_status)
		VALUES ($1, '第一章', 1, '/tmp/1.txt', 10, 'done'),
		       ($1, '第二章', 2, '/tmp/2.txt', 20, 'pending')`, bookID); err != nil {
		t.Fatalf("insert chapters: %v", err)
	}

	t.Run("exists", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withBookIDParam(httptest.NewRequest(http.MethodGet, "/api/books/1", nil), bookID)
		handler.HandleGetBook(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var book map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &book); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if int64(book["id"].(float64)) != bookID {
			t.Fatalf("id = %v, want %d", book["id"], bookID)
		}
		if book["title"] != "测试书" {
			t.Fatalf("title = %v, want 测试书", book["title"])
		}
		if int(book["dehydrated_count"].(float64)) != 1 {
			t.Fatalf("dehydrated_count = %v, want 1", book["dehydrated_count"])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withBookIDParam(httptest.NewRequest(http.MethodGet, "/api/books/999999", nil), 999999)
		handler.HandleGetBook(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "书籍不存在") {
			t.Fatalf("body = %q, want contain 书籍不存在", rec.Body.String())
		}
	})
}

func TestBooksDelete(t *testing.T) {
	pool, handler, cfg, cleanup := setupBooksTest(t)
	defer cleanup()

	ctx := context.Background()
	var bookID int64
	if err := pool.QueryRow(ctx, `INSERT INTO books (title, author, source_format, source_path, total_chapters, has_volumes, parse_status)
		VALUES ('待删书', '作者', 'txt', $1, 1, 0, 'done') RETURNING id`, filepath.Join(cfg.DataDir, "uploads", "to-delete.txt")).Scan(&bookID); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "uploads", "to-delete.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write upload file: %v", err)
	}
	rawPath, err := storage.WriteRawChapter(cfg.DataDir, bookID, 1, "第一章 正文")
	if err != nil {
		t.Fatalf("WriteRawChapter: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chapters (book_id, title, seq, raw_path, raw_char_count, dehydrate_status)
		VALUES ($1, '第一章', 1, $2, 5, 'done')`, bookID, rawPath); err != nil {
		t.Fatalf("insert chapter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := withBookIDParam(httptest.NewRequest(http.MethodDelete, "/api/books/1", nil), bookID)
	handler.HandleDeleteBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("body = %q, want {\"ok\":true}", strings.TrimSpace(rec.Body.String()))
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM books WHERE id=$1`, bookID).Scan(&count); err != nil {
		t.Fatalf("count books: %v", err)
	}
	if count != 0 {
		t.Fatalf("remaining books = %d, want 0", count)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "uploads", "to-delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("upload file still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "books", strconv.FormatInt(bookID, 10))); !os.IsNotExist(err) {
		t.Fatalf("book files still exist: %v", err)
	}

	listRec := httptest.NewRecorder()
	handler.HandleListBooks(listRec, httptest.NewRequest(http.MethodGet, "/api/books", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	if strings.TrimSpace(listRec.Body.String()) != "[]" {
		t.Fatalf("list body = %q, want []", strings.TrimSpace(listRec.Body.String()))
	}
}

func setupBooksTest(t *testing.T) (*pgxpool.Pool, *BooksHandler, *config.Config, func()) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE job_chapters, jobs, chapters, volumes, books RESTART IDENTITY CASCADE`); err != nil {
		pool.Close()
		t.Fatalf("truncate tables: %v", err)
	}

	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "uploads"), 0o755); err != nil {
		pool.Close()
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "books"), 0o755); err != nil {
		pool.Close()
		t.Fatalf("mkdir books: %v", err)
	}

	cfg := &config.Config{DataDir: dataDir}
	handler := NewBooksHandler(pool, cfg)

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `TRUNCATE TABLE job_chapters, jobs, chapters, volumes, books RESTART IDENTITY CASCADE`)
		pool.Close()
	}

	return pool, handler, cfg, cleanup
}

func waitForBookParseDone(t *testing.T, pool *pgxpool.Pool, bookID int64) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var parseError *string
		err := pool.QueryRow(context.Background(), `SELECT parse_status, parse_error FROM books WHERE id=$1`, bookID).Scan(&status, &parseError)
		if err != nil {
			t.Fatalf("query parse status: %v", err)
		}
		if status == "done" {
			return
		}
		if status == "failed" {
			if parseError == nil {
				t.Fatal("parse failed without parse_error")
			}
			t.Fatalf("parse failed: %s", *parseError)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for parse completion")
}

func withBookIDParam(req *http.Request, bookID int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("bookID", strconv.FormatInt(bookID, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
