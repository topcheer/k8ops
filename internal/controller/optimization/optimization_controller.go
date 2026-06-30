package optimization

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
	"github.com/ggai/k8ops/internal/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OptimizationReconciler reconciles an OptimizationSuggestion object.
type OptimizationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Log         *slog.Logger
	ProviderCfg provider.ProviderConfig
}

func (r *OptimizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.With("optimizationsuggestion", req.NamespacedName)

	opt := &aiv1alpha1.OptimizationSuggestion{}
	if err := r.Get(ctx, req.NamespacedName, opt); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if opt.Status.Phase != "" && opt.Status.Phase != "Pending" {
		return ctrl.Result{}, nil
	}

	opt.Status.Phase = "Analyzing"
	if err := r.Status().Update(ctx, opt); err != nil {
		return ctrl.Result{Requeue: true}, nil
	}

	// Build user message
	userMsg := fmt.Sprintf("## Optimization Analysis Request\n\n")
	userMsg += fmt.Sprintf("**Scope:** %s", opt.Spec.Scope.Type)
	if opt.Spec.Scope.Namespace != "" {
		userMsg += fmt.Sprintf(" (namespace: %s)", opt.Spec.Scope.Namespace)
	}
	if opt.Spec.Scope.Workload != "" {
		userMsg += fmt.Sprintf(" (workload: %s)", opt.Spec.Scope.Workload)
	}
	userMsg += "\n"
	if len(opt.Spec.Categories) > 0 {
		userMsg += fmt.Sprintf("**Categories:** %v\n", opt.Spec.Categories)
	}
	userMsg += fmt.Sprintf("**Analysis window:** %s\n", opt.Spec.Window)
	userMsg += "\nPlease analyze the cluster resources and provide optimization recommendations."

	// Build tool registry (read-only tools for optimization)
	kubeClient, err := k8s.NewKubeClient()
	if err != nil {
		opt.Status.Phase = "Failed"
		opt.Status.Summary = fmt.Sprintf("Failed: %v", err)
		return ctrl.Result{}, r.Status().Update(ctx, opt)
	}

	registry := tools.NewRegistry()
	registry.Register(&k8s.GetResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListResourcesTool{Client: kubeClient})
	registry.Register(&k8s.DescribeResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListAPIResourcesTool{Client: kubeClient})
	registry.Register(&k8s.GetNodesTool{Client: kubeClient})

	// Resolve provider
	providerCfg, err := r.resolveProviderConfig(ctx)
	if err != nil {
		opt.Status.Phase = "Failed"
		opt.Status.Summary = fmt.Sprintf("Failed: %v", err)
		return ctrl.Result{}, r.Status().Update(ctx, opt)
	}

	prov, err := provider.New(providerCfg)
	if err != nil {
		opt.Status.Phase = "Failed"
		opt.Status.Summary = fmt.Sprintf("Failed: %v", err)
		return ctrl.Result{}, r.Status().Update(ctx, opt)
	}

	// Run agent
	agentInstance := agent.New(agent.AgentConfig{
		Provider:     prov,
		Registry:     registry,
		SystemPrompt: agent.OptimizationSystemPrompt(),
		MaxSteps:     10,
		Timeout:      120 * time.Second,
	}, log)

	result, err := agentInstance.Run(ctx, userMsg)
	if err != nil {
		opt.Status.Phase = "Failed"
		opt.Status.Summary = fmt.Sprintf("Failed: %v", err)
		return ctrl.Result{}, r.Status().Update(ctx, opt)
	}

	opt.Status.Phase = "Completed"
	opt.Status.AnalyzedAt = *nowPtr()

	// Parse structured response
	var structured struct {
		Summary               string                    `json:"summary"`
		Suggestions           []aiv1alpha1.Suggestion   `json:"suggestions"`
		TotalEstimatedSavings string                    `json:"totalEstimatedSavings"`
	}
	if err := json.Unmarshal([]byte(result.Answer), &structured); err == nil {
		opt.Status.Summary = structured.Summary
		opt.Status.Suggestions = structured.Suggestions
		opt.Status.TotalEstimatedSavings = structured.TotalEstimatedSavings
	} else {
		opt.Status.Summary = result.Answer
	}

	return ctrl.Result{}, r.Status().Update(ctx, opt)
}

func (r *OptimizationReconciler) resolveProviderConfig(ctx context.Context) (provider.ProviderConfig, error) {
	cfg := r.ProviderCfg
	configList := &aiv1alpha1.K8opsConfigList{}
	if err := r.List(ctx, configList); err == nil && len(configList.Items) > 0 {
		clusterCfg := configList.Items[0]
		if clusterCfg.Spec.Provider.Type != "" {
			cfg.Type = clusterCfg.Spec.Provider.Type
			cfg.Model = clusterCfg.Spec.Provider.Model
			cfg.Endpoint = clusterCfg.Spec.Provider.Endpoint
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

func nowPtr() *metav1.Time {
	t := metav1.Now()
	return &t
}

// SetupWithManager sets up the controller with the Manager.
func (r *OptimizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.OptimizationSuggestion{}).
		Complete(r)
}
