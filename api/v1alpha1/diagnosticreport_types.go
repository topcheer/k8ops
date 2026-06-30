/*
Copyright 2024 ggai.dev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TriggerType describes how the diagnostic was initiated.
// +kubebuilder:validation:Enum=event;alert;manual;schedule;watch
type TriggerType string

const (
	// TriggerEvent from Kubernetes event watching.
	TriggerEvent TriggerType = "event"
	// TriggerAlert from Prometheus/Loki alert.
	TriggerAlert TriggerType = "alert"
	// TriggerManual user-initiated.
	TriggerManual TriggerType = "manual"
	// TriggerSchedule from periodic schedule.
	TriggerSchedule TriggerType = "schedule"
	// TriggerWatch from resource watch.
	TriggerWatch TriggerType = "watch"
)

// DiagnosticTrigger describes what triggered this diagnostic.
type DiagnosticTrigger struct {
	// Type of trigger.
	Type TriggerType `json:"type"`

	// ResourceRef is the resource that triggered the diagnostic.
	// +optional
	ResourceRef *ResourceRef `json:"resourceRef,omitempty"`

	// EventMessage is the original event/alert message.
	// +optional
	EventMessage string `json:"eventMessage,omitempty"`

	// Reason from the triggering event.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// Finding represents a single diagnostic finding.
type Finding struct {
	// Severity: critical, high, medium, low, info.
	// +kubebuilder:validation:Enum=critical;high;medium;low;info
	Severity string `json:"severity"`

	// Category classifies the finding (e.g. CrashLoopBackOff, OOMKilled, PVC Pending).
	Category string `json:"category"`

	// Description of the finding.
	Description string `json:"description"`

	// RootCause analysis result.
	// +optional
	RootCause string `json:"rootCause,omitempty"`

	// Evidence supporting the finding.
	// +optional
	Evidence []Evidence `json:"evidence,omitempty"`

	// SuggestedActions for remediation.
	// +optional
	SuggestedActions []SuggestedAction `json:"suggestedActions,omitempty"`
}

// Evidence is data backing a finding.
type Evidence struct {
	// Type: log, event, metric, yaml, command-output.
	Type string `json:"type"`
	// Source where the evidence was collected from.
	Source string `json:"source,omitempty"`
	// Message the evidence content.
	Message string `json:"message"`
	// Timestamp of the evidence.
	// +optional
	Timestamp metav1.Time `json:"timestamp,omitempty"`
}

// SuggestedAction is an AI-generated remediation suggestion.
type SuggestedAction struct {
	// Type of action: patchResource, scaleResource, restartPod, deleteResource, runCommand, etc.
	Type string `json:"type"`

	// Description of the action.
	Description string `json:"description"`

	// Risk level: low, medium, high, critical.
	// +kubebuilder:validation:Enum=low;medium;high;critical
	Risk string `json:"risk"`

	// Target resource.
	// +optional
	Target *ResourceRef `json:"target,omitempty"`

	// Patch is a JSON patch for patchResource actions.
	// +optional
	Patch string `json:"patch,omitempty"`

	// Command for runCommand actions.
	// +optional
	Command string `json:"command,omitempty"`
}

// DiagnosticReportSpec defines the desired state of DiagnosticReport.
type DiagnosticReportSpec struct {
	// Trigger describes what initiated this diagnostic.
	Trigger DiagnosticTrigger `json:"trigger"`

	// Analysis configuration.
	// +optional
	Analysis *AnalysisSpec `json:"analysis,omitempty"`

	// ResourceScope narrows the diagnostic scope.
	// +optional
	ResourceScope *ResourceRef `json:"resourceScope,omitempty"`
}

// DiagnosticReportStatus defines the observed state of DiagnosticReport.
type DiagnosticReportStatus struct {
	// Phase: Pending, Analyzing, Completed, Failed.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Analyzing;Completed;Failed
	Phase string `json:"phase,omitempty"`

	// Findings from the analysis.
	// +optional
	Findings []Finding `json:"findings,omitempty"`

	// Summary is a human-readable summary.
	// +optional
	Summary string `json:"summary,omitempty"`

	// Confidence score (0.0 - 1.0).
	// +optional
	Confidence float64 `json:"confidence,omitempty"`

	// AIModel used for analysis.
	// +optional
	AIModel string `json:"aiModel,omitempty"`

	// AnalyzedAt timestamp.
	// +optional
	AnalyzedAt metav1.Time `json:"analyzedAt,omitempty"`

	// Error message if analysis failed.
	// +optional
	Error string `json:"error,omitempty"`

	// AgentTrace records the AI agent's tool calls during analysis.
	// +optional
	AgentTrace []AgentStep `json:"agentTrace,omitempty"`
}

// AgentStep records a single step in the agent's reasoning chain.
type AgentStep struct {
	// Step number.
	Step int `json:"step"`
	// Thought is the AI's reasoning for this step.
	Thought string `json:"thought,omitempty"`
	// Action tool name.
	Action string `json:"action,omitempty"`
	// ActionInput parameters passed to the tool.
	ActionInput string `json:"actionInput,omitempty"`
	// Observation result from the tool.
	Observation string `json:"observation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//
// DiagnosticReport is the Schema for the diagnosticreports API.
type DiagnosticReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiagnosticReportSpec   `json:"spec,omitempty"`
	Status DiagnosticReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
//
// DiagnosticReportList contains a list of DiagnosticReport.
type DiagnosticReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DiagnosticReport{}, &DiagnosticReportList{})
}
