package storage

import (
	"os"
	"strings"
	"testing"
)

func TestRawChapterPath(t *testing.T) {
	dataDir := "/data/novels"
	path := RawChapterPath(dataDir, 42, 7)
	expected := "/data/novels/books/42/raw/7.txt"
	if path != expected {
		t.Errorf("RawChapterPath = %q, want %q", path, expected)
	}
}

func TestDehydratedChapterPath(t *testing.T) {
	dataDir := "/data/novels"
	path := DehydratedChapterPath(dataDir, 42, 7)
	expected := "/data/novels/books/42/dehydrated/7.txt"
	if path != expected {
		t.Errorf("DehydratedChapterPath = %q, want %q", path, expected)
	}
}

func TestPathFormats(t *testing.T) {
	tests := []struct {
		name      string
		fn        func(string, int64, int64) string
		dataDir   string
		bookID    int64
		chapterID int64
		expected  string
	}{
		{
			name:      "raw path with trailing slash in dataDir",
			fn:        RawChapterPath,
			dataDir:   "/data/novels/",
			bookID:    100,
			chapterID: 200,
			expected:  "/data/novels/books/100/raw/200.txt",
		},
		{
			name:      "dehydrated path relative dataDir",
			fn:        DehydratedChapterPath,
			dataDir:   "data",
			bookID:    1,
			chapterID: 99,
			expected:  "data/books/1/dehydrated/99.txt",
		},
		{
			name:      "raw path large IDs",
			fn:        RawChapterPath,
			dataDir:   "/mnt/books",
			bookID:    999999,
			chapterID: 888888,
			expected:  "/mnt/books/books/999999/raw/888888.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.dataDir, tt.bookID, tt.chapterID)
			if got != tt.expected {
				t.Errorf("%s(%q, %d, %d) = %q, want %q",
					tt.name, tt.dataDir, tt.bookID, tt.chapterID, got, tt.expected)
			}
		})
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	content := "这是一章测试内容，包含中文、标点和特殊字符：\n第二行内容\n\n第三行。"
	path, err := WriteRawChapter(tmpDir, 1, 1, content)
	if err != nil {
		t.Fatalf("WriteRawChapter failed: %v", err)
	}

	read, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter failed: %v", err)
	}

	if read != content {
		t.Errorf("round-trip mismatch:\ngot  = %q\nwant = %q", read, content)
	}
}

func TestAtomicWriteNormal(t *testing.T) {
	tmpDir := t.TempDir()
	path := DehydratedChapterPath(tmpDir, 1, 1)
	content := "脱水后的内容"

	err := WriteDehydratedChapter(path, content)
	if err != nil {
		t.Fatalf("WriteDehydratedChapter failed: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error(".tmp file should not exist after atomic write")
	}

	read, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter failed: %v", err)
	}
	if read != content {
		t.Errorf("content mismatch: got %q, want %q", read, content)
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := DehydratedChapterPath(tmpDir, 1, 1)

	if err := WriteDehydratedChapter(path, "first version"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	if err := WriteDehydratedChapter(path, "second version"); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	read, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter failed: %v", err)
	}
	if read != "second version" {
		t.Errorf("overwrite failed: got %q, want %q", read, "second version")
	}
}

func TestAtomicWriteSimulateCrash(t *testing.T) {
	tmpDir := t.TempDir()
	path := DehydratedChapterPath(tmpDir, 1, 1)

	if err := WriteDehydratedChapter(path, "original content"); err != nil {
		t.Fatalf("original write failed: %v", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte("partial corrupt data"), 0644); err != nil {
		t.Fatalf("failed to create .tmp: %v", err)
	}

	read, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter failed after simulated crash: %v", err)
	}
	if read != "original content" {
		t.Errorf("original content corrupted after simulated crash: got %q", read)
	}

	if err := WriteDehydratedChapter(path, "new proper content"); err != nil {
		t.Fatalf("recovery write failed: %v", err)
	}

	read2, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter after recovery failed: %v", err)
	}
	if read2 != "new proper content" {
		t.Errorf("recovery write content mismatch: got %q, want %q", read2, "new proper content")
	}
}

func TestDeleteBookFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some chapter files
	if _, err := WriteRawChapter(tmpDir, 1, 1, "raw chapter 1"); err != nil {
		t.Fatalf("WriteRawChapter 1: %v", err)
	}
	if _, err := WriteRawChapter(tmpDir, 1, 2, "raw chapter 2"); err != nil {
		t.Fatalf("WriteRawChapter 2: %v", err)
	}
	path := DehydratedChapterPath(tmpDir, 1, 3)
	if err := WriteDehydratedChapter(path, "dehydrated chapter 3"); err != nil {
		t.Fatalf("WriteDehydratedChapter: %v", err)
	}

	// Verify files exist
	bookDir := RawChapterPath(tmpDir, 1, 0)
	bookPath := strings.TrimSuffix(bookDir, "/raw/0.txt")
	if _, err := os.Stat(bookPath); os.IsNotExist(err) {
		t.Fatalf("book dir should exist before delete: %s", bookPath)
	}

	if err := DeleteBookFiles(tmpDir, 1); err != nil {
		t.Fatalf("DeleteBookFiles failed: %v", err)
	}

	bookPathCheck := strings.TrimSuffix(RawChapterPath(tmpDir, 1, 0), "/raw/0.txt")
	if _, err := os.Stat(bookPathCheck); !os.IsNotExist(err) {
		t.Error("book directory should not exist after delete")
	}
}

func TestDeleteNonExistentBook(t *testing.T) {
	tmpDir := t.TempDir()
	// Deleting a non-existent book should not error
	err := DeleteBookFiles(tmpDir, 99999)
	if err != nil {
		t.Errorf("DeleteBookFiles for non-existent book should not error: %v", err)
	}
}

func TestWriteRawChapterCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	nested := tmpDir + "/deeply/nested/data"
	path, err := WriteRawChapter(nested, 5, 10, "content")
	if err != nil {
		t.Fatalf("WriteRawChapter with deeply nested dirs failed: %v", err)
	}
	read, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter failed: %v", err)
	}
	if read != "content" {
		t.Errorf("content mismatch: %q", read)
	}
}

func TestReadChapterNotFound(t *testing.T) {
	_, err := ReadChapter("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("ReadChapter should return error for non-existent file")
	}
}

func TestReadChapterEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/empty.txt"
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}
	content, err := ReadChapter(path)
	if err != nil {
		t.Fatalf("ReadChapter on empty file failed: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string, got %q", content)
	}
}
