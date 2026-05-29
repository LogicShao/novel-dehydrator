package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LogicShao/novel-dehydrator/internal/services/deepseek"
)

type stubChatStore struct {
	bookTitle      string
	chapterTitle   string
	rawPath        string
	dehydratedPath *string
	status         string
	err            error
}

func (s *stubChatStore) GetChatContext(context.Context, int64, int64) (string, string, string, *string, string, error) {
	return s.bookTitle, s.chapterTitle, s.rawPath, s.dehydratedPath, s.status, s.err
}

type stubChatClient struct {
	answer   string
	err      error
	messages []deepseek.Message
}

func (s *stubChatClient) ChatCompletion(_ context.Context, messages []deepseek.Message, stream bool) (string, error) {
	s.messages = append([]deepseek.Message(nil), messages...)
	if stream {
		return "", errors.New("unexpected streaming")
	}
	return s.answer, s.err
}

func TestHandleChapterChat(t *testing.T) {
	path := "done.txt"
	store := &stubChatStore{bookTitle: "测试书", chapterTitle: "第一章", rawPath: "raw.txt", dehydratedPath: &path, status: "done"}
	client := &stubChatClient{answer: "这是答案"}
	handler := handleChapterChat(store, client, func(path string) (string, error) {
		if path != "done.txt" {
			t.Fatalf("path = %q, want done.txt", path)
		}
		return "章节正文", nil
	})
	body := strings.NewReader(`{"chapter_id":2,"question":"主角是谁？","history":[{"role":"user","content":"之前问题"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/books/1/chat", body)
	req = withRouteParam(req, "bookID", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(client.messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(client.messages))
	}
	if client.messages[0].Role != "system" || !strings.Contains(client.messages[0].Content, "章节正文") {
		t.Fatalf("unexpected system message: %+v", client.messages[0])
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out["answer"] != "这是答案" {
		t.Fatalf("answer = %q", out["answer"])
	}
}

func TestHandleChapterChatReadError(t *testing.T) {
	store := &stubChatStore{bookTitle: "测试书", chapterTitle: "第一章", rawPath: "raw.txt", status: "pending"}
	handler := handleChapterChat(store, &stubChatClient{}, func(string) (string, error) { return "", errors.New("boom") })
	req := httptest.NewRequest(http.MethodPost, "/api/books/1/chat", strings.NewReader(`{"chapter_id":2,"question":"?"}`))
	req = withRouteParam(req, "bookID", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func withRouteParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
