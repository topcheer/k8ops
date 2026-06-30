// Package openai implements an OpenAI-compatible chat completion provider.
// Any vendor with an OpenAI-compatible API (OpenAI, Azure OpenAI, DeepSeek,
// Moonshot, Together, OpenRouter, vLLM, Ollama, etc.) works with this provider.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ggai/k8ops/internal/provider"
)

const (
	defaultEndpoint = "https://api.openai.com/v1"
	defaultModel    = "gpt-4o"
	defaultTimeout  = 120 * time.Second
)

func init() {
	provider.Register("openai", New)
}

// OpenAIProvider implements provider.Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// New creates a new OpenAI-compatible provider.
func New(cfg provider.ProviderConfig) (provider.Provider, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	temp := cfg.Temperature
	if temp == 0 {
		temp = 0.1
	}
	return &OpenAIProvider{
		apiKey:   cfg.APIKey,
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: defaultTimeout},
	}, nil
}

func (p *OpenAIProvider) Name() string { return "openai" }

// request/response types matching OpenAI API spec
type oaiMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []oaiToolCall  `json:"tool_calls,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiTool struct {
	Type     string         `json:"type"`
	Function oaiToolSchema  `json:"function"`
}

type oaiToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
}

type oaiChoice struct {
	Index int `json:"index"`
	Message struct {
		Role      string        `json:"role"`
		Content   string        `json:"content"`
		ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *oaiError `json:"error,omitempty"`
}

type oaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	oaiReq := p.buildRequest(req)
	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
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
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var oaiResp oaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if oaiResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := oaiResp.Choices[0]
	result := &provider.CompletionResponse{
		Content:          choice.Message.Content,
		PromptTokens:     oaiResp.Usage.PromptTokens,
		CompletionTokens: oaiResp.Usage.CompletionTokens,
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}

// StreamComplete sends a streaming chat completion. onDelta is called with
// text chunks as they arrive. Returns the final assembled response.
func (p *OpenAIProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	oaiReq := p.buildRequest(req)

	// Add stream=true to request body
	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Inject "stream": true into the JSON
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	raw["stream"] = true
	raw["stream_options"] = map[string]any{"include_usage": true}
	body, err = json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	url := p.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var fullContent strings.Builder
	var toolCalls []provider.ToolCall
	var promptTokens, completionTokens int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				fullContent.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			// Accumulate tool calls by index
			for _, tc := range choice.Delta.ToolCalls {
				for len(toolCalls) <= tc.Index {
					toolCalls = append(toolCalls, provider.ToolCall{})
				}
				if tc.ID != "" {
					toolCalls[tc.Index].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCalls[tc.Index].Name = tc.Function.Name
				}
				toolCalls[tc.Index].Arguments += tc.Function.Arguments
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	return &provider.CompletionResponse{
		Content:          fullContent.String(),
		ToolCalls:        toolCalls,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}, nil
}

// buildRequest converts provider.CompletionRequest to oaiRequest.
func (p *OpenAIProvider) buildRequest(req provider.CompletionRequest) oaiRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	oaiReq := oaiRequest{
		Model:       model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    make([]oaiMessage, len(req.Messages)),
	}

	for i, msg := range req.Messages {
		oaiReq.Messages[i] = oaiMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		for _, tc := range msg.ToolCalls {
			var otc oaiToolCall
			otc.ID = tc.ID
			otc.Type = "function"
			otc.Function.Name = tc.Name
			otc.Function.Arguments = tc.Arguments
			oaiReq.Messages[i].ToolCalls = append(oaiReq.Messages[i].ToolCalls, otc)
		}
	}

	for _, t := range req.Tools {
		oaiReq.Tools = append(oaiReq.Tools, oaiTool{
			Type: t.Type,
			Function: oaiToolSchema{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	return oaiReq
}
