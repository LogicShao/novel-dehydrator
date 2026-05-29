package dehydrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/services/deepseek"
	"github.com/LogicShao/novel-dehydrator/internal/services/prompts"
)

const chapterMetaDelimiter = "---CHAPTER_META---"

type Dehydrator struct {
	pool           *pgxpool.Pool
	client         deepseek.ChatCompleter
	chunkCharLimit int
}

func New(pool *pgxpool.Pool, client deepseek.ChatCompleter, chunkCharLimit int) *Dehydrator {
	if chunkCharLimit <= 0 {
		chunkCharLimit = 12000
	}

	return &Dehydrator{
		pool:           pool,
		client:         client,
		chunkCharLimit: chunkCharLimit,
	}
}

func (d *Dehydrator) DehydrateChapter(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error) {
	if d == nil {
		return "", fmt.Errorf("dehydrator: nil receiver")
	}
	if d.client == nil {
		return "", fmt.Errorf("dehydrator: nil deepseek client")
	}

	chunks := splitIntoChunks(rawText, d.chunkCharLimit)
	systemPrompt := d.getSystemPrompt(ctx)
	positionHint := prompts.PositionHintNormal
	if totalChapters > 0 && float64(chapterSeq)/float64(totalChapters) <= 0.2 {
		positionHint = prompts.PositionHintEarly
	}

	results := make([]string, 0, len(chunks))
	metaSections := make([]string, 0, len(chunks))
	for idx, chunk := range chunks {
		chunkHint := ""
		if len(chunks) > 1 {
			chunkHint = fmt.Sprintf(prompts.ChunkHint, len(chunks), idx+1)
		}

		userContent := fmt.Sprintf(
			prompts.DehydrateUser,
			bookTitle,
			chapterTitle,
			chapterSeq,
			totalChapters,
			positionHint,
			chunkHint,
			chunk,
		)

		messages := []deepseek.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		}

		result, err := d.client.ChatCompletion(ctx, messages, true)
		if err != nil {
			return "", fmt.Errorf("dehydrator: chat completion for chunk %d/%d: %w", idx+1, len(chunks), err)
		}

		body, meta := splitBodyAndMeta(result)
		results = append(results, body)
		if meta != "" {
			metaSections = append(metaSections, meta)
		}
	}

	return finalizeChapterOutput(strings.Join(results, "\n\n"), metaSections), nil
}

func splitIntoChunks(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	separator := "\n\n"
	paragraphs := strings.Split(text, separator)
	if !strings.Contains(text, separator) {
		separator = "\n"
		paragraphs = strings.Split(text, separator)
	}

	chunks := make([]string, 0)
	current := make([]string, 0)
	currentLen := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.Join(current, separator))
		current = current[:0]
		currentLen = 0
	}

	for _, para := range paragraphs {
		if currentLen+len(para) > limit && len(current) > 0 {
			flush()
		}
		current = append(current, para)
		currentLen += len(para)
	}
	flush()

	if len(chunks) == 0 {
		return []string{text}
	}

	return chunks
}

func (d *Dehydrator) getSystemPrompt(ctx context.Context) string {
	if d.pool == nil {
		return prompts.DehydrateSystem
	}

	var systemPrompt string
	err := d.pool.QueryRow(ctx, "SELECT system_prompt FROM app_settings WHERE id=1").Scan(&systemPrompt)
	if err != nil || strings.TrimSpace(systemPrompt) == "" {
		return prompts.DehydrateSystem
	}

	return strings.TrimSpace(systemPrompt)
}

func splitBodyAndMeta(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	idx := strings.Index(trimmed, chapterMetaDelimiter)
	if idx == -1 {
		return trimmed, ""
	}
	body := strings.TrimSpace(trimmed[:idx])
	meta := strings.TrimSpace(trimmed[idx+len(chapterMetaDelimiter):])
	return body, meta
}

func finalizeChapterOutput(body string, metaSections []string) string {
	trimmedBody := strings.TrimSpace(body)
	if len(metaSections) == 0 {
		return trimmedBody + "\n" + chapterMetaDelimiter + "\n"
	}
	meta := strings.TrimSpace(metaSections[len(metaSections)-1])
	if meta == "" {
		return trimmedBody + "\n" + chapterMetaDelimiter + "\n"
	}
	return trimmedBody + "\n" + chapterMetaDelimiter + "\n" + meta + "\n" + chapterMetaDelimiter + "\n"
}
