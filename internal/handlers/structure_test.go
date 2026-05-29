package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/LogicShao/novel-dehydrator/internal/models"
)

type stubStructureStore struct {
	out models.BookStructureOut
	err error
}

func (s *stubStructureStore) GetBookStructure(context.Context, int64) (models.BookStructureOut, error) {
	return s.out, s.err
}

func TestHandleGetStructureSuccess(t *testing.T) {
	volumeID := int64(10)
	handler := handleGetStructure(&stubStructureStore{out: models.BookStructureOut{
		HasVolumes: true,
		Volumes: []models.Volume{{
			ID:    volumeID,
			Title: "卷一",
			Seq:   1,
			Chapters: []models.Chapter{{
				ID:              100,
				Title:           "第一章",
				Seq:             1,
				VolumeID:        &volumeID,
				RawCharCount:    1200,
				DehydrateStatus: "done",
			}},
		}},
		LooseChapters: []models.Chapter{{ID: 101, Title: "番外", Seq: 2}},
	}})

	req := newRouteRequest(http.MethodGet, "/api/books/1/structure", map[string]string{"bookID": "1"}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var out models.BookStructureOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.HasVolumes || len(out.Volumes) != 1 || len(out.Volumes[0].Chapters) != 1 || len(out.LooseChapters) != 1 {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestHandleGetStructureNotFound(t *testing.T) {
	handler := handleGetStructure(&stubStructureStore{err: pgx.ErrNoRows})
	req := newRouteRequest(http.MethodGet, "/api/books/1/structure", map[string]string{"bookID": "1"}, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetStructureBadBookID(t *testing.T) {
	handler := handleGetStructure(&stubStructureStore{err: errors.New("should not call")})
	req := newRouteRequest(http.MethodGet, "/api/books/x/structure", map[string]string{"bookID": "x"}, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func newRouteRequest(method, target string, params map[string]string, _ http.Handler) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req
}
