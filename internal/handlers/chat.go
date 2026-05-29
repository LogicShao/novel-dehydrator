package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/models"
	"github.com/LogicShao/novel-dehydrator/internal/services/deepseek"
	"github.com/LogicShao/novel-dehydrator/internal/storage"
)

type chatStore interface {
	GetChatContext(ctx context.Context, bookID, chapterID int64) (bookTitle, chapterTitle, rawPath string, dehydratedPath *string, status string, err error)
}

type chatCompleter interface {
	ChatCompletion(ctx context.Context, messages []deepseek.Message, stream bool) (string, error)
}

type postgresChatStore struct{ pool *pgxpool.Pool }

func HandleChapterChat(pool *pgxpool.Pool, client *deepseek.Client) http.HandlerFunc {
	return handleChapterChat(&postgresChatStore{pool: pool}, client, storage.ReadChapter)
}

func handleChapterChat(store chatStore, client chatCompleter, readChapter func(string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}

		var req models.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request")
			return
		}

		bookTitle, chapterTitle, rawPath, dehydratedPath, status, err := store.GetChatContext(r.Context(), bookID, req.ChapterID)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeJSONError(w, http.StatusNotFound, "章节不存在")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "查询章节失败")
			return
		}

		path := rawPath
		if status == "done" && dehydratedPath != nil && *dehydratedPath != "" {
			path = *dehydratedPath
		}
		chapterText, err := readChapter(path)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "章节内容读取失败")
			return
		}

		messages := make([]deepseek.Message, 0, len(req.History)+2)
		messages = append(messages, deepseek.Message{
			Role:    "system",
			Content: fmt.Sprintf("书名：《%s》\n章节：%s\n\n章节内容：\n%s", bookTitle, chapterTitle, chapterText),
		})
		for _, item := range req.History {
			messages = append(messages, deepseek.Message{Role: item.Role, Content: item.Content})
		}
		messages = append(messages, deepseek.Message{Role: "user", Content: req.Question})

		answer, err := client.ChatCompletion(r.Context(), messages, false)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("AI 调用失败：%s", err.Error()))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"answer": answer})
	}
}

func (s *postgresChatStore) GetChatContext(ctx context.Context, bookID, chapterID int64) (string, string, string, *string, string, error) {
	var (
		bookTitle      string
		chapterTitle   string
		rawPath        string
		dehydratedPath *string
		status         string
	)
	err := s.pool.QueryRow(ctx, `SELECT b.title, c.title, c.raw_path, c.dehydrated_path, c.dehydrate_status
		FROM books b JOIN chapters c ON c.book_id = b.id
		WHERE b.id=$1 AND c.id=$2`, bookID, chapterID).
		Scan(&bookTitle, &chapterTitle, &rawPath, &dehydratedPath, &status)
	return bookTitle, chapterTitle, rawPath, dehydratedPath, status, err
}
