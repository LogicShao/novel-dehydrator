package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func RawChapterPath(dataDir string, bookID, chapterID int64) string {
	return filepath.Join(dataDir, "books", strconv.FormatInt(bookID, 10), "raw", fmt.Sprintf("%d.txt", chapterID))
}

func DehydratedChapterPath(dataDir string, bookID, chapterID int64) string {
	return filepath.Join(dataDir, "books", strconv.FormatInt(bookID, 10), "dehydrated", fmt.Sprintf("%d.txt", chapterID))
}

func WriteRawChapter(dataDir string, bookID, chapterID int64, content string) (string, error) {
	path := RawChapterPath(dataDir, bookID, chapterID)
	if err := writeFile(path, content); err != nil {
		return "", err
	}
	return path, nil
}

func WriteDehydratedChapter(path, content string) error {
	return writeFile(path, content)
}

func ReadChapter(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DeleteBookFiles(dataDir string, bookID int64) error {
	bookDir := filepath.Join(dataDir, "books", strconv.FormatInt(bookID, 10))
	err := os.RemoveAll(bookDir)
	if err != nil {
		return fmt.Errorf("storage: delete book files: %w", err)
	}
	return nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("storage: mkdir parent: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("storage: write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("storage: rename temp file: %w", err)
	}
	return nil
}
