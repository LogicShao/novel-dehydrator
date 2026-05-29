package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DeepseekAPIKey       string
	DeepseekModel        string
	DeepseekBaseURL      string
	DehydrateConcurrency int
	MaxRetries           int
	ChunkCharLimit       int
	AuthPassword         string
	DatabaseURL          string
	Port                 string
	DataDir              string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("config: .env not loaded: %v", err)
	}

	cfg := &Config{
		DeepseekAPIKey:       getEnv("DEEPSEEK_API_KEY", ""),
		DeepseekModel:        getEnv("DEEPSEEK_MODEL", "deepseek-v4-flash"),
		DeepseekBaseURL:      getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		DehydrateConcurrency: getEnvInt("DEHYDRATE_CONCURRENCY", 20),
		MaxRetries:           getEnvInt("MAX_RETRIES", 3),
		ChunkCharLimit:       getEnvInt("CHUNK_CHAR_LIMIT", 12000),
		AuthPassword:         getEnv("AUTH_PASSWORD", ""),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://localhost:5432/novel_dehydrator"),
		Port:                 getEnv("PORT", "8765"),
		DataDir:              getEnv("DATA_DIR", "data"),
	}

	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0755); err != nil {
		log.Printf("config: mkdir %s/uploads: %v", cfg.DataDir, err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "books"), 0755); err != nil {
		log.Printf("config: mkdir %s/books: %v", cfg.DataDir, err)
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}
