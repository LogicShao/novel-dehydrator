package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/middleware"
)

func TestHandleLoginSuccess(t *testing.T) {
	cfg := &config.Config{AuthPassword: "testpass"}
	handler := HandleLogin(cfg)

	body := `{"password":"testpass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !result["ok"] {
		t.Error("expected ok=true")
	}

	// Verify cookie was set
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != middleware.AUTH_COOKIE_NAME {
		t.Errorf("cookie name: expected %s, got %s", middleware.AUTH_COOKIE_NAME, c.Name)
	}
	if c.Value != middleware.CreateAuthToken("testpass") {
		t.Error("cookie value does not match expected token")
	}
	if c.Path != "/" {
		t.Errorf("cookie path: expected /, got %s", c.Path)
	}
	if !c.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if c.MaxAge != 2592000 {
		t.Errorf("cookie MaxAge: expected 2592000, got %d", c.MaxAge)
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	cfg := &config.Config{AuthPassword: "testpass"}
	handler := HandleLogin(cfg)

	body := `{"password":"wrongpass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleLoginNoPasswordConfigured(t *testing.T) {
	cfg := &config.Config{AuthPassword: ""}
	handler := HandleLogin(cfg)

	body := `{"password":"anything"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when auth not configured, got %d", rec.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	handler := HandleLogout()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !result["ok"] {
		t.Error("expected ok=true")
	}

	// Verify cookie is cleared (MaxAge=-1)
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == middleware.AUTH_COOKIE_NAME && c.MaxAge == -1 {
			found = true
		}
	}
	if !found {
		t.Error("logout should set cookie with MaxAge=-1")
	}
}

func TestHandleStatusAuthenticated(t *testing.T) {
	cfg := &config.Config{AuthPassword: "testpass"}
	handler := HandleStatus(cfg)

	token := middleware.CreateAuthToken("testpass")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: middleware.AUTH_COOKIE_NAME, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !result["authenticated"] {
		t.Error("expected authenticated=true")
	}
}

func TestHandleStatusNotAuthenticated(t *testing.T) {
	cfg := &config.Config{AuthPassword: "testpass"}
	handler := HandleStatus(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["authenticated"] {
		t.Error("expected authenticated=false without cookie")
	}
}

func TestHandleStatusNoPassword(t *testing.T) {
	cfg := &config.Config{AuthPassword: ""}
	handler := HandleStatus(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !result["authenticated"] {
		t.Error("expected authenticated=true when no password configured")
	}
}
