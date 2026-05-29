package models

import (
	"encoding/json"
	"testing"
	"time"
)

// TestBookJSON verifies Book JSON tags match Python snake_case convention
func TestBookJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	errMsg := "parse error example"

	b := Book{
		ID:              1,
		Title:           "测试小说",
		Author:          "作者名",
		SourceFormat:    "txt",
		SourcePath:      "/data/book.txt",
		TotalChapters:   100,
		HasVolumes:      true,
		ParseStatus:     "done",
		ParseError:      &errMsg,
		DehydratedCount: 42,
		CreatedAt:       now,
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("json.Marshal(Book) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Book) failed: %v", err)
	}

	// Verify snake_case keys
	expectedKeys := []string{
		"id", "title", "author", "source_format", "source_path",
		"total_chapters", "has_volumes", "parse_status", "parse_error",
		"dehydrated_count", "created_at",
	}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("Book JSON missing key: %s", key)
		}
	}

	// Verify omitempty: parse_error should be present when not nil
	if result["parse_error"] != errMsg {
		t.Errorf("Book.parse_error: expected %q, got %v", errMsg, result["parse_error"])
	}
}

// TestBookJSONOmitEmpty verifies ParseError is omitted when nil
func TestBookJSONOmitEmpty(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	b := Book{
		ID:              1,
		Title:           "Test",
		Author:          "Author",
		SourceFormat:    "epub",
		SourcePath:      "/data/test.epub",
		TotalChapters:   10,
		HasVolumes:      false,
		ParseStatus:     "pending",
		ParseError:      nil,
		DehydratedCount: 0,
		CreatedAt:       now,
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("json.Marshal(Book) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Book) failed: %v", err)
	}

	// parse_error should be omitted when nil
	if _, ok := result["parse_error"]; ok {
		t.Error("Book.parse_error should be omitted when nil")
	}
}

// TestVolumeJSON verifies Volume JSON serialization
func TestVolumeJSON(t *testing.T) {
	v := Volume{
		ID:           1,
		BookID:       10,
		Title:        "第一卷 初入江湖",
		Seq:          1,
		DetectSource: nil,
		Chapters:     []Chapter{},
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(Volume) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Volume) failed: %v", err)
	}

	expectedKeys := []string{"id", "book_id", "title", "seq"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("Volume JSON missing key: %s", key)
		}
	}

	// detect_source should be omitted when nil
	if _, ok := result["detect_source"]; ok {
		t.Error("Volume.detect_source should be omitted when nil")
	}

	// chapters omitted when empty (omitempty)
	if _, ok := result["chapters"]; ok {
		t.Error("Volume.chapters should be omitted when empty")
	}
}

// TestVolumeJSONWithDetectSource verifies DetectSource serializes when set
func TestVolumeJSONWithDetectSource(t *testing.T) {
	ds := "auto-detected"
	v := Volume{
		ID:           1,
		BookID:       10,
		Title:        "第一卷",
		Seq:          1,
		DetectSource: &ds,
		Chapters:     nil,
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(Volume) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Volume) failed: %v", err)
	}

	if result["detect_source"] != ds {
		t.Errorf("Volume.detect_source: expected %q, got %v", ds, result["detect_source"])
	}

	// chapters should be omitted when nil (omitempty)
	if _, ok := result["chapters"]; ok {
		t.Error("Volume.chapters should be omitted when nil")
	}
}

// TestVolumeRoundTrip verifies JSON round-trip for Volume
func TestVolumeRoundTrip(t *testing.T) {
	v1 := Volume{
		ID:      2,
		BookID:  5,
		Title:   "第一卷",
		Seq:     1,
		Chapters: []Chapter{
			{
				ID:               1,
				BookID:           5,
				Title:            "第一章 山边小村",
				Seq:              1,
				RawPath:          "/data/ch1.txt",
				RawCharCount:     3200,
				DehydrateStatus:  "done",
				DehydratedCharCount: 800,
			},
		},
	}

	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("json.Marshal(Volume) failed: %v", err)
	}

	var v2 Volume
	if err := json.Unmarshal(data, &v2); err != nil {
		t.Fatalf("json.Unmarshal(Volume) failed: %v", err)
	}

	if v2.ID != v1.ID || v2.Title != v1.Title || v2.Seq != v1.Seq {
		t.Error("Volume round-trip mismatch")
	}
	if len(v2.Chapters) != 1 || v2.Chapters[0].Title != "第一章 山边小村" {
		t.Error("Volume chapter round-trip mismatch")
	}
}

// TestChapterJSON verifies Chapter JSON tags
func TestChapterJSON(t *testing.T) {
	processedAt := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	volumeID := int64(5)
	errMsg := "AI API timeout"
	dehydratedPath := "/data/dehydrated/ch1.txt"
	ratio := 0.75

	c := Chapter{
		ID:                  1,
		BookID:              10,
		VolumeID:            &volumeID,
		Title:               "第一章 山边小村",
		Seq:                 1,
		RawPath:             "/data/raw/ch1.txt",
		RawCharCount:        3200,
		DehydrateStatus:     "done",
		DehydratedPath:      &dehydratedPath,
		DehydratedCharCount: 800,
		CompressionRatio:    &ratio,
		ErrorMsg:            &errMsg,
		RetryCount:          0,
		ProcessedAt:         &processedAt,
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal(Chapter) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Chapter) failed: %v", err)
	}

	expectedKeys := []string{
		"id", "book_id", "volume_id", "title", "seq", "raw_path",
		"raw_char_count", "dehydrate_status", "dehydrated_path",
		"dehydrated_char_count", "compression_ratio", "error_msg",
		"retry_count", "processed_at",
	}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("Chapter JSON missing key: %s", key)
		}
	}
}

// TestChapterJSONOmitEmpty verifies nil pointers are omitted
func TestChapterJSONOmitEmpty(t *testing.T) {
	c := Chapter{
		ID:               1,
		BookID:           10,
		Title:            "第一章",
		Seq:              1,
		RawPath:          "/data/raw/ch1.txt",
		RawCharCount:     3200,
		DehydrateStatus:  "pending",
		DehydratedCharCount: 0,
		RetryCount:       0,
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal(Chapter) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Chapter) failed: %v", err)
	}

	// These should all be omitted when nil
	omitKeys := []string{"volume_id", "dehydrated_path", "compression_ratio", "error_msg", "processed_at"}
	for _, key := range omitKeys {
		if _, ok := result[key]; ok {
			t.Errorf("Chapter.%s should be omitted when nil", key)
		}
	}
}

// TestChapterRoundTrip verifies JSON round-trip
func TestChapterRoundTrip(t *testing.T) {
	c1 := Chapter{
		ID:               1,
		BookID:           10,
		Title:            "第一章",
		Seq:              1,
		RawPath:          "/path",
		RawCharCount:     100,
		DehydrateStatus:  "done",
		DehydratedCharCount: 40,
		RetryCount:       0,
	}

	data, err := json.Marshal(c1)
	if err != nil {
		t.Fatalf("json.Marshal(Chapter) failed: %v", err)
	}

	var c2 Chapter
	if err := json.Unmarshal(data, &c2); err != nil {
		t.Fatalf("json.Unmarshal(Chapter) failed: %v", err)
	}

	if c2.Title != "第一章" || c2.DehydrateStatus != "done" {
		t.Error("Chapter round-trip mismatch")
	}
}

// TestJobJSON verifies Job JSON serialization
func TestJobJSON(t *testing.T) {
	now := time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC)
	currentCh := int64(42)

	j := Job{
		ID:               1,
		BookID:           10,
		Status:           "running",
		ScopeType:        "volumes",
		TotalCount:       100,
		DoneCount:        41,
		FailedCount:      2,
		CurrentChapterID: &currentCh,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("json.Marshal(Job) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(Job) failed: %v", err)
	}

	expectedKeys := []string{
		"id", "book_id", "status", "scope_type", "total_count",
		"done_count", "failed_count", "current_chapter_id",
		"created_at", "updated_at",
	}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("Job JSON missing key: %s", key)
		}
	}
}

// TestAppSettingsJSON verifies AppSettings JSON serialization
func TestAppSettingsJSON(t *testing.T) {
	s := AppSettings{
		DeepseekAPIKey:  "sk-test-key",
		DeepseekModel:   "deepseek-v4-flash",
		DeepseekBaseURL: "https://api.deepseek.com",
		Concurrency:     5,
		SystemPrompt:    "You are a helpful assistant.",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(AppSettings) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(AppSettings) failed: %v", err)
	}

	expectedKeys := []string{
		"deepseek_api_key", "deepseek_model", "deepseek_base_url",
		"concurrency", "system_prompt",
	}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("AppSettings JSON missing key: %s", key)
		}
	}

	if result["deepseek_api_key"] != "sk-test-key" {
		t.Errorf("deepseek_api_key mismatch: got %v", result["deepseek_api_key"])
	}
}

// TestLoginRequestJSON verifies LoginRequest serialization
func TestLoginRequestJSON(t *testing.T) {
	req := LoginRequest{Password: "secret123"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal(LoginRequest) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(LoginRequest) failed: %v", err)
	}

	if result["password"] != "secret123" {
		t.Errorf("LoginRequest password: expected %q, got %v", "secret123", result["password"])
	}
}

// TestStartJobRequestJSON verifies StartJobRequest serialization
func TestStartJobRequestJSON(t *testing.T) {
	req := StartJobRequest{
		ScopeType:  "volumes",
		VolumeIDs:  []int64{1, 2, 3},
		ChapterIDs: nil,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal(StartJobRequest) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(StartJobRequest) failed: %v", err)
	}

	if result["scope_type"] != "volumes" {
		t.Errorf("scope_type mismatch: got %v", result["scope_type"])
	}

	// chapter_ids should be omitted when nil/empty
	if _, ok := result["chapter_ids"]; ok {
		t.Error("StartJobRequest.chapter_ids should be omitted when nil")
	}
}

// TestEstimateRequestJSON verifies EstimateRequest serialization
func TestEstimateRequestJSON(t *testing.T) {
	req := EstimateRequest{ChapterIDs: []int64{10, 20, 30}}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal(EstimateRequest) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(EstimateRequest) failed: %v", err)
	}

	if _, ok := result["chapter_ids"]; !ok {
		t.Error("EstimateRequest JSON missing key: chapter_ids")
	}
}

// TestSettingsInJSON verifies SettingsIn serialization
func TestSettingsInJSON(t *testing.T) {
	s := SettingsIn{
		DeepseekAPIKey:  "sk-new-key",
		DeepseekModel:   "deepseek-v4-pro",
		DeepseekBaseURL: "https://api.deepseek.com",
		Concurrency:     20,
		SystemPrompt:    "你是网文脱水助手。",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(SettingsIn) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(SettingsIn) failed: %v", err)
	}

	if result["deepseek_model"] != "deepseek-v4-pro" {
		t.Errorf("deepseek_model mismatch: got %v", result["deepseek_model"])
	}
}

// TestSettingsOutJSON verifies SettingsOut serialization
func TestSettingsOutJSON(t *testing.T) {
	s := SettingsOut{
		DeepseekAPIKey:   "****",
		APIKeyConfigured: true,
		DeepseekModel:    "deepseek-v4-flash",
		DeepseekBaseURL:  "https://api.deepseek.com",
		Concurrency:      5,
		SystemPrompt:     "",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(SettingsOut) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(SettingsOut) failed: %v", err)
	}

	expectedKeys := []string{
		"deepseek_api_key", "api_key_configured", "deepseek_model",
		"deepseek_base_url", "concurrency", "system_prompt",
	}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("SettingsOut JSON missing key: %s", key)
		}
	}

	apiConfigured, ok := result["api_key_configured"].(bool)
	if !ok || !apiConfigured {
		t.Errorf("api_key_configured should be true, got %v", result["api_key_configured"])
	}
}

// TestBookStructureOutJSON verifies BookStructureOut serialization
func TestBookStructureOutJSON(t *testing.T) {
	bso := BookStructureOut{
		HasVolumes: true,
		Volumes: []Volume{
			{ID: 1, BookID: 10, Title: "第一卷", Seq: 1},
		},
		LooseChapters: []Chapter{
			{ID: 5, BookID: 10, Title: "序章", Seq: 0},
		},
	}

	data, err := json.Marshal(bso)
	if err != nil {
		t.Fatalf("json.Marshal(BookStructureOut) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(BookStructureOut) failed: %v", err)
	}

	expectedKeys := []string{"has_volumes", "volumes", "loose_chapters"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("BookStructureOut JSON missing key: %s", key)
		}
	}
}

// TestChatRequestJSON verifies ChatRequest serialization
func TestChatRequestJSON(t *testing.T) {
	req := ChatRequest{
		ChapterID: 42,
		Question:  "这个角色的动机是什么？",
		History: []ChatMessage{
			{Role: "user", Content: "介绍一下主角"},
			{Role: "assistant", Content: "主角是韩立，一个普通少年。"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal(ChatRequest) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(ChatRequest) failed: %v", err)
	}

	if result["chapter_id"].(float64) != 42 {
		t.Errorf("chapter_id mismatch: got %v", result["chapter_id"])
	}

	history, ok := result["history"].([]interface{})
	if !ok || len(history) != 2 {
		t.Fatalf("ChatRequest.history should have 2 items, got %v", result["history"])
	}
}

// TestChatMessageJSON verifies ChatMessage serialization
func TestChatMessageJSON(t *testing.T) {
	msg := ChatMessage{Role: "user", Content: "Hello"}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal(ChatMessage) failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(ChatMessage) failed: %v", err)
	}

	if result["role"] != "user" {
		t.Errorf("role mismatch: got %v", result["role"])
	}
	if result["content"] != "Hello" {
		t.Errorf("content mismatch: got %v", result["content"])
	}
}

// TestTimeMarshaling verifies time.Time serializes as RFC3339
func TestTimeMarshaling(t *testing.T) {
	now := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)
	b := Book{
		ID:        1,
		Title:     "Test",
		Author:    "Author",
		SourceFormat: "txt",
		SourcePath: "/path",
		ParseStatus: "done",
		CreatedAt: now,
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	createdAt, ok := result["created_at"].(string)
	if !ok {
		t.Fatalf("created_at should be a string, got %T", result["created_at"])
	}

	// time.Time marshals to RFC3339 format
	expected := "2025-06-15T14:30:00Z"
	if createdAt != expected {
		t.Errorf("created_at: expected %q, got %q", expected, createdAt)
	}
}
