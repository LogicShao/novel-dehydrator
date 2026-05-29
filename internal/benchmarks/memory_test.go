package benchmarks

import (
	"math/rand"
	"sync"
	"testing"
)

// generate10MBStrings creates a slice of strings totaling approximately 10MB.
func generate10MBStrings() []string {
	const targetBytes = 10 * 1024 * 1024 // 10 MB
	const avgLineLen = 80
	numLines := targetBytes / avgLineLen

	lines := make([]string, numLines)
	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789  \n")

	var total int
	for i := range lines {
		// random line length 40-120 chars
		n := 40 + rand.Intn(80)
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = letters[rand.Intn(len(letters))]
		}
		lines[i] = string(buf)
		total += len(lines[i])
	}
	return lines
}

// BenchmarkParse10MBFile simulates parsing a 10MB text file.
func BenchmarkParse10MBFile(b *testing.B) {
	lines := generate10MBStrings()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var totalLen int
		for _, line := range lines {
			totalLen += len(line)
		}
		_ = totalLen
	}
}

// chapter simulates a single chapter to be dehydrated.
type chapter struct {
	ID      int
	Title   string
	Content string
	Summary string
}

// BenchmarkConcurrentDehydrate simulates 10 goroutines processing 100 chapters each.
func BenchmarkConcurrentDehydrate(b *testing.B) {
	// Pre-create 1000 chapters outside the benchmark loop.
	chapters := make([]chapter, 1000)
	for i := range chapters {
		chapters[i] = chapter{
			ID:      i,
			Title:   "Chapter Title " + string(rune('A'+i%26)),
			Content: randString(4096),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for g := 0; g < 10; g++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				start := gid * 100
				end := start + 100
				for _, ch := range chapters[start:end] {
					ch.Summary = randString(256)
					_ = ch.Summary
				}
			}(g)
		}
		wg.Wait()
	}
}

// subscriber simulates an SSE subscriber that receives events.
type subscriber struct {
	ID      int
	BufSize int
	Events  []sseEvent
	mu      sync.Mutex
}

type sseEvent struct {
	Type string
	Data string
}

// BenchmarkSSEBroadcast simulates 100 subscribers receiving events.
func BenchmarkSSEBroadcast(b *testing.B) {
	const numSubscribers = 100
	const eventsPerBatch = 10

	// Pre-create subscribers outside the timer.
	subs := make([]subscriber, numSubscribers)
	for i := range subs {
		subs[i] = subscriber{
			ID:      i,
			BufSize: 64,
			Events:  make([]sseEvent, 0, eventsPerBatch*2),
		}
	}

	event := sseEvent{Type: "progress", Data: randString(512)}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for j := range subs {
			wg.Add(1)
			go func(s *subscriber) {
				defer wg.Done()
				s.mu.Lock()
				s.Events = append(s.Events, event)
				// Drop old events if buffer exceeds capacity.
				if len(s.Events) > s.BufSize {
					s.Events = s.Events[len(s.Events)-s.BufSize:]
				}
				s.mu.Unlock()
			}(&subs[j])
		}
		wg.Wait()
	}
}

// dbRow simulates a database query result row.
type dbRow struct {
	ID          int
	BookID      int
	ChapterNum  int
	Summary     string
	Characters  string
	Foreshadow  string
	WordCount   int
	ProcessedAt string
}

// BenchmarkDBAggregation simulates aggregating 10K database rows.
func BenchmarkDBAggregation(b *testing.B) {
	const numRows = 10_000

	// Pre-create rows outside the timer.
	rows := make([]dbRow, numRows)
	for i := range rows {
		rows[i] = dbRow{
			ID:          i,
			BookID:      i % 50,
			ChapterNum:  i % 200,
			Summary:     randString(512),
			Characters:  randString(128),
			Foreshadow:  randString(256),
			WordCount:   rand.Intn(10000),
			ProcessedAt: "2026-05-28T12:00:00Z",
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Aggregate by BookID: count chapters and sum word counts.
		agg := make(map[int]struct {
			ChapterCount int
			TotalWords   int
		}, 50)

		for _, r := range rows {
			entry := agg[r.BookID]
			entry.ChapterCount++
			entry.TotalWords += r.WordCount
			agg[r.BookID] = entry
		}
		_ = len(agg)
	}
}

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
