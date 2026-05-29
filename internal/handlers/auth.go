package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/middleware"
)

// HandleLogin handles POST /api/auth/login.
func HandleLogin(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"detail":"invalid request"}`, http.StatusBadRequest)
			return
		}

		if cfg.AuthPassword == "" {
			http.Error(w, `{"detail":"AUTH_PASSWORD is not configured"}`, http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(req.Password), []byte(cfg.AuthPassword)) != 1 {
			http.Error(w, `{"detail":"Invalid password"}`, http.StatusUnauthorized)
			return
		}

		token := middleware.CreateAuthToken(cfg.AuthPassword)
		http.SetCookie(w, &http.Cookie{
			Name:     middleware.AUTH_COOKIE_NAME,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   2592000,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// HandleLogout handles POST /api/auth/logout.
func HandleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     middleware.AUTH_COOKIE_NAME,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// HandleStatus handles GET /api/auth/status.
func HandleStatus(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticated := false
		if cfg.AuthPassword == "" {
			authenticated = true
		} else {
			cookie, err := r.Cookie(middleware.AUTH_COOKIE_NAME)
			if err == nil {
				expectedToken := middleware.CreateAuthToken(cfg.AuthPassword)
				if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expectedToken)) == 1 {
					authenticated = true
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authenticated": authenticated})
	}
}
