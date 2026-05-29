package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/services/jobmanager"
)

type progressManager interface {
	Subscribe(jobID int64) <-chan jobmanager.Event
	Unsubscribe(jobID int64, ch <-chan jobmanager.Event)
}

func HandleJobProgressStream(pool *pgxpool.Pool, manager *jobmanager.Manager) http.HandlerFunc {
	return handleJobProgressStream(manager)
}

func handleJobProgressStream(manager progressManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseInt64Param(w, r, "jobID")
		if !ok {
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := manager.Subscribe(jobID)
		defer manager.Unsubscribe(jobID, ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				payload, err := json.Marshal(event.Data)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)
				flusher.Flush()
			}
		}
	}
}
