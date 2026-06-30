// Package anthropic implements an Anthropic Claude-compatible chat provider.
// Any vendor with an Anthropic-compatible API works with this provider.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ggai/k8ops/internal/provider"
)

const (
	defaultEndpoint = "https://api.anthropic.com"
	defaultModel    = "claude-3-5-sonnet-20241022"
	apiVersion      = "2023-06-01"
	defaultTimeout  = 120 * time.Second
)

func init() {
	provider.Register("anthropic", New)
}

// AnthropicProvider implements provider.Provider for Anthropic-compatible APIs.
type AnthropicProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

func New(cfg provider.ProviderConfig) (provider.Provider, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	return &AnthropicProvider{
		apiKey:   cfg.APIKey,
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: defaultTimeout},
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

// Anthropic API request/response types
type antContent struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
}

type antMessage struct {
	Role    string       `json:"role"`
	Content []antContent `json:"content"`
}

type antTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type antRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	System      string       `json:"system,omitempty"`
	Messages    []antMessage `json:"messages"`
	Tools       []antTool    `json:"tools,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
}

type antResponse struct {
	Content    []antContent `json:"content"`
	StopReason string       `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	var systemPrompt string
	var antMsgs []antMessage

	for _, msg := range req.Messages {
		if msg.Role == provider.RoleSystem {
			if systemPrompt != "" {
				systemPrompt += "\n"
			}
			systemPrompt += msg.Content
			continue
		}

		antMsg := antMessage{
			Role:    string(msg.Role),
			Content: []antContent{{Type: "text", Text: msg.Content}},
		}

		// Handle tool result messages
		if msg.Role == provider.RoleTool && msg.ToolCallID != "" {
			antMsg.Role = "user"
			antMsg.Content = []antContent{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}}
		}

		// Handle assistant messages with tool calls
		if msg.Role == provider.RoleAssistant && len(msg.ToolCalls) > 0 {
			contents := []antContent{}
			if msg.Content != "" {
				contents = append(contents, antContent{Type: "text", Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var input any
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
				contents = append(contents, antContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			antMsg.Content = contents
		}

		antMsgs = append(antMsgs, antMsg)
	}

	antReq := antRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      systemPrompt,
		Messages:    antMsgs,
		Temperature: req.Temperature,
	}

	for _, t := range req.Tools {
		antReq.Tools = append(antReq.Tools, antTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	body, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.endpoint + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", apiVersion)
	if p.apiKey != "" {
		httpReq.Header.Set("x-api-key", p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var antResp antResponse
	if err := json.Unmarshal(respBody, &antResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if antResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", antResp.Error.Message)
	}

	result := &provider.CompletionResponse{
		PromptTokens:     antResp.Usage.InputTokens,
		CompletionTokens: antResp.Usage.OutputTokens,
	}

	for _, c := range antResp.Content {
		switch c.Type {
		case "text":
			result.Content += c.Text
		case "tool_use":
			argsBytes, _ := json.Marshal(c.Input)
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: string(argsBytes),
			})
		}
	}

	return result, nil
}

// StreamComplete falls back to non-streaming for Anthropic.
func (p *AnthropicProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if onDelta != nil && resp.Content != "" {
		onDelta(resp.Content)
	}
	return resp, nil
}
