package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const AUTH_COOKIE_NAME = "novel_auth"

var PUBLIC_PATHS = map[string]bool{
	"/login":          true,
	"/api/auth/login": true,
	"/api/auth/logout": true,
}

// CreateAuthToken generates a SHA-256 hex token from the given password.
func CreateAuthToken(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}

// AuthMiddleware returns a chi-compatible middleware that enforces cookie-based
// authentication. If authPassword is empty authentication is skipped entirely.
// Public paths (/static/*, /login, /api/auth/login, /api/auth/logout) bypass
// authentication. API paths return 401 JSON; all other paths redirect to /login.
func AuthMiddleware(authPassword string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authPassword == "" {
				next.ServeHTTP(w, r)
				return
			}

			path := r.URL.Path

			if PUBLIC_PATHS[path] {
				next.ServeHTTP(w, r)
				return
			}

			if len(path) >= 8 && path[:8] == "/static/" {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(AUTH_COOKIE_NAME)
			actualToken := ""
			if err == nil {
				actualToken = cookie.Value
			}

			expectedToken := CreateAuthToken(authPassword)

			if hmac.Equal([]byte(actualToken), []byte(expectedToken)) {
				next.ServeHTTP(w, r)
				return
			}

			if len(path) >= 4 && path[:4] == "/api" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"detail": "Unauthorized"})
				return
			}

			nextPath := path
			if r.URL.RawQuery != "" {
				nextPath += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, "/login?next="+url.QueryEscape(nextPath), http.StatusTemporaryRedirect)
		})
	}
}
