package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type stubExportStore struct {
	title          string
	titleErr       error
	rawPath        string
	dehydratedPath *string
	status         string
	chapterErr     error
}

func (s *stubExportStore) GetBookTitle(context.Context, int64) (string, error) { return s.title, s.titleErr }
func (s *stubExportStore) GetChapterPaths(context.Context, int64, int64) (string, *string, string, error) {
	return s.rawPath, s.dehydratedPath, s.status, s.chapterErr
}

type stubExportService struct {
	txt  []byte
	epub []byte
	err  error
}

func (s *stubExportService) ExportTXT(context.Context, int64) ([]byte, error)  { return s.txt, s.err }
func (s *stubExportService) ExportEPUB(context.Context, int64) ([]byte, error) { return s.epub, s.err }

func TestHandleExportTXT(t *testing.T) {
	handler := handleExport(&stubExportStore{title: "测试书"}, &stubExportService{txt: []byte("hello")})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRouteRequest(http.MethodGet, "/api/books/1/export?format=txt", map[string]string{"bookID": "1"}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), `attachment; filename="测试书_脱水版.txt"`) {
		t.Fatalf("unexpected disposition: %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestHandleExportBookNotFound(t *testing.T) {
	handler := handleExport(&stubExportStore{titleErr: pgx.ErrNoRows}, &stubExportService{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRouteRequest(http.MethodGet, "/api/books/1/export?format=epub", map[string]string{"bookID": "1"}, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleChapterContent(t *testing.T) {
	path := "dehydrated.txt"
	handler := handleChapterContent(&stubExportStore{rawPath: "raw.txt", dehydratedPath: &path, status: "done"}, func(path string) (string, error) {
		if path != "dehydrated.txt" {
			t.Fatalf("path = %q, want dehydrated.txt", path)
		}
		return "正文\n---CHAPTER_META---\n备注", nil
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRouteRequest(http.MethodGet, "/api/books/1/chapters/2/content?version=dehydrated", map[string]string{"bookID": "1", "chapterID": "2"}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "正文") || strings.Contains(rec.Body.String(), "备注") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestHandleChapterContentReadError(t *testing.T) {
	handler := handleChapterContent(&stubExportStore{rawPath: "raw.txt", status: "pending"}, func(string) (string, error) {
		return "", errors.New("boom")
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRouteRequest(http.MethodGet, "/api/books/1/chapters/2/content?version=raw", map[string]string{"bookID": "1", "chapterID": "2"}, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
