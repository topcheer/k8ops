package agent

import (
	"strings"
	"testing"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
)

func TestDiagnosticSystemPrompt(t *testing.T) {
	prompt := DiagnosticSystemPrompt()
	if !strings.Contains(prompt, "k8ops") {
		t.Error("prompt should contain 'k8ops'")
	}
	if !strings.Contains(prompt, "diagnose") {
		t.Error("prompt should contain 'diagnose'")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Error("prompt should mention JSON format")
	}
}

func TestOptimizationSystemPrompt(t *testing.T) {
	prompt := OptimizationSystemPrompt()
	if !strings.Contains(prompt, "optimization") {
		t.Error("prompt should contain 'optimization'")
	}
	if !strings.Contains(prompt, "suggestions") {
		t.Error("prompt should mention 'suggestions'")
	}
}

// Ensure AgentStep type is compatible with API types.
func TestStepConversion(t *testing.T) {
	step := Step{
		Thought:     "I should check the pod",
		Action:      "k8s_get_resource",
		ActionInput: `{"name":"test"}`,
		Observation: "Pod not found",
	}

	// Simulate the conversion done in the controller
	apiStep := aiv1alpha1.AgentStep{
		Step:        1,
		Thought:     step.Thought,
		Action:      step.Action,
		ActionInput: step.ActionInput,
		Observation: step.Observation,
	}

	if apiStep.Action != "k8s_get_resource" {
		t.Errorf("expected action 'k8s_get_resource', got '%s'", apiStep.Action)
	}
	if apiStep.Step != 1 {
		t.Errorf("expected step 1, got %d", apiStep.Step)
	}
}

func TestNew(t *testing.T) {
	agent := New(AgentConfig{}, nil)
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	// Check defaults
	if agent.cfg.MaxSteps != 15 {
		t.Errorf("expected default MaxSteps=15, got %d", agent.cfg.MaxSteps)
	}
}
