package models

import "time"

// ── Database models ──

type Book struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	SourceFormat    string    `json:"source_format"`
	SourcePath      string    `json:"source_path"`
	TotalChapters   int       `json:"total_chapters"`
	HasVolumes      bool      `json:"has_volumes"`
	ParseStatus     string    `json:"parse_status"`
	ParseError      *string   `json:"parse_error,omitempty"`
	DehydratedCount int       `json:"dehydrated_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type Volume struct {
	ID           int64     `json:"id"`
	BookID       int64     `json:"book_id"`
	Title        string    `json:"title"`
	Seq          int       `json:"seq"`
	DetectSource *string   `json:"detect_source,omitempty"`
	Chapters     []Chapter `json:"chapters,omitempty"`
}

type Chapter struct {
	ID                  int64      `json:"id"`
	BookID              int64      `json:"book_id"`
	VolumeID            *int64     `json:"volume_id,omitempty"`
	Title               string     `json:"title"`
	Seq                 int        `json:"seq"`
	RawPath             string     `json:"raw_path"`
	RawCharCount        int        `json:"raw_char_count"`
	DehydrateStatus     string     `json:"dehydrate_status"`
	DehydratedPath      *string    `json:"dehydrated_path,omitempty"`
	DehydratedCharCount int        `json:"dehydrated_char_count"`
	CompressionRatio    *float64   `json:"compression_ratio,omitempty"`
	ErrorMsg            *string    `json:"error_msg,omitempty"`
	RetryCount          int        `json:"retry_count"`
	ProcessedAt         *time.Time `json:"processed_at,omitempty"`
}

type Job struct {
	ID               int64     `json:"id"`
	BookID           int64     `json:"book_id"`
	Status           string    `json:"status"`
	ScopeType        string    `json:"scope_type"`
	TotalCount       int       `json:"total_count"`
	DoneCount        int       `json:"done_count"`
	FailedCount      int       `json:"failed_count"`
	CurrentChapterID *int64    `json:"current_chapter_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AppSettings struct {
	DeepseekAPIKey  string `json:"deepseek_api_key"`
	DeepseekModel   string `json:"deepseek_model"`
	DeepseekBaseURL string `json:"deepseek_base_url"`
	Concurrency     int    `json:"concurrency"`
	SystemPrompt    string `json:"system_prompt"`
}

// ── API request/response models ──

type LoginRequest struct {
	Password string `json:"password"`
}

type StartJobRequest struct {
	ScopeType  string  `json:"scope_type"`
	VolumeIDs  []int64 `json:"volume_ids,omitempty"`
	ChapterIDs []int64 `json:"chapter_ids,omitempty"`
}

type EstimateRequest struct {
	ChapterIDs []int64 `json:"chapter_ids"`
}

type SettingsIn struct {
	DeepseekAPIKey  string `json:"deepseek_api_key"`
	DeepseekModel   string `json:"deepseek_model"`
	DeepseekBaseURL string `json:"deepseek_base_url"`
	Concurrency     int    `json:"concurrency"`
	SystemPrompt    string `json:"system_prompt"`
}

type SettingsOut struct {
	DeepseekAPIKey   string `json:"deepseek_api_key"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	DeepseekModel    string `json:"deepseek_model"`
	DeepseekBaseURL  string `json:"deepseek_base_url"`
	Concurrency      int    `json:"concurrency"`
	SystemPrompt     string `json:"system_prompt"`
}

type BookStructureOut struct {
	HasVolumes    bool      `json:"has_volumes"`
	Volumes       []Volume  `json:"volumes"`
	LooseChapters []Chapter `json:"loose_chapters"`
}

type ChatRequest struct {
	ChapterID int64         `json:"chapter_id"`
	Question  string        `json:"question"`
	History   []ChatMessage `json:"history"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Folder struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	BookCount int       `json:"book_count"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateFolderRequest struct {
	Name string `json:"name"`
}

type AddBooksToFolderRequest struct {
	BookIDs []int64 `json:"book_ids"`
}
