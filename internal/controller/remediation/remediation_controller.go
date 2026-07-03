package remediation

import (
	"context"
	"fmt"
	"log/slog"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/safety"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	remediationtools "github.com/ggai/k8ops/internal/tools/remediation"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "aiops.ggai.dev/remediation-finalizer"

// RemediationReconciler reconciles a RemediationPlan object.
type RemediationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *rest.Config
	Log    *slog.Logger
}

// +kubebuilder:rbac:groups=aiops.ggai.dev,resources=remediationplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aiops.ggai.dev,resources=remediationplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

func (r *RemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.With("remediationplan", req.NamespacedName)

	plan := &aiv1alpha1.RemediationPlan{}
	if err := r.Get(ctx, req.NamespacedName, plan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !plan.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(plan, finalizerName)
		return ctrl.Result{}, r.Update(ctx, plan)
	}

	// Only process pending plans
	if plan.Status.Phase != "" && plan.Status.Phase != "Pending" {
		return ctrl.Result{}, nil
	}

	// Load config for safety checks
	configList := &aiv1alpha1.K8opsConfigList{}
	var safetyCfg *aiv1alpha1.SafetySpec
	var autoCfg *aiv1alpha1.AutoRemediationSpec
	if err := r.List(ctx, configList); err == nil && len(configList.Items) > 0 {
		safetyCfg = &configList.Items[0].Spec.Safety
		autoCfg = &configList.Items[0].Spec.AutoRemediation
	}

	checker := safety.NewChecker(safetyCfg, autoCfg)

	// Check mode
	mode := plan.Spec.Mode
	if mode == "" {
		mode = "auto"
	}
	if mode == "dry-run" {
		log.Info("dry-run mode, skipping execution")
		plan.Status.Phase = "Completed"
		plan.Status.Summary = "Dry-run: no actions executed"
		for i := range plan.Spec.Actions {
			plan.Status.Results = append(plan.Status.Results, aiv1alpha1.ActionResult{
				Index:   i,
				Status:  "Skipped",
				Message: "Dry-run mode",
			})
		}
		return ctrl.Result{}, r.Status().Update(ctx, plan)
	}

	// Start execution
	plan.Status.Phase = "Executing"
	plan.Status.StartedAt = *nowPtr()
	if err := r.Status().Update(ctx, plan); err != nil {
		return ctrl.Result{Requeue: true}, nil
	}

	// Build tool registry
	registry, err := r.buildToolRegistry()
	if err != nil {
		plan.Status.Phase = "Failed"
		plan.Status.Summary = fmt.Sprintf("Failed to build tools: %v", err)
		return ctrl.Result{}, r.Status().Update(ctx, plan)
	}

	// Execute actions
	allSuccess := true
	for i, action := range plan.Spec.Actions {
		actionLog := log.With("action", i, "type", action.Type)

		// Safety check
		checkResult := checker.CheckAction(action)
		if !checkResult.Allowed {
			actionLog.Warn("action blocked by safety check", "reason", checkResult.Reason)
			plan.Status.Results = append(plan.Status.Results, aiv1alpha1.ActionResult{
				Index:   i,
				Status:  "Skipped",
				Message: fmt.Sprintf("Blocked by safety: %s", checkResult.Reason),
			})
			continue
		}

		// Execute the action via the appropriate tool
		result := r.executeAction(ctx, registry, action)
		plan.Status.Results = append(plan.Status.Results, aiv1alpha1.ActionResult{
			Index:      i,
			Status:     statusFromResult(result),
			Message:    result.Output + result.Error,
			ExecutedAt: *nowPtr(),
		})

		if !result.Success {
			allSuccess = false
			if plan.Spec.RollbackOnFailure {
				actionLog.Info("action failed, rolling back")
				r.rollback(ctx, plan, i)
				plan.Status.Phase = "RolledBack"
				break
			}
		}
	}

	plan.Status.CompletedAt = *nowPtr()
	if plan.Status.Phase != "RolledBack" {
		if allSuccess {
			plan.Status.Phase = "Completed"
			plan.Status.Summary = fmt.Sprintf("All %d actions completed successfully", len(plan.Spec.Actions))
		} else {
			plan.Status.Phase = "Failed"
			plan.Status.Summary = "Some actions failed"
		}
	}

	return ctrl.Result{}, r.Status().Update(ctx, plan)
}

func statusFromResult(result *tools.ToolResult) string {
	if result.Success {
		return "Success"
	}
	return "Failed"
}

func (r *RemediationReconciler) executeAction(ctx context.Context, registry *tools.Registry, action aiv1alpha1.RemediationAction) *tools.ToolResult {
	toolName := actionTypeToTool(action.Type)
	tool, ok := registry.Get(toolName)
	if !ok {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("unknown tool: %s", toolName)}
	}

	args := map[string]any{}
	if action.Target != nil {
		args["apiVersion"] = action.Target.APIVersion
		args["kind"] = action.Target.Kind
		args["name"] = action.Target.Name
		args["namespace"] = action.Target.Namespace
	}
	if action.Patch != "" {
		args["patch"] = action.Patch
	}
	if action.Replicas != nil {
		args["replicas"] = *action.Replicas
	}
	if action.Manifest != "" {
		args["manifest"] = action.Manifest
	}
	if action.Command != "" {
		args["command"] = action.Command
	}
	if action.Target != nil && action.Target.Kind == "Node" || action.Type == "cordonNode" || action.Type == "uncordonNode" {
		if action.Target != nil {
			args["node"] = action.Target.Name
		}
		if action.Type == "uncordonNode" {
			args["uncordon"] = true
		}
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}
	}
	return result
}

func actionTypeToTool(actionType string) string {
	switch actionType {
	case "patchResource":
		return "k8s_patch_resource"
	case "scaleResource":
		return "k8s_scale_resource"
	case "restartPod":
		return "k8s_restart_pod"
	case "cordonNode", "uncordonNode":
		return "k8s_cordon_node"
	case "deleteResource":
		return "k8s_delete_evicted_pods"
	case "createResource", "applyManifest":
		return "k8s_apply_manifest"
	case "runCommand":
		return "host_exec"
	default:
		return actionType
	}
}

func (r *RemediationReconciler) rollback(ctx context.Context, plan *aiv1alpha1.RemediationPlan, upTo int) {
	// Best-effort rollback by marking actions as rolled back
	for i := range plan.Status.Results {
		if i < upTo && plan.Status.Results[i].Status == "Success" {
			plan.Status.Results[i].Status = "RolledBack"
		}
	}
}

func (r *RemediationReconciler) buildToolRegistry() (*tools.Registry, error) {
	remediator, err := remediationtools.NewRemediator(r.Config)
	if err != nil {
		return nil, err
	}

	registry := tools.NewRegistry()
	registry.Register(&remediationtools.PatchResourceTool{R: remediator})
	registry.Register(&remediationtools.ScaleResourceTool{R: remediator})
	registry.Register(&remediationtools.RestartPodTool{R: remediator})
	registry.Register(&remediationtools.CordonNodeTool{R: remediator})
	registry.Register(&remediationtools.DeleteEvictedPodsTool{R: remediator})
	registry.Register(&remediationtools.ApplyManifestTool{R: remediator})
	registry.Register(&host.HostExecTool{})
	return registry, nil
}

func nowPtr() *metav1.Time {
	t := metav1.Now()
	return &t
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemediationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.RemediationPlan{}).
		Complete(r)
}
