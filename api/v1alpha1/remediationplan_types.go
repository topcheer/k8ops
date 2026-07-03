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

// RemediationAction defines a single remediation action.
type RemediationAction struct {
	// Type of action.
	// +kubebuilder:validation:Enum=patchResource;scaleResource;restartPod;deleteResource;runCommand;cordonNode;uncordonNode;drainNode;cleanImages;cleanLogs;adjustResource;createResource;applyManifest
	Type string `json:"type"`

	// Description of the action.
	Description string `json:"description"`

	// Risk level.
	// +kubebuilder:validation:Enum=low;medium;high;critical
	Risk string `json:"risk"`

	// Target resource for resource actions.
	// +optional
	Target *ResourceRef `json:"target,omitempty"`

	// Patch is JSON merge patch for patchResource/adjustResource.
	// +optional
	Patch string `json:"patch,omitempty"`

	// Replicas for scaleResource.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Manifest YAML for createResource/applyManifest.
	// +optional
	Manifest string `json:"manifest,omitempty"`

	// Command for runCommand actions (runs on the node).
	// +optional
	Command string `json:"command,omitempty"`

	// Timeout for the action.
	// +optional
	Timeout string `json:"timeout,omitempty"`
}

// RemediationPlanSpec defines the desired state of RemediationPlan.
type RemediationPlanSpec struct {
	// DiagnosticRef references the DiagnosticReport that triggered this remediation.
	// +optional
	DiagnosticRef string `json:"diagnosticRef,omitempty"`

	// Actions to execute.
	Actions []RemediationAction `json:"actions"`

	// Mode: auto, manual, dry-run.
	// +optional
	// +kubebuilder:validation:Enum=auto;manual;dry-run
	// +kubebuilder:default=auto
	Mode string `json:"mode,omitempty"`

	// RollbackOnFailure rolls back all actions if any fails.
	// +optional
	// +kubebuilder:default=true
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
}

// ActionResult records the outcome of an action.
type ActionResult struct {
	// Index of the action in the plan.
	Index int `json:"index"`

	// Status: Pending, Executing, Success, Failed, Skipped, RolledBack.
	// +kubebuilder:validation:Enum=Pending;Executing;Success;Failed;Skipped;RolledBack
	Status string `json:"status"`

	// Message with details.
	// +optional
	Message string `json:"message,omitempty"`

	// ExecutedAt timestamp.
	// +optional
	ExecutedAt metav1.Time `json:"executedAt,omitempty"`

	// RollbackData stores pre-action state for rollback.
	// +optional
	RollbackData string `json:"rollbackData,omitempty"`
}

// RemediationPlan phase constants.
const (
	RemediationPlanPhasePending    = "Pending"
	RemediationPlanPhaseApproved   = "Approved"
	RemediationPlanPhaseExecuting  = "Executing"
	RemediationPlanPhaseCompleted  = "Completed"
	RemediationPlanPhaseFailed     = "Failed"
	RemediationPlanPhaseRolledBack = "RolledBack"
)

// RemediationPlanStatus defines the observed state of RemediationPlan.
type RemediationPlanStatus struct {
	// Phase: Pending, Approved, Executing, Completed, Failed, Rollback.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Approved;Executing;Completed;Failed;RolledBack
	Phase string `json:"phase,omitempty"`

	// Results for each action.
	// +optional
	Results []ActionResult `json:"results,omitempty"`

	// StartedAt timestamp.
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt timestamp.
	// +optional
	CompletedAt metav1.Time `json:"completedAt,omitempty"`

	// Summary message.
	// +optional
	Summary string `json:"summary,omitempty"`

	// ApprovedBy records who approved this remediation plan.
	// +optional
	ApprovedBy string `json:"approvedBy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//
// RemediationPlan is the Schema for the remediationplans API.
type RemediationPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationPlanSpec   `json:"spec,omitempty"`
	Status RemediationPlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
//
// RemediationPlanList contains a list of RemediationPlan.
type RemediationPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationPlan{}, &RemediationPlanList{})
}
