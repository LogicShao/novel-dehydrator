package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LogicShao/novel-dehydrator/internal/db"
)

func setupFoldersTest(t *testing.T) (*FoldersHandler, func()) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE book_folders, folders RESTART IDENTITY CASCADE`); err != nil {
		pool.Close()
		t.Fatalf("truncate tables: %v", err)
	}

	handler := NewFoldersHandler(pool)

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `TRUNCATE TABLE book_folders, folders RESTART IDENTITY CASCADE`)
		pool.Close()
	}

	return handler, cleanup
}

func TestFoldersList(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	handler.HandleListFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var folders []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &folders); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestFoldersCreate(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	body := bytes.NewBufferString(`{"name":"我的书单"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/folders", body)
	handler.HandleCreateFolder(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var folder map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &folder); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if folder["name"] != "我的书单" {
		t.Fatalf("name = %q, want 我的书单", folder["name"])
	}
}

func TestFoldersCreateEmptyName(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	body := bytes.NewBufferString(`{"name":""}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/folders", body)
	handler.HandleCreateFolder(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestFoldersRename(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"name":"旧名称"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/folders", createBody)
	handler.HandleCreateFolder(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	folderID := int64(created["id"].(float64))

	renameBody := bytes.NewBufferString(`{"name":"新名称"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/folders/"+strconv.FormatInt(folderID, 10), renameBody)
	req = withFolderIDParam(req, folderID)
	handler.HandleRenameFolder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]string
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result["name"] != "新名称" {
		t.Fatalf("name = %q, want 新名称", result["name"])
	}
}

func TestFoldersDelete(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"name":"待删除"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/folders", createBody)
	handler.HandleCreateFolder(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	folderID := int64(created["id"].(float64))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/folders/"+strconv.FormatInt(folderID, 10), nil)
	req = withFolderIDParam(req, folderID)
	handler.HandleDeleteFolder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestFoldersAddBooks(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"name":"书单"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/folders", createBody)
	handler.HandleCreateFolder(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	folderID := int64(created["id"].(float64))

	addBody := bytes.NewBufferString(`{"book_ids":[1,2,3]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/folders/"+strconv.FormatInt(folderID, 10)+"/books", addBody)
	req = withFolderIDParam(req, folderID)
	handler.HandleAddBooks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestFoldersRemoveBook(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"name":"书单"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/folders", createBody)
	handler.HandleCreateFolder(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	folderID := int64(created["id"].(float64))

	addBody := bytes.NewBufferString(`{"book_ids":[1]}`)
	addRec := httptest.NewRecorder()
	addReq := httptest.NewRequest(http.MethodPost, "/api/folders/"+strconv.FormatInt(folderID, 10)+"/books", addBody)
	addReq = withFolderIDParam(addReq, folderID)
	handler.HandleAddBooks(addRec, addReq)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/folders/"+strconv.FormatInt(folderID, 10)+"/books/1", nil)
	req = withFolderAndBookIDParam(req, folderID, 1)
	handler.HandleRemoveBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestFoldersListBooks(t *testing.T) {
	handler, cleanup := setupFoldersTest(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"name":"书单"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/folders", createBody)
	handler.HandleCreateFolder(createRec, createReq)

	var created map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &created)
	folderID := int64(created["id"].(float64))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/folders/"+strconv.FormatInt(folderID, 10)+"/books", nil)
	req = withFolderIDParam(req, folderID)
	handler.HandleListFolderBooks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func withFolderIDParam(req *http.Request, folderID int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("folderID", strconv.FormatInt(folderID, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func withFolderAndBookIDParam(req *http.Request, folderID, bookID int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("folderID", strconv.FormatInt(folderID, 10))
	rctx.URLParams.Add("bookID", strconv.FormatInt(bookID, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
