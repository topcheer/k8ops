/*
Copyright 2024 ggai.dev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

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

// OptimizationType describes the kind of optimization.
// +kubebuilder:validation:Enum=resource-rightsize;hpa-recommendation;pdb-recommendation;cost-reduction;performance;reliability;security
type OptimizationType string

// Suggestion is a single optimization recommendation.
type Suggestion struct {
	// Type of optimization.
	Type OptimizationType `json:"type"`

	// Target workload.
	// +optional
	Target *ResourceRef `json:"target,omitempty"`

	// Description of the suggestion.
	Description string `json:"description"`

	// Current state (e.g. resource requests/limits).
	// +optional
	Current map[string]string `json:"current,omitempty"`

	// Recommended state.
	// +optional
	Recommended map[string]string `json:"recommended,omitempty"`

	// EstimatedSavings description (e.g. "60% cost reduction").
	// +optional
	EstimatedSavings string `json:"estimatedSavings,omitempty"`

	// Confidence score (0.0 - 1.0).
	// +optional
	Confidence float64 `json:"confidence,omitempty"`

	// Priority: critical, high, medium, low.
	// +kubebuilder:validation:Enum=critical;high;medium;low
	// +kubebuilder:default=medium
	Priority string `json:"priority"`

	// Risk of applying this suggestion.
	// +kubebuilder:validation:Enum=low;medium;high;critical
	// +kubebuilder:default=low
	Risk string `json:"risk"`
}

// OptimizationScope defines what the analysis covers.
type OptimizationScope struct {
	// Type: cluster, namespace, workload.
	// +kubebuilder:validation:Enum=cluster;namespace;workload
	Type string `json:"type"`

	// Namespace for namespace/workload scope.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Workload for workload scope.
	// +optional
	Workload string `json:"workload,omitempty"`
}

// OptimizationSuggestionSpec defines the desired state of OptimizationSuggestion.
type OptimizationSuggestionSpec struct {
	// Scope of the optimization analysis.
	Scope OptimizationScope `json:"scope"`

	// Categories to analyze.
	// +optional
	Categories []string `json:"categories,omitempty"`

	// Window is the analysis time range.
	// +optional
	// +kubebuilder:default="7d"
	Window string `json:"window,omitempty"`
}

// OptimizationSuggestionStatus defines the observed state.
type OptimizationSuggestionStatus struct {
	// Phase: Pending, Analyzing, Completed, Failed.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Analyzing;Completed;Failed
	Phase string `json:"phase,omitempty"`

	// Suggestions from the analysis.
	// +optional
	Suggestions []Suggestion `json:"suggestions,omitempty"`

	// Summary message.
	// +optional
	Summary string `json:"summary,omitempty"`

	// TotalEstimatedSavings aggregated savings estimate.
	// +optional
	TotalEstimatedSavings string `json:"totalEstimatedSavings,omitempty"`

	// AnalyzedAt timestamp.
	// +optional
	AnalyzedAt metav1.Time `json:"analyzedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//
// OptimizationSuggestion is the Schema for the optimizationsuggestions API.
type OptimizationSuggestion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OptimizationSuggestionSpec   `json:"spec,omitempty"`
	Status OptimizationSuggestionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
//
// OptimizationSuggestionList contains a list of OptimizationSuggestion.
type OptimizationSuggestionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OptimizationSuggestion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OptimizationSuggestion{}, &OptimizationSuggestionList{})
}
