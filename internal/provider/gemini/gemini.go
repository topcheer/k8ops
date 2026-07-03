// Package gemini implements a Google Gemini-compatible chat provider.
// Any vendor with a Gemini-compatible API works with this provider.
package gemini

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
	defaultEndpoint = "https://generativelanguage.googleapis.com"
	defaultModel    = "gemini-1.5-pro"
	defaultTimeout  = 120 * time.Second
)

func init() {
	provider.Register("gemini", New)
}

// GeminiProvider implements provider.Provider for Gemini-compatible APIs.
type GeminiProvider struct {
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
	return &GeminiProvider{
		apiKey:   cfg.APIKey,
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: defaultTimeout},
	}, nil
}

func (p *GeminiProvider) Name() string { return "gemini" }

// Gemini API types
type gemPart struct {
	Text string `json:"text,omitempty"`
	// Function call
	FunctionCall *struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"functionCall,omitempty"`
	// Function response
	FunctionResponse *struct {
		Name     string         `json:"name"`
		Response map[string]any `json:"response"`
	} `json:"functionResponse,omitempty"`
}

type gemContent struct {
	Role  string    `json:"role"`
	Parts []gemPart `json:"parts"`
}

type gemSchema struct {
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Required    []string       `json:"required,omitempty"`
	Items       map[string]any `json:"items,omitempty"`
}

type gemFunctionDecl struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Parameters  gemSchema `json:"parameters"`
}

type gemTool struct {
	FunctionDeclarations []gemFunctionDecl `json:"functionDeclarations"`
}

type gemRequest struct {
	Contents          []gemContent `json:"contents"`
	SystemInstruction *gemContent  `json:"systemInstruction,omitempty"`
	Tools             []gemTool    `json:"tools,omitempty"`
	GenerationConfig  struct {
		MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
		Temperature     float64 `json:"temperature,omitempty"`
	} `json:"generationConfig"`
}

type gemCandidate struct {
	Content      gemContent `json:"content"`
	FinishReason string     `json:"finishReason,omitempty"`
}

type gemResponse struct {
	Candidates    []gemCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	gemReq := gemRequest{}
	gemReq.GenerationConfig.MaxOutputTokens = req.MaxTokens
	gemReq.GenerationConfig.Temperature = req.Temperature

	for _, msg := range req.Messages {
		if msg.Role == provider.RoleSystem {
			gemReq.SystemInstruction = &gemContent{
				Parts: []gemPart{{Text: msg.Content}},
			}
			continue
		}

		role := "user"
		if msg.Role == provider.RoleAssistant {
			role = "model"
		}

		content := gemContent{Role: role}

		if msg.Role == provider.RoleTool && msg.ToolCallID != "" {
			// Function response
			var respData map[string]any
			_ = json.Unmarshal([]byte(msg.Content), &respData)
			if respData == nil {
				respData = map[string]any{"result": msg.Content}
			}
			content.Parts = []gemPart{{
				FunctionResponse: &struct {
					Name     string         `json:"name"`
					Response map[string]any `json:"response"`
				}{Name: msg.ToolCallID, Response: respData},
			}}
		} else if len(msg.ToolCalls) > 0 {
			// Assistant with function calls
			if msg.Content != "" {
				content.Parts = append(content.Parts, gemPart{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				content.Parts = append(content.Parts, gemPart{
					FunctionCall: &struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					}{Name: tc.Name, Args: args},
				})
			}
		} else {
			content.Parts = []gemPart{{Text: msg.Content}}
		}

		gemReq.Contents = append(gemReq.Contents, content)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		tool := gemTool{}
		for _, t := range req.Tools {
			fd := gemFunctionDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
			}
			if t.Function.Parameters != nil {
				if t, ok := t.Function.Parameters["type"].(string); ok {
					fd.Parameters.Type = t
				}
				if d, ok := t.Function.Parameters["description"].(string); ok {
					fd.Parameters.Description = d
				}
				if p, ok := t.Function.Parameters["properties"].(map[string]any); ok {
					fd.Parameters.Properties = p
				}
				if r, ok := t.Function.Parameters["required"].([]any); ok {
					for _, v := range r {
						if s, ok := v.(string); ok {
							fd.Parameters.Required = append(fd.Parameters.Required, s)
						}
					}
				}
				if i, ok := t.Function.Parameters["items"].(map[string]any); ok {
					fd.Parameters.Items = i
				}
			}
			tool.FunctionDeclarations = append(tool.FunctionDeclarations, fd)
		}
		gemReq.Tools = []gemTool{tool}
	}

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.endpoint, model, p.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gemResp gemResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if gemResp.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", gemResp.Error.Message)
	}

	result := &provider.CompletionResponse{
		PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
		CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
	}

	for _, cand := range gemResp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				result.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsBytes, _ := json.Marshal(part.FunctionCall.Args)
				result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
					ID:        part.FunctionCall.Name, // Gemini doesn't use IDs, use name
					Name:      part.FunctionCall.Name,
					Arguments: string(argsBytes),
				})
			}
		}
	}

	return result, nil
}

// StreamComplete falls back to non-streaming for Gemini.
func (p *GeminiProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if onDelta != nil && resp.Content != "" {
		onDelta(resp.Content)
	}
	return resp, nil
}
