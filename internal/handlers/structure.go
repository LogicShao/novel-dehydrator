package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/models"
)

type structureStore interface {
	GetBookStructure(ctx context.Context, bookID int64) (models.BookStructureOut, error)
}

type postgresStructureStore struct {
	pool *pgxpool.Pool
}

func HandleGetStructure(pool *pgxpool.Pool) http.HandlerFunc {
	return handleGetStructure(&postgresStructureStore{pool: pool})
}

func handleGetStructure(store structureStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}

		out, err := store.GetBookStructure(r.Context(), bookID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "书籍不存在")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "查询书籍结构失败")
			return
		}

		writeJSON(w, http.StatusOK, out)
	}
}

func (s *postgresStructureStore) GetBookStructure(ctx context.Context, bookID int64) (models.BookStructureOut, error) {
	var hasVolumesInt int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(has_volumes, 0) FROM books WHERE id=$1`, bookID).Scan(&hasVolumesInt); err != nil {
		return models.BookStructureOut{}, err
	}
	hasVolumes := hasVolumesInt != 0

	volRows, err := s.pool.Query(ctx, `SELECT id, book_id, title, seq, detect_source FROM volumes WHERE book_id=$1 ORDER BY seq`, bookID)
	if err != nil {
		return models.BookStructureOut{}, err
	}
	defer volRows.Close()

	volumes := make([]models.Volume, 0)
	volumeMap := make(map[int64]int)
	for volRows.Next() {
		var volume models.Volume
		if err := volRows.Scan(&volume.ID, &volume.BookID, &volume.Title, &volume.Seq, &volume.DetectSource); err != nil {
			return models.BookStructureOut{}, err
		}
		volume.Chapters = []models.Chapter{}
		volumeMap[volume.ID] = len(volumes)
		volumes = append(volumes, volume)
	}
	if err := volRows.Err(); err != nil {
		return models.BookStructureOut{}, err
	}

	chapterRows, err := s.pool.Query(ctx, `SELECT id, book_id, volume_id, title, seq, raw_path, raw_char_count,
		dehydrate_status, dehydrated_path, dehydrated_char_count, compression_ratio, error_msg,
		retry_count, processed_at
		FROM chapters WHERE book_id=$1 ORDER BY seq`, bookID)
	if err != nil {
		return models.BookStructureOut{}, err
	}
	defer chapterRows.Close()

	looseChapters := make([]models.Chapter, 0)
	for chapterRows.Next() {
		var chapter models.Chapter
		if err := chapterRows.Scan(
			&chapter.ID,
			&chapter.BookID,
			&chapter.VolumeID,
			&chapter.Title,
			&chapter.Seq,
			&chapter.RawPath,
			&chapter.RawCharCount,
			&chapter.DehydrateStatus,
			&chapter.DehydratedPath,
			&chapter.DehydratedCharCount,
			&chapter.CompressionRatio,
			&chapter.ErrorMsg,
			&chapter.RetryCount,
			&chapter.ProcessedAt,
		); err != nil {
			return models.BookStructureOut{}, err
		}

		if chapter.VolumeID == nil {
			looseChapters = append(looseChapters, chapter)
			continue
		}

		idx, ok := volumeMap[*chapter.VolumeID]
		if !ok {
			looseChapters = append(looseChapters, chapter)
			continue
		}
		volumes[idx].Chapters = append(volumes[idx].Chapters, chapter)
	}
	if err := chapterRows.Err(); err != nil {
		return models.BookStructureOut{}, err
	}

	return models.BookStructureOut{
		HasVolumes:    hasVolumes,
		Volumes:       volumes,
		LooseChapters: looseChapters,
	}, nil
	}

func parseInt64Param(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value := chi.URLParam(r, name)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid path parameter")
		return 0, false
	}
	return parsed, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
