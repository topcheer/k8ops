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

// ProviderSpec defines the AI provider configuration.
type ProviderSpec struct {
	// Type is the provider standard: openai, anthropic, or gemini.
	// Any vendor compatible with these standards can be used.
	// +kubebuilder:validation:Enum=openai;anthropic;gemini
	Type string `json:"type"`

	// Model is the model name to use (e.g. gpt-4o, claude-3-5-sonnet, gemini-1.5-pro).
	Model string `json:"model"`

	// APIKeySecretRef references a Secret containing the API key.
	// +optional
	APIKeySecretRef *SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// Endpoint overrides the default API endpoint.
	// Use this to point to a compatible vendor (e.g. Azure OpenAI, DeepSeek, etc.).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// MaxTokens limits the response length.
	// +optional
	// +kubebuilder:default=4096
	MaxTokens int `json:"maxTokens,omitempty"`

	// Temperature controls randomness (0.0 - 2.0).
	// +optional
	// +kubebuilder:default=0.1
	Temperature float64 `json:"temperature,omitempty"`
}

// SecretKeySelector selects a key from a Secret.
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// AutoRemediationSpec configures auto-remediation behavior.
type AutoRemediationSpec struct {
	// Enabled controls whether auto-remediation is active.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// MaxRiskLevel is the maximum risk level that can be auto-applied.
	// low = only safe operations, medium = moderate changes, high = any operation.
	// +optional
	// +kubebuilder:validation:Enum=low;medium;high;critical
	// +kubebuilder:default=medium
	MaxRiskLevel string `json:"maxRiskLevel,omitempty"`

	// NamespaceAllowList restricts auto-remediation to specific namespaces.
	// Empty means all namespaces.
	// +optional
	NamespaceAllowList []string `json:"namespaceAllowList,omitempty"`

	// NamespaceDenyList prevents auto-remediation in specific namespaces.
	// +optional
	NamespaceDenyList []string `json:"namespaceDenyList,omitempty"`

	// DryRun runs remediation in simulation mode without actual changes.
	// +optional
	// +kubebuilder:default=false
	DryRun bool `json:"dryRun,omitempty"`

	// MaxConcurrentRemediations limits parallel remediation executions.
	// +optional
	// +kubebuilder:default=5
	MaxConcurrentRemediations int `json:"maxConcurrentRemediations,omitempty"`
}

// NotificationSpec configures alerting and notifications.
type NotificationSpec struct {
	// Slack webhook URL secret reference.
	// +optional
	Slack *SlackNotification `json:"slack,omitempty"`

	// Generic webhook URL for notifications.
	// +optional
	WebhookURL string `json:"webhookUrl,omitempty"`

	// Enable Prometheus alerting rules.
	// +optional
	// +kubebuilder:default=true
	PrometheusAlerting bool `json:"prometheusAlerting,omitempty"`
}

// SlackNotification configures Slack notifications.
type SlackNotification struct {
	WebhookSecretRef *SecretKeySelector `json:"webhookSecretRef,omitempty"`
	Channel          string             `json:"channel,omitempty"`
}

// AnalysisSpec configures the analysis behavior.
type AnalysisSpec struct {
	// DefaultDepth sets the default analysis depth.
	// +optional
	// +kubebuilder:validation:Enum=quick;standard;deep
	// +kubebuilder:default=standard
	DefaultDepth string `json:"defaultDepth,omitempty"`

	// MaxConcurrency limits parallel AI analyses.
	// +optional
	// +kubebuilder:default=3
	MaxConcurrency int `json:"maxConcurrency,omitempty"`

	// Timeout for a single analysis request.
	// +optional
	// +kubebuilder:default="120s"
	Timeout string `json:"timeout,omitempty"`

	// IncludeLogs includes pod logs in analysis.
	// +optional
	// +kubebuilder:default=true
	IncludeLogs bool `json:"includeLogs,omitempty"`

	// IncludeEvents includes Kubernetes events in analysis.
	// +optional
	// +kubebuilder:default=true
	IncludeEvents bool `json:"includeEvents,omitempty"`

	// IncludeMetrics includes Prometheus metrics in analysis (if available).
	// +optional
	// +kubebuilder:default=false
	IncludeMetrics bool `json:"includeMetrics,omitempty"`
}

// SafetySpec defines the safety guardrails.
type SafetySpec struct {
	// AllowedOperations whitelist of operation types the AI can perform.
	// +optional
	AllowedOperations []string `json:"allowedOperations,omitempty"`

	// DeniedOperations blacklist of operation types the AI cannot perform.
	// +optional
	DeniedOperations []string `json:"deniedOperations,omitempty"`

	// ProtectedResources are resources that should never be modified.
	// +optional
	ProtectedResources []ResourceRef `json:"protectedResources,omitempty"`

	// MaxActionsPerRemediation limits the number of actions in a single remediation.
	// +optional
	// +kubebuilder:default=10
	MaxActionsPerRemediation int `json:"maxActionsPerRemediation,omitempty"`
}

// ResourceRef references a Kubernetes resource.
type ResourceRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// K8opsConfigSpec defines the desired state of K8opsConfig.
type K8opsConfigSpec struct {
	// Provider configures the AI LLM backend.
	Provider ProviderSpec `json:"provider"`

	// Analysis configures diagnostic analysis behavior.
	// +optional
	Analysis AnalysisSpec `json:"analysis,omitempty"`

	// AutoRemediation configures auto-remediation behavior.
	// +optional
	AutoRemediation AutoRemediationSpec `json:"autoRemediation,omitempty"`

	// Notifications configures alerting channels.
	// +optional
	Notifications NotificationSpec `json:"notifications,omitempty"`

	// Safety defines guardrails for AI actions.
	// +optional
	Safety SafetySpec `json:"safety,omitempty"`
}

// K8opsConfigStatus reflects the observed state of K8opsConfig.
type K8opsConfigStatus struct {
	// Ready indicates the operator is operational.
	// +optional
	Ready bool `json:"ready"`

	// ActiveProvider shows the currently active provider.
	// +optional
	ActiveProvider string `json:"activeProvider,omitempty"`

	// LastSync is the last time the config was synced.
	// +optional
	LastSync metav1.Time `json:"lastSync,omitempty"`

	// DiagnosticsCount is total diagnostics performed.
	// +optional
	DiagnosticsCount int64 `json:"diagnosticsCount,omitempty"`

	// RemediationsCount is total remediations executed.
	// +optional
	RemediationsCount int64 `json:"remediationsCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
//
// K8opsConfig is the Schema for the k8opsconfigs API.
// It defines the global configuration for the k8ops AI operator.
type K8opsConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K8opsConfigSpec   `json:"spec,omitempty"`
	Status K8opsConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
//
// K8opsConfigList contains a list of K8opsConfig.
type K8opsConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K8opsConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K8opsConfig{}, &K8opsConfigList{})
}
