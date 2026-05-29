package exporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripMeta(t *testing.T) {
	text := "正文第一段\n正文第二段\n---CHAPTER_META---\n本章剧情：xxx"
	got := StripMeta(text)
	if got != "正文第一段\n正文第二段" {
		t.Fatalf("StripMeta() = %q", got)
	}

	plain := "只有正文"
	if StripMeta(plain) != plain {
		t.Fatalf("StripMeta() should keep plain text")
	}
}

func TestBuildTXTFormat(t *testing.T) {
	dataDir := t.TempDir()
	path1 := filepath.Join(dataDir, "chapter1.txt")
	path2 := filepath.Join(dataDir, "chapter2.txt")
	if err := os.WriteFile(path1, []byte("正文一\n---CHAPTER_META---\n附注"), 0o644); err != nil {
		t.Fatalf("WriteFile(path1) error = %v", err)
	}
	if err := os.WriteFile(path2, []byte("正文二"), 0o644); err != nil {
		t.Fatalf("WriteFile(path2) error = %v", err)
	}

	chapters := []chapterRow{
		{ID: 1, Title: "第一章", RawPath: path1},
		{ID: 2, Title: "第二章", RawPath: path2},
	}

	got, err := buildTXT("测试书", chapters, dataDir)
	if err != nil {
		t.Fatalf("buildTXT() error = %v", err)
	}

	text := string(got)
	checks := []string{"《测试书》脱水版", "第一章", "正文一", "第二章", "正文二"}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("buildTXT() output missing %q: %q", check, text)
		}
	}
	if strings.Contains(text, "附注") || strings.Contains(text, chapterMetaDelimiter) {
		t.Fatalf("buildTXT() should strip meta section: %q", text)
	}
}
