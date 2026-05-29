package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/models"
	"github.com/LogicShao/novel-dehydrator/internal/services/jobmanager"
)

const (
	charsPerToken      = 1.67
	systemPromptTokens = 800
	flashInputPrice    = 1.0
	flashOutputPrice   = 4.0
)

type estimateResult struct {
	TotalChars   int     `json:"total_chars"`
	NumChapters  int     `json:"num_chapters"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Model        string  `json:"model"`
	CostYuan     float64 `json:"cost_yuan"`
}

type jobsStore interface {
	Estimate(ctx context.Context, bookID int64, chapterIDs []int64) (estimateResult, error)
	CreateJob(ctx context.Context, bookID int64, req models.StartJobRequest) (models.Job, error)
	GetJob(ctx context.Context, jobID int64) (*models.Job, error)
	CancelJob(ctx context.Context, jobID int64) error
	GetLatestJob(ctx context.Context, bookID int64) (*models.Job, error)
	checkAPIKey(ctx context.Context) (bool, error)
}

type jobController interface {
	StartJob(ctx context.Context, jobID int64)
	PauseJob(jobID int64)
	ResumeJob(ctx context.Context, jobID int64)
	CancelJob(jobID int64)
}

type postgresJobsStore struct {
	pool           *pgxpool.Pool
	fallbackAPIKey string
}

func HandleEstimate(pool *pgxpool.Pool) http.HandlerFunc {
	return handleEstimate(&postgresJobsStore{pool: pool})
}

func HandleCreateJob(pool *pgxpool.Pool, cfg *config.Config, manager *jobmanager.Manager) http.HandlerFunc {
	return handleCreateJob(&postgresJobsStore{pool: pool, fallbackAPIKey: fallbackAPIKey(cfg)}, manager)
}

func HandleGetJob(pool *pgxpool.Pool) http.HandlerFunc {
	return handleGetJob(&postgresJobsStore{pool: pool})
}

func HandlePauseJob(manager *jobmanager.Manager) http.HandlerFunc {
	return handlePauseJob(manager)
}

func HandleResumeJob(manager *jobmanager.Manager) http.HandlerFunc {
	return handleResumeJob(manager)
}

func HandleCancelJob(pool *pgxpool.Pool, manager *jobmanager.Manager) http.HandlerFunc {
	return handleCancelJob(&postgresJobsStore{pool: pool}, manager)
}

func HandleLatestJob(pool *pgxpool.Pool) http.HandlerFunc {
	return handleLatestJob(&postgresJobsStore{pool: pool})
}

func handleEstimate(store jobsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}

		var req models.EstimateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request")
			return
		}

		result, err := store.Estimate(r.Context(), bookID, req.ChapterIDs)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "估算失败")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleCreateJob(store jobsStore, manager jobController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}

		var req models.StartJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request")
			return
		}

		hasKey, err := store.checkAPIKey(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "查询配置失败")
			return
		}
		if !hasKey {
			writeJSONError(w, http.StatusBadRequest, "请先配置 DeepSeek API Key")
			return
		}

		job, err := store.CreateJob(r.Context(), bookID, req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if manager != nil {
			manager.StartJob(r.Context(), job.ID)
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func handleGetJob(store jobsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseInt64Param(w, r, "jobID")
		if !ok {
			return
		}
		job, err := store.GetJob(r.Context(), jobID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "查询任务失败")
			return
		}
		if job == nil {
			writeJSONError(w, http.StatusNotFound, "任务不存在")
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func handlePauseJob(manager jobController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseInt64Param(w, r, "jobID")
		if !ok {
			return
		}
		if manager != nil {
			manager.PauseJob(jobID)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleResumeJob(manager jobController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseInt64Param(w, r, "jobID")
		if !ok {
			return
		}
		if manager != nil {
			manager.ResumeJob(r.Context(), jobID)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleCancelJob(store jobsStore, manager jobController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseInt64Param(w, r, "jobID")
		if !ok {
			return
		}
		if manager != nil {
			manager.CancelJob(jobID)
		}
		if err := store.CancelJob(r.Context(), jobID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "取消任务失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleLatestJob(store jobsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID, ok := parseInt64Param(w, r, "bookID")
		if !ok {
			return
		}
		job, err := store.GetLatestJob(r.Context(), bookID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "查询最新任务失败")
			return
		}
		if job == nil {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func (s *postgresJobsStore) Estimate(ctx context.Context, bookID int64, chapterIDs []int64) (estimateResult, error) {
	if len(chapterIDs) == 0 {
		return estimateResult{Model: "deepseek-v4-flash"}, nil
	}

	rows, err := s.pool.Query(ctx, `SELECT raw_char_count FROM chapters WHERE book_id=$1 AND id = ANY($2)`, bookID, chapterIDs)
	if err != nil {
		return estimateResult{}, err
	}
	defer rows.Close()

	totalChars := 0
	numChapters := 0
	for rows.Next() {
		var rawCharCount int
		if err := rows.Scan(&rawCharCount); err != nil {
			return estimateResult{}, err
		}
		totalChars += rawCharCount
		numChapters++
	}
	if err := rows.Err(); err != nil {
		return estimateResult{}, err
	}

	inputTokens := int(math.Round(float64(totalChars)/charsPerToken)) + systemPromptTokens*numChapters
	outputTokens := int(math.Round(float64(totalChars) * 0.35 / charsPerToken))
	cost := (float64(inputTokens)*flashInputPrice + float64(outputTokens)*flashOutputPrice) / 1_000_000

	return estimateResult{
		TotalChars:   totalChars,
		NumChapters:  numChapters,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		Model:        "deepseek-v4-flash",
		CostYuan:     math.Round(cost*10000) / 10000,
	}, nil
}

func (s *postgresJobsStore) CreateJob(ctx context.Context, bookID int64, req models.StartJobRequest) (models.Job, error) {
	chapterIDs, err := s.resolveChapterIDs(ctx, bookID, req)
	if err != nil {
		return models.Job{}, err
	}
	if len(chapterIDs) == 0 {
		return models.Job{}, errBadRequest("没有找到可脱水的章节")
	}

	var job models.Job
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Job{}, err
	}
	defer tx.Rollback(ctx)

	err = func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT 1 FROM books WHERE id=$1", bookID).Scan(new(int)); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `INSERT INTO jobs (book_id, scope_type, total_count)
			VALUES ($1, $2, $3)
			RETURNING id, book_id, status, scope_type, total_count, done_count, failed_count, current_chapter_id, created_at, updated_at`,
			bookID, req.ScopeType, len(chapterIDs),
		).Scan(&job.ID, &job.BookID, &job.Status, &job.ScopeType, &job.TotalCount, &job.DoneCount, &job.FailedCount, &job.CurrentChapterID, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return err
		}

		for _, chapterID := range chapterIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO job_chapters (job_id, chapter_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, job.ID, chapterID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE chapters SET dehydrate_status='pending'
			WHERE id = ANY($1) AND dehydrate_status IN ('failed', 'processing')`, chapterIDs)
		return err
	}(tx)
	if err != nil {
		return models.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Job{}, err
	}
	return job, err
}

func (s *postgresJobsStore) GetJob(ctx context.Context, jobID int64) (*models.Job, error) {
	var job models.Job
	err := s.pool.QueryRow(ctx, `SELECT id, book_id, status, scope_type, total_count, done_count, failed_count,
		current_chapter_id, created_at, updated_at FROM jobs WHERE id=$1`, jobID).
		Scan(&job.ID, &job.BookID, &job.Status, &job.ScopeType, &job.TotalCount, &job.DoneCount, &job.FailedCount, &job.CurrentChapterID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (s *postgresJobsStore) CancelJob(ctx context.Context, jobID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status='cancelled', updated_at=NOW() WHERE id=$1`, jobID)
	return err
}

func (s *postgresJobsStore) GetLatestJob(ctx context.Context, bookID int64) (*models.Job, error) {
	var job models.Job
	err := s.pool.QueryRow(ctx, `SELECT id, book_id, status, scope_type, total_count, done_count, failed_count,
		current_chapter_id, created_at, updated_at FROM jobs WHERE book_id=$1 ORDER BY id DESC LIMIT 1`, bookID).
		Scan(&job.ID, &job.BookID, &job.Status, &job.ScopeType, &job.TotalCount, &job.DoneCount, &job.FailedCount, &job.CurrentChapterID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (s *postgresJobsStore) checkAPIKey(ctx context.Context) (bool, error) {
	if s.pool == nil {
		return strings.TrimSpace(s.fallbackAPIKey) != "", nil
	}
	var key string
	err := s.pool.QueryRow(ctx, `SELECT deepseek_api_key FROM app_settings WHERE id=1`).Scan(&key)
	if err == pgx.ErrNoRows {
		return strings.TrimSpace(s.fallbackAPIKey) != "", nil
	}
	if err != nil {
		return false, err
	}
	return hasConfiguredAPIKey(key, s.fallbackAPIKey), nil
}

func (s *postgresJobsStore) resolveChapterIDs(ctx context.Context, bookID int64, req models.StartJobRequest) ([]int64, error) {
	switch req.ScopeType {
	case "volumes":
		if len(req.VolumeIDs) == 0 {
			return nil, errBadRequest("scope_type 必须是 volumes 或 chapters，且对应 ids 不能为空")
		}
		rows, err := s.pool.Query(ctx, `SELECT id FROM chapters WHERE book_id=$1 AND volume_id = ANY($2) ORDER BY seq`, bookID, req.VolumeIDs)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return nil, errBadRequest("没有找到可脱水的章节")
		}
		return ids, rows.Err()
	case "chapters":
		if len(req.ChapterIDs) == 0 {
			return nil, errBadRequest("scope_type 必须是 volumes 或 chapters，且对应 ids 不能为空")
		}
		return req.ChapterIDs, nil
	default:
		return nil, errBadRequest("scope_type 必须是 volumes 或 chapters，且对应 ids 不能为空")
	}
}

type badRequestError struct{ message string }

func (e badRequestError) Error() string { return e.message }

func errBadRequest(message string) error { return badRequestError{message: message} }
