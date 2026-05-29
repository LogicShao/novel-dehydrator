package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LogicShao/novel-dehydrator/internal/services/jobmanager"
)

type stubProgressManager struct {
	ch           chan jobmanager.Event
	unsubscribed bool
	jobID        int64
}

func (s *stubProgressManager) Subscribe(jobID int64) <-chan jobmanager.Event {
	s.jobID = jobID
	return s.ch
}

func (s *stubProgressManager) Unsubscribe(jobID int64, ch <-chan jobmanager.Event) {
	s.unsubscribed = true
}

func TestHandleJobProgressStream(t *testing.T) {
	mgr := &stubProgressManager{ch: make(chan jobmanager.Event, 1)}
	handler := handleJobProgressStream(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/9/stream", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "9")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	mgr.ch <- jobmanager.Event{Type: "progress", Data: map[string]any{"job_id": 9, "done": 1}}
	close(mgr.ch)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream handler")
	}

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: progress") || !strings.Contains(body, `"job_id":9`) {
		t.Fatalf("unexpected body: %q", body)
	}
	if !mgr.unsubscribed || mgr.jobID != 9 {
		t.Fatalf("unexpected manager state: unsubscribed=%v jobID=%d", mgr.unsubscribed, mgr.jobID)
	}
}
