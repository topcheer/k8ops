// Package agent implements the core AI agent engine.
// The agent follows an Observe→Think→Act loop:
// 1. Observe: collect context from the cluster (events, logs, resource state)
// 2. Think:   send the context to the LLM with tool definitions
// 3. Act:     execute the tool calls returned by the LLM
// 4. Loop:    feed results back and continue until the LLM produces a final answer
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
)

// AgentConfig configures the agent.
type AgentConfig struct {
	Provider     provider.Provider
	Registry     *tools.Registry
	SystemPrompt string
	MaxSteps     int
	Timeout      time.Duration
}

// Agent is the AI agent that runs the Observe→Think→Act loop.
type Agent struct {
	cfg AgentConfig
	log *slog.Logger
}

// New creates a new Agent.
func New(cfg AgentConfig, log *slog.Logger) *Agent {
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = 15
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	return &Agent{cfg: cfg, log: log}
}

// Step represents a single step in the agent's execution trace.
type Step struct {
	Thought     string `json:"thought,omitempty"`
	Action      string `json:"action,omitempty"`
	ActionInput string `json:"actionInput,omitempty"`
	Observation string `json:"observation,omitempty"`
}

// Result is the final result of an agent run.
type Result struct {
	Answer     string `json:"answer"`
	Steps      []Step `json:"steps"`
	TokenUsage struct {
		Prompt     int `json:"prompt"`
		Completion int `json:"completion"`
		Total      int `json:"total"`
	} `json:"tokenUsage"`
}

// Run executes the agent loop with the given user message.
func (a *Agent) Run(ctx context.Context, userMessage string) (*Result, error) {
	if a.cfg.Provider == nil {
		return nil, fmt.Errorf("agent provider is not configured")
	}
	if a.cfg.Registry == nil {
		a.cfg.Registry = tools.NewRegistry()
	}

	ctx, cancel := context.WithTimeout(ctx, a.cfg.Timeout)
	defer cancel()

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: a.cfg.SystemPrompt},
		{Role: provider.RoleUser, Content: userMessage},
	}

	toolDefs := a.cfg.Registry.Definitions()
	result := &Result{}

	for step := 0; step < a.cfg.MaxSteps; step++ {
		a.log.Debug("agent step", "step", step+1)

		req := provider.CompletionRequest{
			Messages:    messages,
			Tools:       a.toProviderTools(toolDefs),
			Temperature: 0.1,
			MaxTokens:   4096,
		}

		resp, err := a.cfg.Provider.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("LLM completion failed at step %d: %w", step+1, err)
		}

		result.TokenUsage.Prompt += resp.PromptTokens
		result.TokenUsage.Completion += resp.CompletionTokens

		// If no tool calls, the agent is done
		if len(resp.ToolCalls) == 0 {
			result.Answer = resp.Content
			result.TokenUsage.Total = result.TokenUsage.Prompt + result.TokenUsage.Completion
			return result, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			stepRecord := Step{
				Thought:     resp.Content,
				Action:      tc.Name,
				ActionInput: tc.Arguments,
			}

			a.log.Info("tool call", "tool", tc.Name, "args", tc.Arguments)

			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				stepRecord.Observation = fmt.Sprintf("Error: failed to parse arguments: %v", err)
				result.Steps = append(result.Steps, stepRecord)
				messages = append(messages, provider.Message{
					Role:       provider.RoleTool,
					ToolCallID: tc.ID,
					Content:    stepRecord.Observation,
				})
				continue
			}

			tool, ok := a.cfg.Registry.Get(tc.Name)
			if !ok {
				stepRecord.Observation = fmt.Sprintf("Error: unknown tool '%s'", tc.Name)
				result.Steps = append(result.Steps, stepRecord)
				messages = append(messages, provider.Message{
					Role:       provider.RoleTool,
					ToolCallID: tc.ID,
					Content:    stepRecord.Observation,
				})
				continue
			}

			// Execute the tool
			toolResult, err := tool.Execute(ctx, args)
			if err != nil {
				stepRecord.Observation = fmt.Sprintf("Error executing tool: %v", err)
			} else if !toolResult.Success {
				stepRecord.Observation = fmt.Sprintf("Tool returned error: %s\n%s", toolResult.Error, toolResult.Output)
			} else {
				obs := toolResult.Output
				// Truncate very long observations
				if len(obs) > 8000 {
					obs = obs[:8000] + "\n... (truncated, use more specific queries for full data)"
				}
				stepRecord.Observation = obs
			}

			result.Steps = append(result.Steps, stepRecord)

			// Add tool result to messages
			toolMsgContent := stepRecord.Observation
			if toolResult != nil && !toolResult.Success && toolResult.Error != "" {
				toolMsgContent = fmt.Sprintf("Error: %s\n%s", toolResult.Error, toolResult.Output)
			}
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				ToolCallID: tc.ID,
				Content:    toolMsgContent,
			})
		}
	}

	result.Answer = "Agent reached maximum steps without a final conclusion."
	result.TokenUsage.Total = result.TokenUsage.Prompt + result.TokenUsage.Completion
	return result, nil
}

// toProviderTools converts internal tool defs to provider tool definitions.
func (a *Agent) toProviderTools(defs []tools.ToolDef) []provider.ToolDefinition {
	result := make([]provider.ToolDefinition, 0, len(defs))
	for _, d := range defs {
		result = append(result, provider.ToolDefinition{
			Type: d.Type,
			Function: provider.ToolFunctionSchema{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		})
	}
	return result
}

// SystemPrompts returns the system prompts for different analysis types.
func DiagnosticSystemPrompt() string {
	return `You are k8ops, an expert Kubernetes SRE and AIOps agent running inside a Kubernetes cluster.

Your job is to diagnose and fix Kubernetes cluster issues autonomously.

## Your Capabilities
You have access to tools that let you:
- Get, list, describe any Kubernetes resource (including third-party CRDs)
- Read pod logs, events, and resource details
- Check node status, host disk, network, processes, and systemd services
- Patch, scale, restart, and delete resources
- Apply new manifests
- Cordon/uncordon nodes
- Run arbitrary commands on the host node

## Methodology
1. START by understanding the problem. Read the trigger event carefully.
2. GATHER DATA: Use tools to inspect relevant resources, logs, events.
3. ANALYZE: Identify root cause from the evidence you've gathered.
4. FIX: Take corrective action using the appropriate tool.
5. VERIFY: Confirm the fix worked by re-checking the resource state.
6. REPORT: Summarize your findings and actions.

## Rules
- Always gather evidence before making changes.
- Prefer the least invasive fix (e.g., scale up before adding nodes).
- If you're unsure about a risky operation, describe what you would do and stop.
- Use the k8s_list_api_resources tool to discover available resource types when needed.
- For third-party CRDs, first discover them, then use the standard get/list tools.
- Be thorough: check events, logs, conditions, and related resources.
- Provide clear explanations of what you found and why you took each action.

## Response Format
When you have completed your analysis and any fixes, provide your final answer in this JSON format:
{
  "summary": "Brief description of the issue and resolution",
  "findings": [
    {
      "severity": "critical|high|medium|low|info",
      "category": "Category name",
      "description": "What was found",
      "rootCause": "Root cause analysis",
      "evidence": [{"type": "log|event|metric", "source": "where", "message": "what"}],
      "suggestedActions": [{"type": "action type", "description": "what to do", "risk": "low|medium|high"}]
    }
  ],
  "actionsTaken": ["list of actions you took"],
  "confidence": 0.0-1.0,
  "followUpRecommendations": ["optional recommendations"]
}

Do NOT wrap the JSON in markdown code blocks. Output raw JSON as your final answer.`
}

func OptimizationSystemPrompt() string {
	return `You are k8ops, an expert Kubernetes performance and cost optimization agent.

Your job is to analyze resource usage across the cluster and provide optimization recommendations.

## Methodology
1. GATHER: Inspect resource requests/limits, HPA/PDB configs, node utilization.
2. ANALYZE: Identify over-provisioned or under-provisioned resources.
3. RECOMMEND: Suggest right-sizing, HPA/PDB additions, and cost optimizations.

## Response Format
Provide your final answer as raw JSON in this format:
{
  "summary": "Brief overview of optimization opportunities",
  "suggestions": [
    {
      "type": "resource-rightsize|hpa-recommendation|pdb-recommendation|cost-reduction",
      "description": "What to optimize",
      "current": {"key": "value"},
      "recommended": {"key": "value"},
      "estimatedSavings": "e.g. 40% cost reduction",
      "confidence": 0.0-1.0,
      "priority": "critical|high|medium|low",
      "risk": "low|medium|high"
    }
  ],
  "totalEstimatedSavings": "Overall savings estimate"
}

Do NOT wrap the JSON in markdown code blocks. Output raw JSON as your final answer.`
}
