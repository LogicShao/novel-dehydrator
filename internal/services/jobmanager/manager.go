package jobmanager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/storage"
)

const (
	defaultConcurrency = 5
	maxConcurrency     = 500
	defaultBatchSize   = 10
	eventBufferSize    = 32
)

type Event struct {
	Type string
	Data any
}

type Dehydrator interface {
	DehydrateChapter(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error)
}

type Manager struct {
	pool       *pgxpool.Pool
	dataDir    string
	dehydrator Dehydrator
	storage    chapterStorage
	store      jobStore

	mu   sync.RWMutex
	jobs map[int64]*jobState
}

type jobState struct {
	cancel         context.CancelFunc
	pauseRequested bool
	running        bool
	subscribers    []chan Event
}

type chapterStorage interface {
	ReadChapter(path string) (string, error)
	WriteDehydratedChapter(path, content string) error
	DehydratedChapterPath(dataDir string, bookID, chapterID int64) string
}

type jobStore interface {
	GetConcurrency(ctx context.Context) (int, error)
	PendingChapterIDs(ctx context.Context, jobID int64) ([]int64, error)
	GetBookMeta(ctx context.Context, chapterID int64) (bookMeta, error)
	GetChapter(ctx context.Context, chapterID int64) (chapterRecord, error)
	GetJobCounts(ctx context.Context, jobID int64) (jobCounts, error)
	MarkJobRunning(ctx context.Context, jobID int64) error
	MarkJobPaused(ctx context.Context, jobID int64) error
	MarkJobCancelled(ctx context.Context, jobID int64) error
	MarkJobCompleted(ctx context.Context, jobID int64) error
	MarkChapterProcessing(ctx context.Context, jobID, chapterID int64) error
	MarkChapterDone(ctx context.Context, jobID, chapterID int64, dehydratedPath string, dehydratedChars int, compressionRatio float64) error
	MarkChapterFailed(ctx context.Context, jobID, chapterID int64, errMsg string) error
}

type bookMeta struct {
	BookID        int64
	BookTitle     string
	TotalChapters int
	DoneCount     int
	FailedCount   int
	TotalCount    int
}

type chapterRecord struct {
	ID      int64
	BookID  int64
	Title   string
	RawPath string
	Seq     int
}

type jobCounts struct {
	Done   int
	Failed int
	Total  int
}

type fileStorage struct{}

func (fileStorage) ReadChapter(path string) (string, error) {
	return storage.ReadChapter(path)
}

func (fileStorage) WriteDehydratedChapter(path, content string) error {
	return storage.WriteDehydratedChapter(path, content)
}

func (fileStorage) DehydratedChapterPath(dataDir string, bookID, chapterID int64) string {
	return storage.DehydratedChapterPath(dataDir, bookID, chapterID)
}

func NewManager(pool *pgxpool.Pool, dataDir string, dehydrator Dehydrator) *Manager {
	return &Manager{
		pool:       pool,
		dataDir:    dataDir,
		dehydrator: dehydrator,
		storage:    fileStorage{},
		store:      &postgresStore{pool: pool},
		jobs:       make(map[int64]*jobState),
	}
}

func (m *Manager) StartJob(ctx context.Context, jobID int64) {
	if m == nil {
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	if !m.beginRun(jobID, cancel) {
		cancel()
		return
	}

	go func() {
		defer m.finishRun(jobID)
		m.run(runCtx, jobID)
	}()
}

func (m *Manager) PauseJob(jobID int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	state := m.ensureJobStateLocked(jobID)
	state.pauseRequested = true
	m.mu.Unlock()
	m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "pause_pending"})
}

func (m *Manager) ResumeJob(ctx context.Context, jobID int64) {
	if m == nil {
		return
	}

	if err := m.store.MarkJobRunning(ctx, jobID); err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "failed", "error": err.Error()})
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	if !m.resumeRun(jobID, cancel) {
		cancel()
		return
	}

	go func() {
		defer m.finishRun(jobID)
		m.run(runCtx, jobID)
	}()
}

func (m *Manager) CancelJob(jobID int64) {
	if m == nil {
		return
	}

	m.mu.Lock()
	state := m.ensureJobStateLocked(jobID)
	state.pauseRequested = false
	cancel := state.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if err := m.store.MarkJobCancelled(context.Background(), jobID); err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "error", "error": err.Error()})
	}
	m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "cancelled"})
}

func (m *Manager) Subscribe(jobID int64) <-chan Event {
	ch := make(chan Event, eventBufferSize)
	m.mu.Lock()
	state := m.ensureJobStateLocked(jobID)
	state.subscribers = append(state.subscribers, ch)
	m.mu.Unlock()
	return ch
}

func (m *Manager) Unsubscribe(jobID int64, ch <-chan Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.jobs[jobID]
	if !ok {
		return
	}
	for i, sub := range state.subscribers {
		if (<-chan Event)(sub) != ch {
			continue
		}
		state.subscribers = append(state.subscribers[:i], state.subscribers[i+1:]...)
		close(sub)
		break
	}
}

func (m *Manager) run(ctx context.Context, jobID int64) {
	chapterIDs, err := m.store.PendingChapterIDs(ctx, jobID)
	if err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "failed", "error": err.Error()})
		return
	}

	if len(chapterIDs) == 0 {
		if err := m.store.MarkJobCompleted(ctx, jobID); err == nil {
			m.broadcast(jobID, "done", map[string]any{"job_id": jobID, "status": "completed", "done": 0, "failed": 0, "total": 0})
		}
		return
	}

	meta, err := m.store.GetBookMeta(ctx, chapterIDs[0])
	if err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "failed", "error": err.Error()})
		return
	}

	concurrency, err := m.store.GetConcurrency(ctx)
	if err != nil || concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	if concurrency > maxConcurrency {
		concurrency = maxConcurrency
	}

	for start := 0; start < len(chapterIDs); start += defaultBatchSize {
		if err := ctx.Err(); err != nil {
			return
		}
		if m.isPauseRequested(jobID) {
			m.doPause(ctx, jobID)
			return
		}

		end := start + defaultBatchSize
		if end > len(chapterIDs) {
			end = len(chapterIDs)
		}
		batch := chapterIDs[start:end]
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for _, chapterID := range batch {
			wg.Add(1)
			sem <- struct{}{}
			go func(chapterID int64) {
				defer wg.Done()
				defer func() { <-sem }()

				if err := ctx.Err(); err != nil {
					return
				}
				if m.isPauseRequested(jobID) {
					return
				}

				m.processChapter(ctx, jobID, meta, chapterID)
			}(chapterID)
		}

		wg.Wait()

		if err := ctx.Err(); err != nil {
			return
		}
		if m.isPauseRequested(jobID) {
			m.doPause(ctx, jobID)
			return
		}
	}

	if err := m.store.MarkJobCompleted(ctx, jobID); err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "failed", "error": err.Error()})
		return
	}
	counts, err := m.store.GetJobCounts(ctx, jobID)
	if err != nil {
		counts = jobCounts{}
	}
	m.broadcast(jobID, "done", map[string]any{
		"job_id": jobID,
		"status": "completed",
		"done":   counts.Done,
		"failed": counts.Failed,
		"total":  counts.Total,
	})
}

func (m *Manager) processChapter(ctx context.Context, jobID int64, meta bookMeta, chapterID int64) {
	chapter, err := m.store.GetChapter(ctx, chapterID)
	if err != nil {
		m.broadcast(jobID, "chapter_failed", map[string]any{"chapter_id": chapterID, "status": "failed", "error": truncate(err.Error(), 200)})
		return
	}

	if err := m.store.MarkChapterProcessing(ctx, jobID, chapterID); err != nil {
		if !errors.Is(err, context.Canceled) {
			m.broadcast(jobID, "chapter_failed", map[string]any{"chapter_id": chapterID, "status": "failed", "error": truncate(err.Error(), 200)})
		}
		return
	}

	counts, err := m.store.GetJobCounts(ctx, jobID)
	if err == nil {
		m.broadcast(jobID, "progress", map[string]any{
			"job_id": jobID,
			"done":   counts.Done,
			"failed": counts.Failed,
			"total":  counts.Total,
			"current": map[string]any{
				"id":    chapterID,
				"title": chapter.Title,
			},
		})
	}

	rawText, err := m.storage.ReadChapter(chapter.RawPath)
	if err != nil {
		m.failChapter(ctx, jobID, chapterID, err)
		return
	}

	dehydratedText, err := m.dehydrator.DehydrateChapter(ctx, meta.BookTitle, chapter.Title, rawText, chapter.Seq, meta.TotalChapters)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		m.failChapter(ctx, jobID, chapterID, err)
		return
	}

	dehydratedPath := m.storage.DehydratedChapterPath(m.dataDir, meta.BookID, chapterID)
	if err := m.storage.WriteDehydratedChapter(dehydratedPath, dehydratedText); err != nil {
		m.failChapter(ctx, jobID, chapterID, err)
		return
	}

	rawChars := maxInt(utf8.RuneCountInString(rawText), 1)
	dehydratedChars := utf8.RuneCountInString(dehydratedText)
	ratio := float64(dehydratedChars) / float64(rawChars)

	if err := m.store.MarkChapterDone(ctx, jobID, chapterID, dehydratedPath, dehydratedChars, ratio); err != nil {
		if !errors.Is(err, context.Canceled) {
			m.broadcast(jobID, "chapter_failed", map[string]any{"chapter_id": chapterID, "status": "failed", "error": truncate(err.Error(), 200)})
		}
		return
	}

	m.broadcast(jobID, "chapter_done", map[string]any{
		"chapter_id":        chapterID,
		"status":            "done",
		"compression_ratio": ratio,
	})
}

func (m *Manager) failChapter(ctx context.Context, jobID, chapterID int64, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	errMsg := truncate(err.Error(), 500)
	if err := m.store.MarkChapterFailed(ctx, jobID, chapterID, errMsg); err != nil {
		m.broadcast(jobID, "chapter_failed", map[string]any{
			"chapter_id": chapterID,
			"status":     "failed",
			"error":      truncate(err.Error(), 200),
		})
		return
	}
	m.broadcast(jobID, "chapter_failed", map[string]any{
		"chapter_id": chapterID,
		"status":     "failed",
		"error":      truncate(errMsg, 200),
	})
}

func (m *Manager) doPause(ctx context.Context, jobID int64) {
	if err := m.store.MarkJobPaused(ctx, jobID); err != nil {
		m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "failed", "error": err.Error()})
		return
	}
	m.broadcast(jobID, "job_status", map[string]any{"job_id": jobID, "status": "paused"})
}

func (m *Manager) broadcast(jobID int64, eventType string, data any) {
	m.mu.RLock()
	state, ok := m.jobs[jobID]
	if !ok {
		m.mu.RUnlock()
		return
	}
	subs := append([]chan Event(nil), state.subscribers...)
	m.mu.RUnlock()

	event := Event{Type: eventType, Data: data}
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (m *Manager) beginRun(jobID int64, cancel context.CancelFunc) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.ensureJobStateLocked(jobID)
	if state.running {
		return false
	}
	state.cancel = cancel
	state.pauseRequested = false
	state.running = true
	return true
}

func (m *Manager) resumeRun(jobID int64, cancel context.CancelFunc) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.ensureJobStateLocked(jobID)
	if state.running {
		state.pauseRequested = false
		return false
	}
	state.cancel = cancel
	state.pauseRequested = false
	state.running = true
	return true
}

func (m *Manager) finishRun(jobID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.jobs[jobID]
	if !ok {
		return
	}
	state.running = false
	state.cancel = nil
	if !state.pauseRequested && len(state.subscribers) == 0 {
		delete(m.jobs, jobID)
	}
}

func (m *Manager) isPauseRequested(jobID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.jobs[jobID]
	return ok && state.pauseRequested
}

func (m *Manager) ensureJobStateLocked(jobID int64) *jobState {
	state, ok := m.jobs[jobID]
	if !ok {
		state = &jobState{}
		m.jobs[jobID] = state
	}
	return state
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type postgresStore struct {
	pool *pgxpool.Pool
}

func (s *postgresStore) GetConcurrency(ctx context.Context) (int, error) {
	if s.pool == nil {
		return 0, fmt.Errorf("jobmanager: nil pool")
	}
	var concurrency int
	err := s.pool.QueryRow(ctx, "SELECT concurrency FROM app_settings WHERE id=1").Scan(&concurrency)
	if err != nil {
		return 0, err
	}
	return concurrency, nil
}

func (s *postgresStore) PendingChapterIDs(ctx context.Context, jobID int64) ([]int64, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("jobmanager: nil pool")
	}
	rows, err := s.pool.Query(ctx, `SELECT c.id
		FROM job_chapters jc
		JOIN chapters c ON c.id = jc.chapter_id
		WHERE jc.job_id = $1
		  AND c.dehydrate_status IN ('pending', 'failed', 'processing')
		ORDER BY c.seq`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chapterIDs := make([]int64, 0)
	for rows.Next() {
		var chapterID int64
		if err := rows.Scan(&chapterID); err != nil {
			return nil, err
		}
		chapterIDs = append(chapterIDs, chapterID)
	}
	return chapterIDs, rows.Err()
}

func (s *postgresStore) GetBookMeta(ctx context.Context, chapterID int64) (bookMeta, error) {
	if s.pool == nil {
		return bookMeta{}, fmt.Errorf("jobmanager: nil pool")
	}
	var meta bookMeta
	err := s.pool.QueryRow(ctx, `SELECT b.id, b.title, b.total_chapters
		FROM chapters c
		JOIN books b ON b.id = c.book_id
		WHERE c.id = $1`, chapterID).Scan(&meta.BookID, &meta.BookTitle, &meta.TotalChapters)
	return meta, err
}

func (s *postgresStore) GetChapter(ctx context.Context, chapterID int64) (chapterRecord, error) {
	if s.pool == nil {
		return chapterRecord{}, fmt.Errorf("jobmanager: nil pool")
	}
	var chapter chapterRecord
	err := s.pool.QueryRow(ctx, `SELECT id, book_id, title, raw_path, seq
		FROM chapters
		WHERE id=$1`, chapterID).Scan(&chapter.ID, &chapter.BookID, &chapter.Title, &chapter.RawPath, &chapter.Seq)
	return chapter, err
}

func (s *postgresStore) GetJobCounts(ctx context.Context, jobID int64) (jobCounts, error) {
	if s.pool == nil {
		return jobCounts{}, fmt.Errorf("jobmanager: nil pool")
	}
	var counts jobCounts
	err := s.pool.QueryRow(ctx, "SELECT done_count, failed_count, total_count FROM jobs WHERE id=$1", jobID).Scan(&counts.Done, &counts.Failed, &counts.Total)
	return counts, err
}

func (s *postgresStore) MarkJobRunning(ctx context.Context, jobID int64) error {
	return s.updateJobStatus(ctx, jobID, "running")
}

func (s *postgresStore) MarkJobPaused(ctx context.Context, jobID int64) error {
	return s.updateJobStatus(ctx, jobID, "paused")
}

func (s *postgresStore) MarkJobCancelled(ctx context.Context, jobID int64) error {
	return s.updateJobStatus(ctx, jobID, "cancelled")
}

func (s *postgresStore) MarkJobCompleted(ctx context.Context, jobID int64) error {
	return s.updateJobStatus(ctx, jobID, "completed")
}

func (s *postgresStore) MarkChapterProcessing(ctx context.Context, jobID, chapterID int64) error {
	if s.pool == nil {
		return fmt.Errorf("jobmanager: nil pool")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE chapters SET dehydrate_status='processing' WHERE id=$1", chapterID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE jobs SET current_chapter_id=$1, updated_at=NOW() WHERE id=$2", chapterID, jobID)
		return err
	})
}

func (s *postgresStore) MarkChapterDone(ctx context.Context, jobID, chapterID int64, dehydratedPath string, dehydratedChars int, compressionRatio float64) error {
	if s.pool == nil {
		return fmt.Errorf("jobmanager: nil pool")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE chapters
			SET dehydrate_status='done', dehydrated_path=$1, dehydrated_char_count=$2,
			    compression_ratio=$3, processed_at=NOW()
			WHERE id=$4`, dehydratedPath, dehydratedChars, compressionRatio, chapterID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE jobs SET done_count=done_count+1, updated_at=NOW() WHERE id=$1", jobID)
		return err
	})
}

func (s *postgresStore) MarkChapterFailed(ctx context.Context, jobID, chapterID int64, errMsg string) error {
	if s.pool == nil {
		return fmt.Errorf("jobmanager: nil pool")
	}
	return withTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE chapters
			SET dehydrate_status='failed', error_msg=$1, retry_count=retry_count+1
			WHERE id=$2`, errMsg, chapterID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE jobs SET failed_count=failed_count+1, updated_at=NOW() WHERE id=$1", jobID)
		return err
	})
}

func (s *postgresStore) updateJobStatus(ctx context.Context, jobID int64, status string) error {
	if s.pool == nil {
		return fmt.Errorf("jobmanager: nil pool")
	}
	_, err := s.pool.Exec(ctx, "UPDATE jobs SET status=$1, updated_at=NOW() WHERE id=$2", status, jobID)
	return err
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
