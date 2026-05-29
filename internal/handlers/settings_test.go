package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/models"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test: TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping test: cannot connect to test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func seedSettings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), `
		INSERT INTO app_settings (id, deepseek_api_key, deepseek_model, deepseek_base_url, concurrency, system_prompt)
		VALUES (1, 'sk-test-key', 'deepseek-v4-flash', 'https://api.deepseek.com', 10, 'You are helpful.')
		ON CONFLICT (id) DO UPDATE SET
			deepseek_api_key=EXCLUDED.deepseek_api_key,
			deepseek_model=EXCLUDED.deepseek_model,
			deepseek_base_url=EXCLUDED.deepseek_base_url,
			concurrency=EXCLUDED.concurrency,
			system_prompt=EXCLUDED.system_prompt
	`)
}

func TestHandleGetSettingsHandlerType(t *testing.T) {
	handler := HandleGetSettings(nil, &config.Config{})
	if handler == nil {
		t.Fatal("HandleGetSettings returned nil handler")
	}
}

func TestHandleUpdateSettingsHandlerType(t *testing.T) {
	handler := HandleUpdateSettings(nil, &config.Config{})
	if handler == nil {
		t.Fatal("HandleUpdateSettings returned nil handler")
	}
}

func TestHandleGetSettingsIntegration(t *testing.T) {
	pool := getTestPool(t)
	seedSettings(t, pool)

	handler := HandleGetSettings(pool, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var out models.SettingsOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if out.DeepseekAPIKey != maskedKey {
		t.Errorf("expected masked key %q, got %q", maskedKey, out.DeepseekAPIKey)
	}
	if !out.APIKeyConfigured {
		t.Error("expected api_key_configured=true")
	}
	if out.DeepseekModel != "deepseek-v4-flash" {
		t.Errorf("expected model deepseek-v4-flash, got %q", out.DeepseekModel)
	}
	if out.Concurrency != 10 {
		t.Errorf("expected concurrency 10, got %d", out.Concurrency)
	}
}

func TestHandleUpdateSettingsIntegration(t *testing.T) {
	pool := getTestPool(t)
	seedSettings(t, pool)

	body := `{"deepseek_api_key":"****","deepseek_model":"deepseek-v4-pro","deepseek_base_url":"https://api.deepseek.com","concurrency":20,"system_prompt":"New prompt"}`
	handler := HandleUpdateSettings(pool, &config.Config{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out models.SettingsOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if out.DeepseekAPIKey != maskedKey {
		t.Errorf("expected masked key %q, got %q", maskedKey, out.DeepseekAPIKey)
	}
	if !out.APIKeyConfigured {
		t.Error("expected api_key_configured=true (key preserved from DB)")
	}
	if out.DeepseekModel != "deepseek-v4-pro" {
		t.Errorf("expected model deepseek-v4-pro, got %q", out.DeepseekModel)
	}
	if out.Concurrency != 20 {
		t.Errorf("expected concurrency 20, got %d", out.Concurrency)
	}
	if out.SystemPrompt != "New prompt" {
		t.Errorf("expected system_prompt, got %q", out.SystemPrompt)
	}
}

func TestHandleUpdateSettingsNewKey(t *testing.T) {
	pool := getTestPool(t)
	seedSettings(t, pool)

	body := `{"deepseek_api_key":"sk-new-key","deepseek_model":"deepseek-v4-flash","deepseek_base_url":"https://api.deepseek.com","concurrency":5,"system_prompt":""}`
	handler := HandleUpdateSettings(pool, &config.Config{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out models.SettingsOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if out.DeepseekAPIKey != maskedKey {
		t.Errorf("expected masked key, got %q", out.DeepseekAPIKey)
	}
	if !out.APIKeyConfigured {
		t.Error("expected api_key_configured=true with new key")
	}
}

func TestBuildSettingsOutFallsBackToConfigKey(t *testing.T) {
	out := buildSettingsOut(&config.Config{
		DeepseekAPIKey:  "sk-env-key",
		DeepseekModel:   "deepseek-v4-flash",
		DeepseekBaseURL: "https://api.deepseek.com",
	}, "", "", "", 0, "")

	if out.DeepseekAPIKey != maskedKey {
		t.Fatalf("DeepseekAPIKey = %q, want %q", out.DeepseekAPIKey, maskedKey)
	}
	if !out.APIKeyConfigured {
		t.Fatal("APIKeyConfigured = false, want true")
	}
	if out.DeepseekModel != "deepseek-v4-flash" {
		t.Fatalf("DeepseekModel = %q, want deepseek-v4-flash", out.DeepseekModel)
	}
	if out.DeepseekBaseURL != "https://api.deepseek.com" {
		t.Fatalf("DeepseekBaseURL = %q, want https://api.deepseek.com", out.DeepseekBaseURL)
	}
	if out.Concurrency != 5 {
		t.Fatalf("Concurrency = %d, want 5", out.Concurrency)
	}
}
