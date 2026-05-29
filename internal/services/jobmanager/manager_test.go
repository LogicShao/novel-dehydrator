package jobmanager

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartAndComplete(t *testing.T) {
	manager, store, files, _ := newTestManager(3, 3)
	files.setRaw(1, 1, "raw one")
	files.setRaw(1, 2, "raw two")
	files.setRaw(1, 3, "raw three")

	manager.StartJob(context.Background(), 1)
	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "completed"
	})

	for _, chapterID := range []int64{1, 2, 3} {
		if got := store.chapterStatus(chapterID); got != "done" {
			t.Fatalf("chapter %d status = %q, want done", chapterID, got)
		}
	}
	counts := store.counts(1)
	if counts.Done != 3 || counts.Failed != 0 || counts.Total != 3 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestPauseResume(t *testing.T) {
	manager, store, files, dehydrator := newTestManager(3, 1)
	for _, chapterID := range []int64{1, 2, 3} {
		files.setRaw(1, chapterID, fmt.Sprintf("raw %d", chapterID))
	}

	started := make(chan int64, 3)
	release := make(chan struct{})
	dehydrator.fn = func(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error) {
		started <- int64(chapterSeq)
		<-release
		return "done:" + rawText, nil
	}

	manager.StartJob(context.Background(), 1)
	<-started
	manager.PauseJob(1)
	close(release)

	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "paused"
	})

	if got := store.counts(1).Done; got != 1 {
		t.Fatalf("done before resume = %d, want 1", got)
	}

	manager.ResumeJob(context.Background(), 1)
	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "completed"
	})

	if got := store.counts(1).Done; got != 3 {
		t.Fatalf("done after resume = %d, want 3", got)
	}
}

func TestCancel(t *testing.T) {
	manager, store, files, dehydrator := newTestManager(1, 1)
	files.setRaw(1, 1, "raw")

	ctxSeen := make(chan error, 1)
	dehydrator.fn = func(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error) {
		<-ctx.Done()
		ctxSeen <- ctx.Err()
		return "", ctx.Err()
	}

	manager.StartJob(context.Background(), 1)
	waitFor(t, time.Second, func() bool {
		return store.chapterStatus(1) == "processing"
	})
	manager.CancelJob(1)

	select {
	case err := <-ctxSeen:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ctx err = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancellation")
	}

	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "cancelled"
	})
}

func TestStartJobDetachesFromCallerContext(t *testing.T) {
	manager, store, files, _ := newTestManager(1, 1)
	files.setRaw(1, 1, "raw")

	ctx, cancel := context.WithCancel(context.Background())
	manager.StartJob(ctx, 1)
	cancel()

	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "completed"
	})
	if got := store.chapterStatus(1); got != "done" {
		t.Fatalf("chapter status = %q, want done", got)
	}
}

func TestSubscribeProgress(t *testing.T) {
	manager, store, files, _ := newTestManager(1, 1)
	files.setRaw(1, 1, "raw")

	ch := manager.Subscribe(1)
	defer manager.Unsubscribe(1, ch)

	manager.StartJob(context.Background(), 1)

	types := map[string]bool{}
	timeout := time.After(time.Second)
	for len(types) < 2 {
		select {
		case event := <-ch:
			types[event.Type] = true
		case <-timeout:
			t.Fatalf("timed out waiting for events, got %v", types)
		}
	}

	if !types["progress"] {
		t.Fatalf("missing progress event")
	}
	if !types["chapter_done"] {
		t.Fatalf("missing chapter_done event")
	}
	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "completed"
	})
}

func TestConcurrencyLimit(t *testing.T) {
	manager, store, files, dehydrator := newTestManager(3, 1)
	for _, chapterID := range []int64{1, 2, 3} {
		files.setRaw(1, chapterID, fmt.Sprintf("raw %d", chapterID))
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	release := make(chan struct{})
	dehydrator.fn = func(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error) {
		current := active.Add(1)
		for {
			seen := maxActive.Load()
			if current <= seen || maxActive.CompareAndSwap(seen, current) {
				break
			}
		}
		<-release
		active.Add(-1)
		return "done:" + rawText, nil
	}

	manager.StartJob(context.Background(), 1)
	time.Sleep(100 * time.Millisecond)
	close(release)
	waitFor(t, time.Second, func() bool {
		return store.jobStatus(1) == "completed"
	})

	if got := maxActive.Load(); got != 1 {
		t.Fatalf("max concurrent chapters = %d, want 1", got)
	}
}

type mockStore struct {
	mu          sync.Mutex
	concurrency int
	jobMap      map[int64]*mockJob
	chapterMap  map[int64]*mockChapter
	bookID      int64
	bookTitle   string
	total       int
}

type mockJob struct {
	status    string
	chapters  []int64
	done      int
	failed    int
	total     int
	currentID int64
}

type mockChapter struct {
	chapterRecord
	status           string
	dehydratedPath   string
	dehydratedChars  int
	compressionRatio float64
	errorMsg         string
	retryCount       int
}

func newMockStore(totalChapters, concurrency int) *mockStore {
	store := &mockStore{
		concurrency: concurrency,
		jobMap:      map[int64]*mockJob{},
		chapterMap:  map[int64]*mockChapter{},
		bookID:      1,
		bookTitle:   "Book",
		total:       totalChapters,
	}
	job := &mockJob{status: "running", total: totalChapters}
	for i := 1; i <= totalChapters; i++ {
		chapterID := int64(i)
		job.chapters = append(job.chapters, chapterID)
		store.chapterMap[chapterID] = &mockChapter{
			chapterRecord: chapterRecord{
				ID:      chapterID,
				BookID:  1,
				Title:   fmt.Sprintf("Chapter %d", i),
				RawPath: filepath.Join("raw", fmt.Sprintf("%d.txt", i)),
				Seq:     i,
			},
			status: "pending",
		}
	}
	store.jobMap[1] = job
	return store
}

func (s *mockStore) GetConcurrency(context.Context) (int, error) { return s.concurrency, nil }

func (s *mockStore) PendingChapterIDs(_ context.Context, jobID int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobMap[jobID]
	out := make([]int64, 0, len(job.chapters))
	for _, chapterID := range job.chapters {
		status := s.chapterMap[chapterID].status
		if status == "pending" || status == "failed" || status == "processing" {
			out = append(out, chapterID)
		}
	}
	return out, nil
}

func (s *mockStore) GetBookMeta(context.Context, int64) (bookMeta, error) {
	return bookMeta{BookID: s.bookID, BookTitle: s.bookTitle, TotalChapters: s.total}, nil
}

func (s *mockStore) GetChapter(_ context.Context, chapterID int64) (chapterRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chapter, ok := s.chapterMap[chapterID]
	if !ok {
		return chapterRecord{}, fmt.Errorf("missing chapter %d", chapterID)
	}
	return chapter.chapterRecord, nil
}

func (s *mockStore) GetJobCounts(_ context.Context, jobID int64) (jobCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobMap[jobID]
	return jobCounts{Done: job.done, Failed: job.failed, Total: job.total}, nil
}

func (s *mockStore) MarkJobRunning(_ context.Context, jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobMap[jobID].status = "running"
	return nil
}

func (s *mockStore) MarkJobPaused(_ context.Context, jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobMap[jobID].status = "paused"
	return nil
}

func (s *mockStore) MarkJobCancelled(_ context.Context, jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobMap[jobID].status = "cancelled"
	return nil
}

func (s *mockStore) MarkJobCompleted(_ context.Context, jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobMap[jobID].status = "completed"
	return nil
}

func (s *mockStore) MarkChapterProcessing(_ context.Context, jobID, chapterID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chapterMap[chapterID].status = "processing"
	s.jobMap[jobID].currentID = chapterID
	return nil
}

func (s *mockStore) MarkChapterDone(_ context.Context, jobID, chapterID int64, dehydratedPath string, dehydratedChars int, compressionRatio float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chapter := s.chapterMap[chapterID]
	chapter.status = "done"
	chapter.dehydratedPath = dehydratedPath
	chapter.dehydratedChars = dehydratedChars
	chapter.compressionRatio = compressionRatio
	s.jobMap[jobID].done++
	return nil
}

func (s *mockStore) MarkChapterFailed(_ context.Context, jobID, chapterID int64, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chapter := s.chapterMap[chapterID]
	chapter.status = "failed"
	chapter.errorMsg = errMsg
	chapter.retryCount++
	s.jobMap[jobID].failed++
	return nil
}

func (s *mockStore) chapterStatus(chapterID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chapterMap[chapterID].status
}

func (s *mockStore) jobStatus(jobID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobMap[jobID].status
}

func (s *mockStore) counts(jobID int64) jobCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobMap[jobID]
	return jobCounts{Done: job.done, Failed: job.failed, Total: job.total}
}

type mockStorage struct {
	mu      sync.Mutex
	content map[string]string
	writes  map[string]string
}

func newMockStorage() *mockStorage {
	return &mockStorage{content: map[string]string{}, writes: map[string]string{}}
}

func (s *mockStorage) setRaw(bookID, chapterID int64, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.content[filepath.Join("raw", fmt.Sprintf("%d.txt", chapterID))] = content
}

func (s *mockStorage) ReadChapter(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.content[path]
	if !ok {
		return "", fmt.Errorf("missing file %s", path)
	}
	return content, nil
}

func (s *mockStorage) WriteDehydratedChapter(path, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes[path] = content
	return nil
}

func (s *mockStorage) DehydratedChapterPath(dataDir string, bookID, chapterID int64) string {
	return filepath.Join(dataDir, "books", fmt.Sprintf("%d", bookID), "dehydrated", fmt.Sprintf("%d.txt", chapterID))
}

type mockDehydrator struct {
	fn func(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error)
}

func (d *mockDehydrator) DehydrateChapter(ctx context.Context, bookTitle, chapterTitle, rawText string, chapterSeq, totalChapters int) (string, error) {
	if d.fn != nil {
		return d.fn(ctx, bookTitle, chapterTitle, rawText, chapterSeq, totalChapters)
	}
	return "done:" + rawText, nil
}

func newTestManager(totalChapters, concurrency int) (*Manager, *mockStore, *mockStorage, *mockDehydrator) {
	store := newMockStore(totalChapters, concurrency)
	storage := newMockStorage()
	dehydrator := &mockDehydrator{}
	manager := &Manager{
		dataDir:    "data",
		dehydrator: dehydrator,
		storage:    storage,
		store:      store,
		jobs:       make(map[int64]*jobState),
	}
	return manager, store, storage, dehydrator
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
