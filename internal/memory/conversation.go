// Package memory provides long-term conversation memory with automatic
// context compression. When conversation exceeds a token threshold,
// older messages are summarized into a compact memory block.
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ggai/k8ops/internal/provider"
)

// Conversation maintains a multi-turn conversation with automatic compression.
type Conversation struct {
	mu sync.Mutex

	id       string
	provider provider.Provider
	log      *slog.Logger

	// The message list sent to the LLM: [system, summary?, ...recent messages]
	messages []provider.Message

	// Token estimate for current messages (rough: 1 token ~ 4 chars)
	tokenEstimate int

	// Configuration
	maxTokens      int // threshold to trigger compression
	keepRecent     int // messages to keep uncompressed
	summarizeModel string

	// Persistent memory: key findings and decisions across compressions
	memory []MemoryItem

	// Created/updated timestamps
	createdAt time.Time
	updatedAt time.Time
}

// MemoryItem is a persisted piece of knowledge from earlier in the conversation.
type MemoryItem struct {
	Type      string `json:"type"` // "finding", "decision", "fact"
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// NewConversation creates a new conversation with memory.
func NewConversation(id string, p provider.Provider, systemPrompt string, log *slog.Logger) *Conversation {
	return &Conversation{
		id:         id,
		provider:   p,
		log:        log,
		messages:   []provider.Message{{Role: provider.RoleSystem, Content: systemPrompt}},
		maxTokens:  20000, // trigger compression at ~20k tokens
		keepRecent: 8,     // keep last 8 messages uncompressed
		createdAt:  time.Now(),
		updatedAt:  time.Now(),
	}
}

// AddUserMessage adds a user message and triggers compression if needed.
func (c *Conversation) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, provider.Message{
		Role: provider.RoleUser, Content: content,
	})
	c.tokenEstimate += estimateTokens(content)
	c.updatedAt = time.Now()
}

// AddAssistantMessage adds an assistant response.
func (c *Conversation) AddAssistantMessage(msg provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, msg)
	c.tokenEstimate += estimateTokens(msg.Content)
	c.updatedAt = time.Now()
}

// AddToolResult adds a tool result message.
func (c *Conversation) AddToolResult(toolCallID, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, provider.Message{
		Role: provider.RoleTool, ToolCallID: toolCallID, Content: content,
	})
	c.tokenEstimate += estimateTokens(content)
	c.updatedAt = time.Now()
}

// Messages returns a copy of current messages for LLM submission.
func (c *Conversation) Messages() []provider.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]provider.Message, len(c.messages))
	copy(result, c.messages)
	return result
}

// MaybeCompress checks if compression is needed and performs it.
// Returns true if compression was performed.
func (c *Conversation) MaybeCompress(ctx context.Context) (bool, error) {
	c.mu.Lock()
	needed := c.tokenEstimate > c.maxTokens && len(c.messages) > c.keepRecent+1
	c.mu.Unlock()

	if !needed {
		return false, nil
	}
	return c.compress(ctx)
}

// compress summarizes older messages into a compact memory block.
func (c *Conversation) compress(ctx context.Context) (bool, error) {
	c.mu.Lock()

	// Identify messages to compress (everything except system + keepRecent)
	systemMsg := c.messages[0]
	totalMsgs := len(c.messages) - 1 // excluding system
	if totalMsgs <= c.keepRecent {
		c.mu.Unlock()
		return false, nil
	}

	toCompressCount := totalMsgs - c.keepRecent
	toCompress := c.messages[1 : 1+toCompressCount]
	recent := c.messages[1+toCompressCount:]

	// Build compression prompt
	var compressBuilder strings.Builder
	compressBuilder.WriteString("You are a conversation summarizer. Summarize the following conversation history into:\n")
	compressBuilder.WriteString("1. Key findings and diagnoses (as bullet points)\n")
	compressBuilder.WriteString("2. Actions taken and their results\n")
	compressBuilder.WriteString("3. Important context that should be remembered\n\n")
	compressBuilder.WriteString("Be concise but preserve all critical technical details (pod names, error messages, IPs).\n\n")
	compressBuilder.WriteString("--- CONVERSATION HISTORY ---\n\n")

	for _, msg := range toCompress {
		role := string(msg.Role)
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		compressBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				compressBuilder.WriteString(fmt.Sprintf("  -> Called tool: %s(%s)\n", tc.Name, tc.Arguments))
			}
		}
	}

	prompt := compressBuilder.String()
	c.mu.Unlock()

	// Call LLM to summarize
	resp, err := c.provider.Complete(ctx, provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		Temperature: 0.0,
		MaxTokens:   1500,
	})
	if err != nil {
		c.log.Error("context compression failed", "error", err)
		return false, fmt.Errorf("compression LLM call failed: %w", err)
	}

	summary := resp.Content

	// Record as memory item
	now := time.Now().Format(time.RFC3339)
	c.mu.Lock()
	c.memory = append(c.memory, MemoryItem{
		Type: "summary", Content: summary, Timestamp: now,
	})

	// Rebuild messages: [system, summary_block, ...recent]
	summaryMsg := provider.Message{
		Role: provider.RoleSystem,
		Content: fmt.Sprintf("[COMPRESSED CONTEXT - %d messages compressed]\n"+
			"Previous conversation summary:\n%s\n"+
			"--- End of compressed context. Continue from here. ---", toCompressCount, summary),
	}

	c.messages = append([]provider.Message{systemMsg, summaryMsg}, recent...)
	c.tokenEstimate = estimateTokens(summary) + c.estimateMessagesTokens(recent)
	c.updatedAt = time.Now()
	c.mu.Unlock()

	c.log.Info("context compressed",
		"compressedMessages", toCompressCount,
		"remainingMessages", len(recent),
		"summaryTokens", estimateTokens(summary),
		"totalTokens", c.tokenEstimate,
	)

	return true, nil
}

func (c *Conversation) estimateMessagesTokens(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m.Content)
	}
	return total
}

// Memory returns persistent memory items.
func (c *Conversation) Memory() []MemoryItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]MemoryItem, len(c.memory))
	copy(result, c.memory)
	return result
}

// AddMemory adds a persistent memory item.
func (c *Conversation) AddMemory(itemType, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memory = append(c.memory, MemoryItem{
		Type:      itemType,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// ID returns the conversation ID.
func (c *Conversation) ID() string { return c.id }

// LastActivity returns the last time the conversation received a user or
// assistant message. Used by the engine's TTL cleanup to evict idle sessions.
func (c *Conversation) LastActivity() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.updatedAt
}

// SetLastActivityForTest sets the updatedAt timestamp for testing purposes.
// This allows tests to simulate stale conversations without waiting in real time.
func (c *Conversation) SetLastActivityForTest(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updatedAt = t
}

// Stats returns conversation statistics.
type Stats struct {
	ID            string `json:"id"`
	MessageCount  int    `json:"messageCount"`
	MemoryCount   int    `json:"memoryCount"`
	TokenEstimate int    `json:"tokenEstimate"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Compressed    bool   `json:"compressed"`
}

func (c *Conversation) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{
		ID:            c.id,
		MessageCount:  len(c.messages),
		MemoryCount:   len(c.memory),
		TokenEstimate: c.tokenEstimate,
		CreatedAt:     c.createdAt.Format(time.RFC3339),
		UpdatedAt:     c.updatedAt.Format(time.RFC3339),
		Compressed:    len(c.memory) > 0,
	}
}

// estimateTokens provides a rough token estimate (1 token ≈ 4 chars).
func estimateTokens(s string) int {
	return len(s) / 4
}
