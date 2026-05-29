package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandleIndex(t *testing.T) {
	handler := HandleIndex()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "小说速读") {
		t.Error("expected 小说速读 in body")
	}
}

func TestHandleLoginPage(t *testing.T) {
	handler := HandleLoginPage()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `type="password"`) {
		t.Error("expected password input in body")
	}
}

func TestHandleBookPage(t *testing.T) {
	handler := HandleBookPage()
	r := chi.NewRouter()
	r.Get("/book/{bookID}", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/book/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bookDetail(42)") {
		t.Errorf("expected bookDetail(42), got %q", rec.Body.String()[:100])
	}
}

func TestHandleReaderPage(t *testing.T) {
	handler := HandleReaderPage()
	r := chi.NewRouter()
	r.Get("/book/{bookID}/reader", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/book/99/reader", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "reader(99)") {
		t.Errorf("expected reader(99), got %q", rec.Body.String()[:100])
	}
}

func TestPagesHandlerSignatures(t *testing.T) {
	if h := HandleIndex(); h == nil {
		t.Error("HandleIndex returned nil")
	}
	if h := HandleLoginPage(); h == nil {
		t.Error("HandleLoginPage returned nil")
	}
	if h := HandleBookPage(); h == nil {
		t.Error("HandleBookPage returned nil")
	}
	if h := HandleReaderPage(); h == nil {
		t.Error("HandleReaderPage returned nil")
	}
}
