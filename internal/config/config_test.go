package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	// Use temp dir for DATA_DIR to avoid filesystem pollution
	t.Setenv("DATA_DIR", t.TempDir())

	cfg := Load()

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"DeepseekAPIKey", cfg.DeepseekAPIKey, ""},
		{"DeepseekModel", cfg.DeepseekModel, "deepseek-v4-flash"},
		{"DeepseekBaseURL", cfg.DeepseekBaseURL, "https://api.deepseek.com"},
		{"AuthPassword", cfg.AuthPassword, ""},
		{"DatabaseURL", cfg.DatabaseURL, "postgres://localhost:5432/novel_dehydrator"},
		{"Port", cfg.Port, "8765"},
		{"DataDir", cfg.DataDir, os.Getenv("DATA_DIR")}, // should match the env we set
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}

	intChecks := []struct {
		field string
		got   int
		want  int
	}{
		{"DehydrateConcurrency", cfg.DehydrateConcurrency, 20},
		{"MaxRetries", cfg.MaxRetries, 3},
		{"ChunkCharLimit", cfg.ChunkCharLimit, 12000},
	}

	for _, c := range intChecks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.field, c.got, c.want)
		}
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("DEEPSEEK_MODEL", "deepseek-v4-pro")
	t.Setenv("DEHYDRATE_CONCURRENCY", "10")
	t.Setenv("PORT", "9999")
	t.Setenv("AUTH_PASSWORD", "secret123")
	t.Setenv("DATABASE_URL", "postgres://other:5432/mydb")

	cfg := Load()

	if cfg.DeepseekModel != "deepseek-v4-pro" {
		t.Errorf("DeepseekModel = %q, want deepseek-v4-pro", cfg.DeepseekModel)
	}
	if cfg.DehydrateConcurrency != 10 {
		t.Errorf("DehydrateConcurrency = %d, want 10", cfg.DehydrateConcurrency)
	}
	if cfg.Port != "9999" {
		t.Errorf("Port = %q, want 9999", cfg.Port)
	}
	if cfg.AuthPassword != "secret123" {
		t.Errorf("AuthPassword = %q, want secret123", cfg.AuthPassword)
	}
	if cfg.DatabaseURL != "postgres://other:5432/mydb" {
		t.Errorf("DatabaseURL = %q, want postgres://other:5432/mydb", cfg.DatabaseURL)
	}

	// Unset fields should keep defaults
	if cfg.DeepseekAPIKey != "" {
		t.Errorf("DeepseekAPIKey = %q, want empty", cfg.DeepseekAPIKey)
	}
	if cfg.DeepseekBaseURL != "https://api.deepseek.com" {
		t.Errorf("DeepseekBaseURL = %q, want https://api.deepseek.com", cfg.DeepseekBaseURL)
	}
}

func TestDataDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DATA_DIR", tmpDir)

	cfg := Load()

	if cfg.DataDir != tmpDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, tmpDir)
	}

	// Verify directories were created
	uploadsDir := filepath.Join(tmpDir, "uploads")
	booksDir := filepath.Join(tmpDir, "books")

	for _, dir := range []string{uploadsDir, booksDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %s was not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", dir)
		}
	}
}
