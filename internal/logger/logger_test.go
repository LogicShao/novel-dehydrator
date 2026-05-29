package logger

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := NotFound("书籍不存在")
	if err.Error() != "书籍不存在" {
		t.Errorf("expected '书籍不存在', got '%s'", err.Error())
	}
	if err.Status != 404 {
		t.Errorf("expected status 404, got %d", err.Status)
	}
}

func TestBadRequest(t *testing.T) {
	err := BadRequest("参数错误")
	if err.Status != 400 {
		t.Errorf("expected status 400, got %d", err.Status)
	}
	if err.Message != "参数错误" {
		t.Errorf("expected message '参数错误', got '%s'", err.Message)
	}
}

func TestInternalError(t *testing.T) {
	err := InternalError("服务器内部错误")
	if err.Status != 500 {
		t.Errorf("expected status 500, got %d", err.Status)
	}
	if err.Message != "服务器内部错误" {
		t.Errorf("expected message '服务器内部错误', got '%s'", err.Message)
	}
}

func TestWriteErrorAppError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, NotFound("书籍不存在"))

	if w.Code != 404 {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["detail"] != "书籍不存在" {
		t.Errorf("expected detail '书籍不存在', got '%s'", resp["detail"])
	}
}

func TestWriteErrorGeneric(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, fmt.Errorf("something went wrong"))

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["detail"] != "something went wrong" {
		t.Errorf("expected detail 'something went wrong', got '%s'", resp["detail"])
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}
	WriteJSON(w, 200, data)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type 'application/json; charset=utf-8', got '%s'", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["hello"] != "world" {
		t.Errorf("expected hello='world', got '%s'", resp["hello"])
	}
}
