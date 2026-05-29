package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatedRequest(t *testing.T) {
	password := "testpass"
	token := CreateAuthToken(password)

	handler := AuthMiddleware(password)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "novel_auth", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestUnauthenticatedAPIRequest(t *testing.T) {
	password := "testpass"

	handler := AuthMiddleware(password)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != `{"detail":"Unauthorized"}`+"\n" {
		t.Errorf("expected JSON error body, got %q", body)
	}
}

func TestPublicPathAllowed(t *testing.T) {
	password := "testpass"

	handler := AuthMiddleware(password)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	paths := []string{"/login", "/api/auth/login", "/api/auth/logout", "/static/style.css"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %s: expected 200 OK, got %d", path, rec.Code)
		}
	}
}

func TestNoPasswordMode(t *testing.T) {
	handler := AuthMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK with empty password, got %d", rec.Code)
	}
}
