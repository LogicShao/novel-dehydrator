package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/models"
)

const maskedKey = "****"

func fallbackAPIKey(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.DeepseekAPIKey)
}

func fallbackModel(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.DeepseekModel) != "" {
		return strings.TrimSpace(cfg.DeepseekModel)
	}
	return "deepseek-v4-flash"
}

func fallbackBaseURL(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.DeepseekBaseURL) != "" {
		return strings.TrimSpace(cfg.DeepseekBaseURL)
	}
	return "https://api.deepseek.com"
}

func hasConfiguredAPIKey(dbKey, fallbackKey string) bool {
	return strings.TrimSpace(dbKey) != "" || strings.TrimSpace(fallbackKey) != ""
}

func buildSettingsOut(cfg *config.Config, apiKey, model, baseURL string, concurrency int, sysPrompt string) models.SettingsOut {
	out := models.SettingsOut{
		DeepseekAPIKey:   "",
		APIKeyConfigured: hasConfiguredAPIKey(apiKey, fallbackAPIKey(cfg)),
		DeepseekModel:    strings.TrimSpace(model),
		DeepseekBaseURL:  strings.TrimSpace(baseURL),
		Concurrency:      concurrency,
		SystemPrompt:     sysPrompt,
	}
	if out.APIKeyConfigured {
		out.DeepseekAPIKey = maskedKey
	}
	if out.DeepseekModel == "" {
		out.DeepseekModel = fallbackModel(cfg)
	}
	if out.DeepseekBaseURL == "" {
		out.DeepseekBaseURL = fallbackBaseURL(cfg)
	}
	if out.Concurrency == 0 {
		out.Concurrency = 5
	}
	return out
}

func HandleGetSettings(pool *pgxpool.Pool, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			apiKey      string
			model       string
			baseURL     string
			concurrency int
			sysPrompt   string
		)

		if pool != nil {
			if err := pool.QueryRow(r.Context(),
				`SELECT deepseek_api_key, deepseek_model, deepseek_base_url, concurrency, system_prompt FROM app_settings WHERE id=1`,
			).Scan(&apiKey, &model, &baseURL, &concurrency, &sysPrompt); err != nil {
				slog.Warn("settings: query app_settings failed, using defaults", "error", err)
			}
		}
		out := buildSettingsOut(cfg, apiKey, model, baseURL, concurrency, sysPrompt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

func HandleUpdateSettings(pool *pgxpool.Pool, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, `{"detail":"failed to update settings"}`, http.StatusInternalServerError)
			return
		}

		var in models.SettingsIn
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"detail":"invalid request"}`, http.StatusBadRequest)
			return
		}

		keyToSave := in.DeepseekAPIKey
		if keyToSave == "" || keyToSave == maskedKey {
			var existingKey string
			err := pool.QueryRow(r.Context(),
				`SELECT deepseek_api_key FROM app_settings WHERE id=1`,
			).Scan(&existingKey)
			if err == nil {
				keyToSave = existingKey
			} else {
				keyToSave = ""
			}
		}

		_, err := pool.Exec(r.Context(),
			`UPDATE app_settings SET deepseek_api_key=$1, deepseek_model=$2, deepseek_base_url=$3, concurrency=$4, system_prompt=$5 WHERE id=1`,
			keyToSave, in.DeepseekModel, in.DeepseekBaseURL, in.Concurrency, in.SystemPrompt,
		)
		if err != nil {
			http.Error(w, `{"detail":"failed to update settings"}`, http.StatusInternalServerError)
			return
		}

		out := buildSettingsOut(cfg, keyToSave, in.DeepseekModel, in.DeepseekBaseURL, in.Concurrency, in.SystemPrompt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}
