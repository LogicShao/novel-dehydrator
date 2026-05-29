package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LogicShao/novel-dehydrator/internal/models"
)

type stubJobsStore struct {
	estimate    estimateResult
	estimateErr error
	createdJob  models.Job
	createErr   error
	job         *models.Job
	jobErr      error
	latest      *models.Job
	latestErr   error
	cancelledID int64
	cancelErr   error
	createReq   models.StartJobRequest
	createBook  int64
	estBook     int64
	estIDs      []int64
	hasApiKey   bool
	apiKeyErr   error
}

func (s *stubJobsStore) Estimate(_ context.Context, bookID int64, chapterIDs []int64) (estimateResult, error) {
	s.estBook = bookID
	s.estIDs = append([]int64(nil), chapterIDs...)
	return s.estimate, s.estimateErr
}
func (s *stubJobsStore) CreateJob(_ context.Context, bookID int64, req models.StartJobRequest) (models.Job, error) {
	s.createBook = bookID
	s.createReq = req
	return s.createdJob, s.createErr
}
func (s *stubJobsStore) GetJob(context.Context, int64) (*models.Job, error) { return s.job, s.jobErr }
func (s *stubJobsStore) CancelJob(_ context.Context, jobID int64) error {
	s.cancelledID = jobID
	return s.cancelErr
}
func (s *stubJobsStore) GetLatestJob(context.Context, int64) (*models.Job, error) {
	return s.latest, s.latestErr
}
func (s *stubJobsStore) checkAPIKey(context.Context) (bool, error) { return s.hasApiKey, s.apiKeyErr }

type stubJobController struct {
	started  []int64
	paused   []int64
	resumed  []int64
	canceled []int64
}

func (s *stubJobController) StartJob(_ context.Context, jobID int64) {
	s.started = append(s.started, jobID)
}
func (s *stubJobController) PauseJob(jobID int64) { s.paused = append(s.paused, jobID) }
func (s *stubJobController) ResumeJob(_ context.Context, jobID int64) {
	s.resumed = append(s.resumed, jobID)
}
func (s *stubJobController) CancelJob(jobID int64) { s.canceled = append(s.canceled, jobID) }

func TestHandleEstimate(t *testing.T) {
	store := &stubJobsStore{estimate: estimateResult{TotalChars: 3000, InputTokens: 3396, OutputTokens: 629, TotalTokens: 4025, Model: "deepseek-v4-flash", CostYuan: 0.0059}}
	handler := handleEstimate(store)
	req := newJSONRouteRequest(http.MethodPost, "/api/books/1/estimate", map[string]string{"bookID": "1"}, map[string]any{"chapter_ids": []int64{1, 2}})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.estBook != 1 || len(store.estIDs) != 2 {
		t.Fatalf("unexpected estimate call: book=%d ids=%v", store.estBook, store.estIDs)
	}
}

func TestHandleCreateJob(t *testing.T) {
	now := time.Now().UTC()
	store := &stubJobsStore{createdJob: models.Job{ID: 9, BookID: 1, Status: "running", ScopeType: "chapters", TotalCount: 2, CreatedAt: now, UpdatedAt: now}, hasApiKey: true}
	manager := &stubJobController{}
	handler := handleCreateJob(store, manager)
	req := newJSONRouteRequest(http.MethodPost, "/api/books/1/jobs", map[string]string{"bookID": "1"}, map[string]any{"scope_type": "chapters", "chapter_ids": []int64{11, 12}})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(manager.started) != 1 || manager.started[0] != 9 {
		t.Fatalf("unexpected start calls: %v", manager.started)
	}
	if store.createReq.ScopeType != "chapters" || len(store.createReq.ChapterIDs) != 2 {
		t.Fatalf("unexpected create req: %+v", store.createReq)
	}
}

func TestHandleCreateJobNoAPIKey(t *testing.T) {
	store := &stubJobsStore{hasApiKey: false}
	manager := &stubJobController{}
	handler := handleCreateJob(store, manager)
	req := newJSONRouteRequest(http.MethodPost, "/api/books/1/jobs", map[string]string{"bookID": "1"}, map[string]any{"scope_type": "chapters", "chapter_ids": []int64{11}})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["detail"] != "请先配置 DeepSeek API Key" {
		t.Fatalf("detail = %q, want %q", body["detail"], "请先配置 DeepSeek API Key")
	}
	if len(manager.started) != 0 {
		t.Fatalf("expected no job started, got %v", manager.started)
	}
}

func TestPostgresJobsStoreCheckAPIKeyFallsBackToConfig(t *testing.T) {
	store := &postgresJobsStore{fallbackAPIKey: "sk-env-key"}

	ok, err := store.checkAPIKey(context.Background())
	if err != nil {
		t.Fatalf("checkAPIKey returned error: %v", err)
	}
	if !ok {
		t.Fatal("checkAPIKey = false, want true when fallback key exists")
	}
}

func TestHandleGetJobNotFound(t *testing.T) {
	handler := handleGetJob(&stubJobsStore{})
	req := newRouteRequestWithBody(http.MethodGet, "/api/jobs/99", map[string]string{"jobID": "99"}, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlePauseResumeCancelAndLatest(t *testing.T) {
	store := &stubJobsStore{latest: &models.Job{ID: 7, BookID: 1, Status: "paused"}}
	manager := &stubJobController{}

	pauseRec := httptest.NewRecorder()
	handlePauseJob(manager).ServeHTTP(pauseRec, newRouteRequestWithBody(http.MethodPost, "/api/jobs/7/pause", map[string]string{"jobID": "7"}, nil))
	resumeRec := httptest.NewRecorder()
	handleResumeJob(manager).ServeHTTP(resumeRec, newRouteRequestWithBody(http.MethodPost, "/api/jobs/7/resume", map[string]string{"jobID": "7"}, nil))
	cancelRec := httptest.NewRecorder()
	handleCancelJob(store, manager).ServeHTTP(cancelRec, newRouteRequestWithBody(http.MethodPost, "/api/jobs/7/cancel", map[string]string{"jobID": "7"}, nil))
	latestRec := httptest.NewRecorder()
	handleLatestJob(store).ServeHTTP(latestRec, newRouteRequestWithBody(http.MethodGet, "/api/books/1/jobs/latest", map[string]string{"bookID": "1"}, nil))

	if len(manager.paused) != 1 || len(manager.resumed) != 1 || len(manager.canceled) != 1 {
		t.Fatalf("unexpected manager calls: paused=%v resumed=%v canceled=%v", manager.paused, manager.resumed, manager.canceled)
	}
	if store.cancelledID != 7 {
		t.Fatalf("cancelled id = %d, want 7", store.cancelledID)
	}
	if latestRec.Code != http.StatusOK || pauseRec.Code != http.StatusOK || resumeRec.Code != http.StatusOK || cancelRec.Code != http.StatusOK {
		t.Fatalf("unexpected statuses pause=%d resume=%d cancel=%d latest=%d", pauseRec.Code, resumeRec.Code, cancelRec.Code, latestRec.Code)
	}
}

func newJSONRouteRequest(method, target string, params map[string]string, payload any) *http.Request {
	body, _ := json.Marshal(payload)
	return newRouteRequestWithBody(method, target, params, bytes.NewReader(body))
}

func newRouteRequestWithBody(method, target string, params map[string]string, body *bytes.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = newRouteRequest(method, target, params, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
		req.Header.Set("Content-Type", "application/json")
		rctx := chi.NewRouteContext()
		for key, value := range params {
			rctx.URLParams.Add(key, value)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	return req
}
