package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTXTChapters(t *testing.T) {
	result, err := ParseTXT("/mnt/d/proj/novel-dehydrator/examples/fanren-xiuxian/raw/1455.txt")
	if err != nil {
		t.Fatalf("ParseTXT() error = %v", err)
	}

	if result.Title != "1455" {
		t.Fatalf("result.Title = %q, want %q", result.Title, "1455")
	}

	if len(result.Chapters) != 1 {
		t.Fatalf("len(result.Chapters) = %d, want 1", len(result.Chapters))
	}

	chapter := result.Chapters[0]
	if chapter.Title != "第一章山边小村" {
		t.Fatalf("chapter.Title = %q, want %q", chapter.Title, "第一章山边小村")
	}

	if chapter.VolumeSeq != 0 {
		t.Fatalf("chapter.VolumeSeq = %d, want 0", chapter.VolumeSeq)
	}

	if chapter.VolumeTitle != "" {
		t.Fatalf("chapter.VolumeTitle = %q, want empty", chapter.VolumeTitle)
	}

	if chapter.Content == "" {
		t.Fatal("chapter.Content is empty")
	}

	if want := "二愣子睁大着双眼"; !contains(chapter.Content, want) {
		t.Fatalf("chapter.Content does not contain %q", want)
	}
	if gotPrefix := chapter.Content[:len("二愣子睁大着双眼")]; gotPrefix != "二愣子睁大着双眼" {
		t.Fatalf("chapter.Content prefix = %q, want %q", gotPrefix, "二愣子睁大着双眼")
	}
}

func TestParseTXTVolumeDetection(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "volume.txt")
	content := "第一卷 初入江湖\n第一章 山边小村\n韩立出生在一个小山村。\n第二章 青牛镇\n韩立跟随三叔前往七玄门。\n"

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	result, err := ParseTXT(filePath)
	if err != nil {
		t.Fatalf("ParseTXT() error = %v", err)
	}

	if len(result.Chapters) != 2 {
		t.Fatalf("len(result.Chapters) = %d, want 2", len(result.Chapters))
	}

	for i, chapter := range result.Chapters {
		if chapter.VolumeSeq != 1 {
			t.Fatalf("chapter[%d].VolumeSeq = %d, want 1", i, chapter.VolumeSeq)
		}
		if chapter.VolumeTitle != "第一卷 初入江湖" {
			t.Fatalf("chapter[%d].VolumeTitle = %q, want %q", i, chapter.VolumeTitle, "第一卷 初入江湖")
		}
	}

	if result.Chapters[0].Title != "第一章 山边小村" {
		t.Fatalf("chapter[0].Title = %q, want %q", result.Chapters[0].Title, "第一章 山边小村")
	}
	if result.Chapters[1].Title != "第二章 青牛镇" {
		t.Fatalf("chapter[1].Title = %q, want %q", result.Chapters[1].Title, "第二章 青牛镇")
	}
}

func TestCharsetDetection(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "utf8.txt")
	content := "第一章 测试标题\n这里是 UTF-8 正文。\n"

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	result, err := ParseTXT(filePath)
	if err != nil {
		t.Fatalf("ParseTXT() error = %v", err)
	}

	if len(result.Chapters) != 1 {
		t.Fatalf("len(result.Chapters) = %d, want 1", len(result.Chapters))
	}

	if result.Chapters[0].Title != "第一章 测试标题" {
		t.Fatalf("chapter.Title = %q, want %q", result.Chapters[0].Title, "第一章 测试标题")
	}

	if result.Chapters[0].Content != "这里是 UTF-8 正文。" {
		t.Fatalf("chapter.Content = %q, want %q", result.Chapters[0].Content, "这里是 UTF-8 正文。")
	}
	if result.HasNestedTOC {
		t.Fatal("result.HasNestedTOC = true, want false")
	}
}

func contains(s, substr string) bool {
	return len(substr) == 0 || strings.HasPrefix(s, substr) || strings.Contains(s, substr)
}
