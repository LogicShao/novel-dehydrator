package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LogicShao/novel-dehydrator/internal/services/prompts"
)

func TestHandleGetDefaultPrompts(t *testing.T) {
	handler := HandleGetDefaultPrompts()
	if handler == nil {
		t.Fatal("HandleGetDefaultPrompts returned nil handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/prompts/defaults", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	expectedKeys := []string{"system", "user", "position_early", "position_normal"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("response missing key: %s", key)
		}
	}

	if result["system"] != prompts.DehydrateSystem {
		t.Error("system prompt does not match DehydrateSystem")
	}
	if result["user"] != prompts.DehydrateUser {
		t.Error("user prompt does not match DehydrateUser")
	}
	if result["position_early"] != prompts.PositionHintEarly {
		t.Error("position_early does not match PositionHintEarly")
	}
	if result["position_normal"] != prompts.PositionHintNormal {
		t.Error("position_normal does not match PositionHintNormal")
	}
}

func TestHandleGetDefaultPromptsContentType(t *testing.T) {
	handler := HandleGetDefaultPrompts()
	req := httptest.NewRequest(http.MethodGet, "/api/prompts/defaults", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
