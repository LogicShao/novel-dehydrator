package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var (
	chapterLineRe   = regexp.MustCompile(`^[\s　]*第[零一二三四五六七八九十百千万\d]+[章节回集].*$`)
	chapterPrefixRe = regexp.MustCompile(`^[\s　]*(第[零一二三四五六七八九十百千万\d]+[章节回集])`)
	volumeLineRe    = regexp.MustCompile(`^[\s　]*第[零一二三四五六七八九十百千万\d]+[篇卷部册].*$`)
)

type ParseResult struct {
	Title        string
	Author       string
	Chapters     []RawChapter
	HasNestedTOC bool
}

func ParseTXT(filePath string) (*ParseResult, error) {
	content, err := detectAndReadTXT(filePath)
	if err != nil {
		return nil, err
	}

	result := &ParseResult{
		Title:        strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)),
		Author:       "",
		HasNestedTOC: false,
	}

	lines := splitLines(content)
	chapters := make([]RawChapter, 0)
	currentVolumeTitle := ""
	currentVolumeSeq := 0
	currentTitle := ""
	currentLines := make([]string, 0)

	flushChapter := func() {
		if currentTitle == "" {
			return
		}
		chapters = append(chapters, RawChapter{
			Title:       currentTitle,
			Content:     strings.TrimSpace(strings.Join(currentLines, "\n")),
			VolumeSeq:   currentVolumeSeq,
			VolumeTitle: currentVolumeTitle,
		})
	}

	for i, line := range lines {
		stripped := strings.TrimSpace(line)

		if isVolumeLine(stripped) && !isChapterLine(stripped) {
			flushChapter()
			currentVolumeSeq++
			currentVolumeTitle = stripped
			currentTitle = ""
			currentLines = currentLines[:0]
			continue
		}

		if isChapterLine(stripped) {
			flushChapter()
			currentTitle = stripped
			currentLines = currentLines[:0]
			continue
		}

		if currentTitle == "" {
			nextLine := nextNonEmptyLine(lines, i+1)
			if title, inlineContent, ok := extractInlineChapter(stripped, nextLine); ok {
				flushChapter()
				currentTitle = title
				currentLines = currentLines[:0]
				if inlineContent != "" {
					currentLines = append(currentLines, inlineContent)
				}
				continue
			}
		}

		if currentTitle != "" {
			currentLines = append(currentLines, line)
		}
	}

	flushChapter()

	if len(chapters) == 0 {
		chapters = append(chapters, RawChapter{
			Title:       "全文",
			Content:     strings.TrimSpace(content),
			VolumeSeq:   0,
			VolumeTitle: "",
		})
	}

	result.Chapters = chapters
	return result, nil
}

func detectAndReadTXT(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read txt %s: %w", filePath, err)
	}

	if utf8.Valid(data) {
		return strings.TrimPrefix(string(data), "\ufeff"), nil
	}

	decoders := []transform.Transformer{
		simplifiedchinese.GBK.NewDecoder(),
		simplifiedchinese.GB18030.NewDecoder(),
	}

	for _, decoder := range decoders {
		decoded, _, decodeErr := transform.Bytes(decoder, data)
		if decodeErr == nil {
			return strings.TrimPrefix(string(decoded), "\ufeff"), nil
		}
	}

	return "", fmt.Errorf("unable to decode txt %s as UTF-8/GBK/GB18030", filePath)
}

func splitLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func isChapterLine(line string) bool {
	if line == "" {
		return false
	}
	if utf8.RuneCountInString(line) > 40 {
		return false
	}
	return chapterLineRe.MatchString(line)
}

func isVolumeLine(line string) bool {
	if line == "" || len([]rune(line)) > 20 {
		return false
	}
	return volumeLineRe.MatchString(line)
}

func nextNonEmptyLine(lines []string, start int) string {
	for i := start; i < len(lines); i++ {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func extractInlineChapter(line, nextLine string) (title string, inlineContent string, ok bool) {
	if line == "" {
		return "", "", false
	}

	matches := chapterPrefixRe.FindStringSubmatch(line)
	if len(matches) < 2 {
		return "", "", false
	}

	matchedPrefix := matches[1]
	remainder := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), matchedPrefix))
	if remainder == "" {
		return strings.TrimSpace(line), "", true
	}

	if nextLine != "" {
		if idx := strings.Index(remainder, nextLine); idx >= 0 {
			candidate := strings.TrimSpace(remainder[:idx])
			if candidate != "" && utf8.RuneCountInString(candidate) <= 20 {
				return matchedPrefix + candidate, "", true
			}
		}
	}

	return "", "", false
}
