// Package metrics provides Prometheus instrumentation for k8ops.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DiagnosticsTotal counts diagnostic reports by phase.
	DiagnosticsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_diagnostics_total",
		Help: "Total number of diagnostic reports processed",
	}, []string{"phase", "trigger"})

	// RemediationTotal counts remediation actions by type and result.
	RemediationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_remediation_actions_total",
		Help: "Total remediation actions executed",
	}, []string{"type", "result", "risk"})

	// LLMCallDuration tracks LLM API call latency.
	LLMCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "k8ops_llm_call_duration_seconds",
		Help:    "LLM API call duration in seconds",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 120},
	}, []string{"provider", "model", "status"})

	// LLMTokenUsage tracks token consumption.
	LLMTokenUsage = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_llm_tokens_total",
		Help: "Total LLM tokens consumed",
	}, []string{"provider", "model", "type"}) // type: prompt or completion

	// AgentSteps tracks steps per agent run.
	AgentSteps = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "k8ops_agent_steps",
		Help:    "Number of steps per agent execution",
		Buckets: []float64{1, 3, 5, 8, 10, 12, 15},
	})

	// ToolCallDuration tracks tool execution time.
	ToolCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "k8ops_tool_call_duration_seconds",
		Help:    "Tool execution duration",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"tool", "success"})

	// SafetyBlocks counts actions blocked by safety checker.
	SafetyBlocks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_safety_blocks_total",
		Help: "Actions blocked by safety checker",
	}, []string{"reason"})

	// ActiveDiagnostics is a gauge of currently running diagnostics.
	ActiveDiagnostics = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8ops_active_diagnostics",
		Help: "Number of currently running diagnostic analyses",
	})

	// ActiveRemediations is a gauge of currently executing remediations.
	ActiveRemediations = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8ops_active_remediations",
		Help: "Number of currently executing remediation plans",
	})

	// AuditEvents counts audit events by type.
	AuditEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_audit_events_total",
		Help: "Total audit events recorded",
	}, []string{"type", "severity"})

	// ClusterHealth is a gauge representing cluster health score (0-100).
	ClusterHealth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8ops_cluster_health_score",
		Help: "Cluster health score (0=worst, 100=best)",
	})

	// ConversationCount tracks active conversations.
	ConversationCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8ops_conversation_count",
		Help: "Number of active chat conversations",
	})

	// ToolExecTotal counts tool executions by name.
	ToolExecTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8ops_tool_executions_total",
		Help: "Total tool executions",
	}, []string{"tool", "success"})
)
