package dehydrator

import (
	"context"
	"strings"
	"testing"

	"github.com/LogicShao/novel-dehydrator/internal/services/deepseek"
	"github.com/LogicShao/novel-dehydrator/internal/services/prompts"
)

type mockClient struct {
	responses []string
	calls     [][]deepseek.Message
	streams   []bool
}

func (m *mockClient) ChatCompletion(_ context.Context, messages []deepseek.Message, stream bool) (string, error) {
	cloned := append([]deepseek.Message(nil), messages...)
	m.calls = append(m.calls, cloned)
	m.streams = append(m.streams, stream)
	if len(m.responses) == 0 {
		return "", nil
	}
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response, nil
}

func TestShortChapter(t *testing.T) {
	client := &mockClient{responses: []string{"脱水内容\n---CHAPTER_META---\n本章剧情：测试"}}
	svc := New(nil, client, 100)

	got, err := svc.DehydrateChapter(context.Background(), "书名", "第一章", "一段短内容", 3, 20)
	if err != nil {
		t.Fatalf("DehydrateChapter() error = %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("ChatCompletion call count = %d, want 1", len(client.calls))
	}
	if !client.streams[0] {
		t.Fatal("ChatCompletion stream = false, want true")
	}
	if client.calls[0][0].Content != prompts.DehydrateSystem {
		t.Fatalf("system prompt mismatch")
	}
	if strings.Contains(client.calls[0][1].Content, "这是第1段") {
		t.Fatalf("short chapter should not include chunk hint")
	}
	if !strings.HasSuffix(got, "\n---CHAPTER_META---\n") {
		t.Fatalf("result suffix = %q, want chapter meta delimiter", got)
	}
	if !strings.Contains(got, "本章剧情：测试") {
		t.Fatalf("result should preserve meta content, got %q", got)
	}
}

func TestLongChapterChunking(t *testing.T) {
	text := strings.Join([]string{
		"第一段很长很长很长",
		"第二段很长很长很长",
		"第三段很长很长很长",
	}, "\n\n")
	client := &mockClient{responses: []string{"结果1", "结果2", "结果3"}}
	svc := New(nil, client, len("第一段很长很长很长")+1)

	got, err := svc.DehydrateChapter(context.Background(), "书名", "第二章", text, 10, 100)
	if err != nil {
		t.Fatalf("DehydrateChapter() error = %v", err)
	}

	if len(client.calls) != 3 {
		t.Fatalf("ChatCompletion call count = %d, want 3", len(client.calls))
	}
	for i, call := range client.calls {
		wantHint := "这是第" + string(rune('1'+i)) + "段"
		if !strings.Contains(call[1].Content, wantHint) {
			t.Fatalf("chunk %d user prompt missing hint %q", i+1, wantHint)
		}
	}
	if !strings.Contains(got, "结果1\n\n结果2\n\n结果3") {
		t.Fatalf("combined result = %q, want joined chunk results", got)
	}
}

func TestEarlyChapterPrompt(t *testing.T) {
	client := &mockClient{responses: []string{"脱水内容"}}
	svc := New(nil, client, 100)

	_, err := svc.DehydrateChapter(context.Background(), "书名", "第一章", "早期内容", 1, 100)
	if err != nil {
		t.Fatalf("DehydrateChapter() error = %v", err)
	}

	userContent := client.calls[0][1].Content
	if !strings.Contains(userContent, prompts.PositionHintEarly) {
		t.Fatalf("user prompt missing early hint")
	}
	if strings.Contains(userContent, prompts.PositionHintNormal) {
		t.Fatalf("user prompt unexpectedly contains normal hint")
	}
}
