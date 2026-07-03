package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/agent"
	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
	"github.com/ggai/k8ops/internal/tools/remediation"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "aiops.ggai.dev/diagnostic-finalizer"

// DiagnosticReconciler reconciles a DiagnosticReport object.
type DiagnosticReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Config      *rest.Config
	Log         *slog.Logger
	ProviderCfg provider.ProviderConfig
}

// +kubebuilder:rbac:groups=aiops.ggai.dev,resources=diagnosticreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aiops.ggai.dev,resources=diagnosticreports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func (r *DiagnosticReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.With("diagnosticreport", req.NamespacedName)

	report := &aiv1alpha1.DiagnosticReport{}
	if err := r.Get(ctx, req.NamespacedName, report); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !report.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(report, finalizerName)
		return ctrl.Result{}, r.Update(ctx, report)
	}

	// Only process pending or analyzing reports
	if report.Status.Phase != "" && report.Status.Phase != "Pending" {
		return ctrl.Result{}, nil
	}

	// Update to Analyzing
	report.Status.Phase = "Analyzing"
	if err := r.Status().Update(ctx, report); err != nil {
		log.Error("failed to update status", "error", err)
		return ctrl.Result{Requeue: true}, nil
	}

	// Build user message for the agent
	userMsg := r.buildUserMessage(report)

	// Build tool registry
	registry, err := r.buildToolRegistry()
	if err != nil {
		return r.failReport(ctx, report, fmt.Sprintf("failed to build tools: %v", err))
	}

	// Resolve provider config
	providerCfg, err := r.resolveProviderConfig(ctx)
	if err != nil {
		return r.failReport(ctx, report, fmt.Sprintf("failed to resolve provider config: %v", err))
	}

	// Skip if provider not yet configured (e.g. on startup before ConfigMap is loaded)
	if providerCfg.APIKey == "" {
		log.Info("provider not yet configured, requeueing diagnostic", "report", report.Name)
		report.Status.Phase = "Pending"
		report.Status.Error = "provider not configured, waiting..."
		_ = r.Status().Update(ctx, report)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Create provider
	prov, err := provider.New(providerCfg)
	if err != nil {
		return r.failReport(ctx, report, fmt.Sprintf("failed to create provider: %v", err))
	}

	// Run agent
	agentInstance := agent.New(agent.AgentConfig{
		Provider:     prov,
		Registry:     registry,
		SystemPrompt: agent.DiagnosticSystemPrompt(),
		MaxSteps:     15,
		Timeout:      180 * time.Second,
	}, log)

	result, err := agentInstance.Run(ctx, userMsg)
	if err != nil {
		return r.failReport(ctx, report, fmt.Sprintf("agent error: %v", err))
	}

	// Parse the agent's JSON answer
	report.Status.Phase = "Completed"
	report.Status.Summary = extractField(result.Answer, "summary")
	report.Status.AIModel = providerCfg.Model
	report.Status.AnalyzedAt = *now()
	report.Status.Confidence = extractConfidence(result.Answer)

	// Convert agent steps to trace
	for i, s := range result.Steps {
		report.Status.AgentTrace = append(report.Status.AgentTrace, aiv1alpha1.AgentStep{
			Step:        i + 1,
			Thought:     s.Thought,
			Action:      s.Action,
			ActionInput: s.ActionInput,
			Observation: s.Observation,
		})
	}

	// Try to parse structured findings
	findings := extractFindings(result.Answer)
	if findings != nil {
		report.Status.Findings = findings
	} else {
		// Fallback: create a single finding from the answer
		report.Status.Findings = []aiv1alpha1.Finding{{
			Severity:    "medium",
			Category:    "General",
			Description: result.Answer,
		}}
	}

	if err := r.Status().Update(ctx, report); err != nil {
		log.Error("failed to update final status", "error", err)
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("diagnostic completed", "report", report.Name, "findings", len(report.Status.Findings), "steps", len(result.Steps))

	// If auto-remediation is enabled and findings suggest actions, create RemediationPlan
	r.maybeCreateRemediationPlan(ctx, report, findings)

	return ctrl.Result{}, nil
}

func (r *DiagnosticReconciler) buildUserMessage(report *aiv1alpha1.DiagnosticReport) string {
	msg := fmt.Sprintf("## Diagnostic Request\n\n")
	msg += fmt.Sprintf("**Trigger:** %s\n", report.Spec.Trigger.Type)

	if report.Spec.Trigger.Reason != "" {
		msg += fmt.Sprintf("**Reason:** %s\n", report.Spec.Trigger.Reason)
	}
	if report.Spec.Trigger.EventMessage != "" {
		msg += fmt.Sprintf("**Event Message:** %s\n", report.Spec.Trigger.EventMessage)
	}
	if report.Spec.Trigger.ResourceRef != nil {
		ref := report.Spec.Trigger.ResourceRef
		msg += fmt.Sprintf("**Resource:** %s/%s", ref.Kind, ref.Name)
		if ref.Namespace != "" {
			msg += fmt.Sprintf(" in namespace %s", ref.Namespace)
		}
		msg += "\n"
	}

	msg += "\nPlease investigate this issue, identify the root cause, and take corrective action if possible."
	return msg
}

func (r *DiagnosticReconciler) buildToolRegistry() (*tools.Registry, error) {
	kubeClient, err := k8s.NewKubeClient()
	if err != nil {
		return nil, err
	}

	remediator, err := remediation.NewRemediator(r.Config)
	if err != nil {
		return nil, err
	}

	registry := tools.NewRegistry()
	// K8s read tools
	registry.Register(&k8s.GetResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListResourcesTool{Client: kubeClient})
	registry.Register(&k8s.DescribeResourceTool{Client: kubeClient})
	registry.Register(&k8s.GetPodLogsTool{Client: kubeClient})
	registry.Register(&k8s.ListAPIResourcesTool{Client: kubeClient})
	registry.Register(&k8s.GetNodesTool{Client: kubeClient})
	registry.Register(&k8s.GetEventsTool{Client: kubeClient})
	registry.Register(&k8s.GetNamespacesTool{Client: kubeClient})
	registry.Register(&k8s.GetTopTool{Client: kubeClient})
	registry.Register(&k8s.GetHPATool{Client: kubeClient})
	registry.Register(&k8s.GetPDBTool{Client: kubeClient})
	registry.Register(&k8s.GetStorageTool{Client: kubeClient})
	registry.Register(&k8s.GetClusterVersionTool{Client: kubeClient})
	registry.Register(&k8s.GetServicesTool{Client: kubeClient})
	registry.Register(&k8s.GetConfigMapTool{Client: kubeClient})
	registry.Register(&k8s.GetIngressTool{Client: kubeClient})
	registry.Register(&k8s.GetNetworkPolicyTool{Client: kubeClient})
	registry.Register(&k8s.GetPodStatusTool{Client: kubeClient})
	registry.Register(&k8s.ExecInPodTool{Client: kubeClient})
	registry.Register(&k8s.DrainNodeTool{Client: kubeClient})

	// Host tools
	registry.Register(&host.HostExecTool{})
	registry.Register(&host.HostDiskUsageTool{})
	registry.Register(&host.HostNetworkTool{})
	registry.Register(&host.HostProcessTool{})
	registry.Register(&host.HostServiceTool{})
	registry.Register(&host.HostInfoTool{})
	registry.Register(&host.HostDmesgTool{})
	registry.Register(&host.HostContainerRuntimeTool{})
	registry.Register(&host.HostKubeletTool{})
	registry.Register(&host.HostIPTablesTool{})
	registry.Register(&host.HostMountsTool{})
	registry.Register(&host.HostDiskIOTool{})
	registry.Register(&host.HostMemoryInfoTool{})

	// Remediation tools (read-then-write order)
	registry.Register(&remediation.PatchResourceTool{R: remediator})
	registry.Register(&remediation.ScaleResourceTool{R: remediator})
	registry.Register(&remediation.RestartPodTool{R: remediator})
	registry.Register(&remediation.CordonNodeTool{R: remediator})
	registry.Register(&remediation.DeleteEvictedPodsTool{R: remediator})
	registry.Register(&remediation.ApplyManifestTool{R: remediator})

	return registry, nil
}

func (r *DiagnosticReconciler) resolveProviderConfig(ctx context.Context) (provider.ProviderConfig, error) {
	cfg := r.ProviderCfg

	// Priority 1: Load from ConfigMap + Secret (set by dashboard provider manager)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: "k8ops-provider-config", Namespace: "k8ops-system"}, cm); err == nil {
		if v, ok := cm.Data["type"]; ok {
			cfg.Type = v
		}
		if v, ok := cm.Data["model"]; ok {
			cfg.Model = v
		}
		if v, ok := cm.Data["endpoint"]; ok {
			cfg.Endpoint = v
		}
		// Load API key from Secret
		sec := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: "k8ops-provider-secret", Namespace: "k8ops-system"}, sec); err == nil {
			if key, ok := sec.Data["apiKey"]; ok {
				cfg.APIKey = string(key)
			}
		}
	}

	// Priority 2: Try K8opsConfig CRD (overrides ConfigMap if present)
	configList := &aiv1alpha1.K8opsConfigList{}
	if err := r.List(ctx, configList); err == nil && len(configList.Items) > 0 {
		clusterCfg := configList.Items[0]
		if clusterCfg.Spec.Provider.Type != "" {
			cfg.Type = clusterCfg.Spec.Provider.Type
			cfg.Model = clusterCfg.Spec.Provider.Model
			cfg.Endpoint = clusterCfg.Spec.Provider.Endpoint
			if clusterCfg.Spec.Provider.MaxTokens > 0 {
				cfg.MaxTokens = clusterCfg.Spec.Provider.MaxTokens
			}
			if clusterCfg.Spec.Provider.Temperature > 0 {
				cfg.Temperature = clusterCfg.Spec.Provider.Temperature
			}

			// Resolve API key from secret
			if clusterCfg.Spec.Provider.APIKeySecretRef != nil {
				secret := &corev1.Secret{}
				key := clusterCfg.Spec.Provider.APIKeySecretRef.Key
				if key == "" {
					key = "api-key"
				}
				if err := r.Get(ctx, client.ObjectKey{Name: clusterCfg.Spec.Provider.APIKeySecretRef.Name}, secret); err == nil {
					if val, ok := secret.Data[key]; ok {
						cfg.APIKey = string(val)
					}
				}
			}
		}
	}

	return cfg, nil
}

func (r *DiagnosticReconciler) failReport(ctx context.Context, report *aiv1alpha1.DiagnosticReport, errMsg string) (ctrl.Result, error) {
	r.Log.Error("diagnostic failed", "report", report.Name, "error", errMsg)
	report.Status.Phase = "Failed"
	report.Status.Error = errMsg
	_ = r.Status().Update(ctx, report)
	return ctrl.Result{}, nil
}

func (r *DiagnosticReconciler) maybeCreateRemediationPlan(ctx context.Context, report *aiv1alpha1.DiagnosticReport, findings []aiv1alpha1.Finding) {
	// Check if auto-remediation is enabled
	configList := &aiv1alpha1.K8opsConfigList{}
	if err := r.List(ctx, configList); err != nil || len(configList.Items) == 0 {
		return
	}
	clusterCfg := configList.Items[0]
	if !clusterCfg.Spec.AutoRemediation.Enabled {
		return
	}

	// Convert suggested actions to remediation actions
	var actions []aiv1alpha1.RemediationAction
	for _, f := range findings {
		for _, sa := range f.SuggestedActions {
			actions = append(actions, aiv1alpha1.RemediationAction{
				Type:        sa.Type,
				Description: sa.Description,
				Risk:        sa.Risk,
				Target:      sa.Target,
				Patch:       sa.Patch,
				Command:     sa.Command,
			})
		}
	}

	if len(actions) == 0 {
		return
	}

	plan := &aiv1alpha1.RemediationPlan{
		ObjectMeta: ctrl.ObjectMeta{
			GenerateName: fmt.Sprintf("auto-%s-", report.Name),
			Namespace:    report.Namespace,
			Labels: map[string]string{
				"aiops.ggai.dev/diagnostic": report.Name,
				"aiops.ggai.dev/auto":       "true",
			},
		},
		Spec: aiv1alpha1.RemediationPlanSpec{
			DiagnosticRef:     report.Name,
			Actions:           actions,
			Mode:              "auto",
			RollbackOnFailure: true,
		},
	}

	if err := r.Create(ctx, plan); err != nil {
		r.Log.Error("failed to create remediation plan", "error", err)
	} else {
		r.Log.Info("created auto-remediation plan", "plan", plan.Name, "actions", len(actions))
	}
}

// Helper: extract a field from JSON string.
func extractField(jsonStr, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return jsonStr // not JSON, return as-is
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

func extractConfidence(jsonStr string) float64 {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return 0.5
	}
	if v, ok := m["confidence"].(float64); ok {
		return v
	}
	return 0.5
}

func extractFindings(jsonStr string) []aiv1alpha1.Finding {
	var m struct {
		Findings []aiv1alpha1.Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil
	}
	return m.Findings
}

func now() *metav1.Time {
	t := metav1.Now()
	return &t
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiagnosticReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.DiagnosticReport{}).
		Complete(r)
}
