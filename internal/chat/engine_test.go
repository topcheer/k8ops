package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/memory"
	"github.com/ggai/k8ops/internal/provider"
)

// mockProvider satisfies provider.Provider for conversation creation in tests.
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Complete(_ context.Context, _ provider.CompletionRequest) (*provider.CompletionResponse, error) {
	return &provider.CompletionResponse{Content: "mock"}, nil
}
func (m *mockProvider) StreamComplete(_ context.Context, _ provider.CompletionRequest, _ func(string)) (*provider.CompletionResponse, error) {
	return &provider.CompletionResponse{Content: "mock"}, nil
}

func newTestEngine() *Engine {
	return &Engine{
		conversations: make(map[string]*memory.Conversation),
		provider:      func() provider.Provider { return &mockProvider{} },
		systemPrompt:  "test",
		maxSteps:      15,
		timeout:       180 * time.Second,
	}
}

// --- TTL Eviction Tests ---

func TestEvictIdle_RemovesExpiredConversations(t *testing.T) {
	e := newTestEngine()

	conv := e.GetOrCreateConversation("old-session")
	if conv == nil {
		t.Fatal("failed to create conversation")
	}
	setLastActivityForTest(e, "old-session", time.Now().Add(-1*time.Hour))

	fresh := e.GetOrCreateConversation("fresh-session")
	if fresh == nil {
		t.Fatal("failed to create fresh conversation")
	}

	evicted := e.evictIdle()
	if evicted != 1 {
		t.Fatalf("evictIdle() = %d, want 1", evicted)
	}

	e.mu.RLock()
	_, oldExists := e.conversations["old-session"]
	_, freshExists := e.conversations["fresh-session"]
	e.mu.RUnlock()

	if oldExists {
		t.Error("expired conversation was not evicted")
	}
	if !freshExists {
		t.Error("fresh conversation was incorrectly evicted")
	}
}

func TestEvictIdle_KeepsRecentConversations(t *testing.T) {
	e := newTestEngine()

	e.GetOrCreateConversation("recent")
	setLastActivityForTest(e, "recent", time.Now().Add(-5*time.Minute))

	evicted := e.evictIdle()
	if evicted != 0 {
		t.Fatalf("evictIdle() = %d, want 0", evicted)
	}
}

func TestEvictIdle_BoundaryJustOver30Min(t *testing.T) {
	e := newTestEngine()

	e.GetOrCreateConversation("boundary")
	setLastActivityForTest(e, "boundary", time.Now().Add(-31*time.Minute))

	evicted := e.evictIdle()
	if evicted != 1 {
		t.Fatalf("evictIdle() = %d, want 1", evicted)
	}
}

func TestEvictIdle_MultipleExpired(t *testing.T) {
	e := newTestEngine()

	for i := 0; i < 5; i++ {
		id := "expired-" + string(rune('a'+i))
		e.GetOrCreateConversation(id)
		setLastActivityForTest(e, id, time.Now().Add(-2*time.Hour))
	}
	for i := 0; i < 3; i++ {
		id := "fresh-" + string(rune('a'+i))
		e.GetOrCreateConversation(id)
		setLastActivityForTest(e, id, time.Now().Add(-1*time.Minute))
	}

	evicted := e.evictIdle()
	if evicted != 5 {
		t.Fatalf("evictIdle() = %d, want 5", evicted)
	}

	e.mu.RLock()
	remaining := len(e.conversations)
	e.mu.RUnlock()

	if remaining != 3 {
		t.Errorf("remaining = %d, want 3", remaining)
	}
}

// --- Cap Enforcement ---

func TestEvictOldest_RemovesOldest(t *testing.T) {
	e := newTestEngine()

	e.GetOrCreateConversation("oldest")
	e.GetOrCreateConversation("middle")
	e.GetOrCreateConversation("newest")

	setLastActivityForTest(e, "oldest", time.Now().Add(-10*time.Minute))
	setLastActivityForTest(e, "middle", time.Now().Add(-5*time.Minute))
	setLastActivityForTest(e, "newest", time.Now().Add(-1*time.Minute))

	e.mu.Lock()
	e.evictOldestLocked()
	e.mu.Unlock()

	e.mu.RLock()
	_, hasOldest := e.conversations["oldest"]
	_, hasMiddle := e.conversations["middle"]
	_, hasNewest := e.conversations["newest"]
	e.mu.RUnlock()

	if hasOldest {
		t.Error("oldest should have been evicted")
	}
	if !hasMiddle || !hasNewest {
		t.Error("middle and newest should remain")
	}
}

// --- Start/Stop Cleanup ---

func TestStartStopCleanup_GracefulShutdown(t *testing.T) {
	e := newTestEngine()

	e.StartCleanup()

	e.mu.RLock()
	stopCh := e.cleanupStop
	e.mu.RUnlock()
	if stopCh == nil {
		t.Fatal("cleanupStop should be set after StartCleanup")
	}

	done := make(chan struct{})
	go func() {
		e.StopCleanup()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("StopCleanup did not return within 5 seconds")
	}

	e.mu.RLock()
	stopAfter := e.cleanupStop
	e.mu.RUnlock()
	if stopAfter != nil {
		t.Error("cleanupStop should be nil after StopCleanup")
	}
}

func TestStopCleanup_WhenNotStarted(t *testing.T) {
	e := newTestEngine()

	done := make(chan struct{})
	go func() {
		e.StopCleanup()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("StopCleanup on non-started engine should return immediately")
	}
}

func TestStartCleanup_Idempotent(t *testing.T) {
	e := newTestEngine()
	e.StartCleanup()
	e.StartCleanup() // should be no-op
	e.StopCleanup()
}

// --- Concurrency ---

func TestConcurrentAccess_NoRace(t *testing.T) {
	e := newTestEngine()
	e.StartCleanup()
	defer e.StopCleanup()

	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('a'+n%26))
			conv := e.GetOrCreateConversation(id)
			if conv != nil {
				conv.AddUserMessage("test message")
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.ConversationStats()
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e.DeleteConversation("concurrent-" + string(rune('a'+n%26)))
		}(i)
	}

	wg.Wait()
}

// --- LastActivity ---

func TestConversationLastActivity_UpdatedOnUserMessage(t *testing.T) {
	e := newTestEngine()

	conv := e.GetOrCreateConversation("activity-test")
	if conv == nil {
		t.Fatal("failed to create conversation")
	}

	before := conv.LastActivity()
	time.Sleep(10 * time.Millisecond)
	conv.AddUserMessage("hello")
	after := conv.LastActivity()

	if !after.After(before) {
		t.Error("LastActivity should be updated after AddUserMessage")
	}
}

// --- Helper ---

func setLastActivityForTest(e *Engine, id string, when time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if conv, ok := e.conversations[id]; ok {
		conv.SetLastActivityForTest(when)
	}
}
