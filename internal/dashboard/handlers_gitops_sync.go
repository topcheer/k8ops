package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// GitOpsSyncResult is the ArgoCD & Flux GitOps sync status auditor result.
type GitOpsSyncResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         GitOpsSummary    `json:"summary"`
	Applications    []GitOpsAppEntry `json:"applications"`
	ByNamespace     []GitOpsNSStat   `json:"byNamespace"`
	Issues          []GitOpsIssue    `json:"issues"`
	Recommendations []string         `json:"recommendations"`
	HealthScore     int              `json:"healthScore"`
}

// GitOpsSummary aggregates GitOps tool statistics.
type GitOpsSummary struct {
	ArgoCDDetected     bool `json:"argoCDDetected"`
	FluxDetected       bool `json:"fluxDetected"`
	TotalApps          int  `json:"totalApps"`
	HealthyApps        int  `json:"healthyApps"`
	OutOfSyncApps      int  `json:"outOfSyncApps"`
	SyncFailedApps     int  `json:"syncFailedApps"`
	StaleApps          int  `json:"staleApps"`
	NoAutoSyncApps     int  `json:"noAutoSyncApps"`
	DriftDetected      int  `json:"driftDetected"`
	ArgoCDApps         int  `json:"argoCDApps"`
	FluxSources        int  `json:"fluxSources"`
	FluxKustomizations int  `json:"fluxKustomizations"`
	FluxHelmReleases   int  `json:"fluxHelmReleases"`
}

// GitOpsAppEntry describes one GitOps application's sync status.
type GitOpsAppEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Tool           string `json:"tool"` // argocd or flux
	Kind           string `json:"kind"` // Application, Kustomization, HelmRelease
	SyncStatus     string `json:"syncStatus"`
	HealthStatus   string `json:"healthStatus"`
	RepoURL        string `json:"repoURL,omitempty"`
	TargetRevision string `json:"targetRevision,omitempty"`
	AutoSync       bool   `json:"autoSync"`
	LastSyncAt     string `json:"lastSyncAt,omitempty"`
	Stale          bool   `json:"stale"`
	Drift          bool   `json:"drift"`
	RiskLevel      string `json:"riskLevel"`
}

// GitOpsNSStat per-namespace GitOps stats.
type GitOpsNSStat struct {
	Namespace     string `json:"namespace"`
	TotalApps     int    `json:"totalApps"`
	HealthyApps   int    `json:"healthyApps"`
	OutOfSyncApps int    `json:"outOfSyncApps"`
	FailedApps    int    `json:"failedApps"`
}

// GitOpsIssue describes a specific GitOps problem.
type GitOpsIssue struct {
	Severity   string `json:"severity"`
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// handleGitOpsSync handles GET /api/deployment/gitops-sync
// Audits ArgoCD Application and Flux CRD sync status for GitOps drift detection.
func (s *Server) handleGitOpsSync(w http.ResponseWriter, r *http.Request) {
	result := s.auditGitOpsSync(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) auditGitOpsSync(ctx context.Context) *GitOpsSyncResult {
	result := &GitOpsSyncResult{ScannedAt: time.Now()}

	dynClient, err := dynamic.NewForConfig(s.restConfig)
	if err != nil {
		result.HealthScore = 100
		result.Recommendations = append(result.Recommendations, "Dynamic client not available — cannot scan GitOps CRDs")
		return result
	}

	// Detect ArgoCD
	argoCDApps := s.scanArgoCDApplications(ctx, dynClient)
	// Detect Flux
	fluxApps := s.scanFluxResources(ctx, dynClient)

	result.Applications = append(result.Applications, argoCDApps...)
	result.Applications = append(result.Applications, fluxApps...)

	// Build summary
	result.Summary.ArgoCDDetected = len(argoCDApps) > 0
	result.Summary.FluxDetected = len(fluxApps) > 0
	result.Summary.TotalApps = len(result.Applications)

	for _, app := range result.Applications {
		switch app.Tool {
		case "argocd":
			result.Summary.ArgoCDApps++
		case "flux":
			switch app.Kind {
			case "GitRepository":
				result.Summary.FluxSources++
			case "Kustomization":
				result.Summary.FluxKustomizations++
			case "HelmRelease":
				result.Summary.FluxHelmReleases++
			}
		}

		if app.SyncStatus == "Synced" && app.HealthStatus == "Healthy" {
			result.Summary.HealthyApps++
		} else if app.SyncStatus == "OutOfSync" {
			result.Summary.OutOfSyncApps++
		} else if app.SyncStatus == "Failed" || app.HealthStatus == "Degraded" {
			result.Summary.SyncFailedApps++
		}
		if app.Stale {
			result.Summary.StaleApps++
		}
		if !app.AutoSync {
			result.Summary.NoAutoSyncApps++
		}
		if app.Drift {
			result.Summary.DriftDetected++
		}
	}

	// Build per-namespace stats
	nsMap := map[string]*GitOpsNSStat{}
	for _, app := range result.Applications {
		ns, ok := nsMap[app.Namespace]
		if !ok {
			ns = &GitOpsNSStat{Namespace: app.Namespace}
			nsMap[app.Namespace] = ns
		}
		ns.TotalApps++
		if app.SyncStatus == "Synced" && app.HealthStatus == "Healthy" {
			ns.HealthyApps++
		}
		if app.SyncStatus == "OutOfSync" {
			ns.OutOfSyncApps++
		}
		if app.SyncStatus == "Failed" || app.HealthStatus == "Degraded" {
			ns.FailedApps++
		}
	}
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Namespace < result.ByNamespace[j].Namespace
	})

	// Generate issues
	for _, app := range result.Applications {
		if app.SyncStatus == "Failed" {
			result.Issues = append(result.Issues, GitOpsIssue{
				Severity:   "critical",
				Resource:   fmt.Sprintf("%s/%s", app.Kind, app.Name),
				Namespace:  app.Namespace,
				Issue:      fmt.Sprintf("Sync failed for %s application %s", app.Tool, app.Name),
				Suggestion: "Check controller logs and resolve sync errors",
			})
		}
		if app.SyncStatus == "OutOfSync" && app.AutoSync {
			result.Issues = append(result.Issues, GitOpsIssue{
				Severity:   "warning",
				Resource:   fmt.Sprintf("%s/%s", app.Kind, app.Name),
				Namespace:  app.Namespace,
				Issue:      fmt.Sprintf("Application %s is OutOfSync with auto-sync enabled", app.Name),
				Suggestion: "Review sync window or check for sync errors",
			})
		}
		if app.Stale {
			result.Issues = append(result.Issues, GitOpsIssue{
				Severity:   "warning",
				Resource:   fmt.Sprintf("%s/%s", app.Kind, app.Name),
				Namespace:  app.Namespace,
				Issue:      fmt.Sprintf("Application %s has not synced in >24h", app.Name),
				Suggestion: "Check Git repository connectivity and controller health",
			})
		}
		if app.Drift {
			result.Issues = append(result.Issues, GitOpsIssue{
				Severity:   "warning",
				Resource:   fmt.Sprintf("%s/%s", app.Kind, app.Name),
				Namespace:  app.Namespace,
				Issue:      fmt.Sprintf("Configuration drift detected for %s", app.Name),
				Suggestion: "Run a sync to bring cluster state in line with Git",
			})
		}
		if app.HealthStatus == "Degraded" {
			result.Issues = append(result.Issues, GitOpsIssue{
				Severity:   "critical",
				Resource:   fmt.Sprintf("%s/%s", app.Kind, app.Name),
				Namespace:  app.Namespace,
				Issue:      fmt.Sprintf("Application %s health is Degraded", app.Name),
				Suggestion: "Check deployed resources and their health status",
			})
		}
	}

	// Recommendations
	if !result.Summary.ArgoCDDetected && !result.Summary.FluxDetected {
		result.Recommendations = append(result.Recommendations, "No GitOps tools detected — consider adopting ArgoCD or Flux for declarative deployment management")
	}
	if result.Summary.SyncFailedApps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d application(s) have sync failures — investigate controller logs and resolve errors", result.Summary.SyncFailedApps))
	}
	if result.Summary.OutOfSyncApps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d application(s) are out of sync — run manual sync or enable auto-sync", result.Summary.OutOfSyncApps))
	}
	if result.Summary.StaleApps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d application(s) have not synced in >24h — check repository connectivity", result.Summary.StaleApps))
	}
	if result.Summary.NoAutoSyncApps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d application(s) have auto-sync disabled — enable for continuous reconciliation", result.Summary.NoAutoSyncApps))
	}
	if result.Summary.DriftDetected > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d application(s) have configuration drift — sync to align cluster with Git state", result.Summary.DriftDetected))
	}

	// Health score
	score := 100
	score -= result.Summary.SyncFailedApps * 15
	score -= result.Summary.OutOfSyncApps * 5
	score -= result.Summary.StaleApps * 5
	score -= result.Summary.DriftDetected * 5
	if result.Summary.TotalApps > 0 {
		healthyRatio := result.Summary.HealthyApps * 100 / result.Summary.TotalApps
		if healthyRatio < 50 {
			score -= 20
		} else if healthyRatio < 80 {
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	return result
}

// scanArgoCDApplications scans for ArgoCD Application CRDs using dynamic client.
func (s *Server) scanArgoCDApplications(ctx context.Context, dynClient dynamic.Interface) []GitOpsAppEntry {
	var apps []GitOpsAppEntry

	if dynClient == nil {
		return apps
	}

	// ArgoCD Application CRD: argoproj.io/v1alpha1, kind Application
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return apps
	}

	for _, item := range list.Items {
		obj := item.Object
		name := item.GetName()
		ns := item.GetNamespace()

		entry := GitOpsAppEntry{
			Name:      name,
			Namespace: ns,
			Tool:      "argocd",
			Kind:      "Application",
		}

		// Extract sync status
		if status, ok := obj["status"].(map[string]interface{}); ok {
			if sync, ok := status["sync"].(map[string]interface{}); ok {
				if statusStr, ok := sync["status"].(string); ok {
					entry.SyncStatus = statusStr
				}
				if rev, ok := sync["revision"].(string); ok {
					entry.TargetRevision = rev
				}
			}
			if health, ok := status["health"].(map[string]interface{}); ok {
				if hs, ok := health["status"].(string); ok {
					entry.HealthStatus = hs
				}
			}
			if opStatus, ok := status["operationState"].(map[string]interface{}); ok {
				if finishedAt, ok := opStatus["finishedAt"].(string); ok {
					entry.LastSyncAt = finishedAt
				}
			}
		}

		// Extract spec
		if spec, ok := obj["spec"].(map[string]interface{}); ok {
			if source, ok := spec["source"].(map[string]interface{}); ok {
				if repoURL, ok := source["repoURL"].(string); ok {
					entry.RepoURL = repoURL
				}
				if targetRev, ok := source["targetRevision"].(string); ok {
					if entry.TargetRevision == "" {
						entry.TargetRevision = targetRev
					}
				}
			}
			if syncPolicy, ok := spec["syncPolicy"].(map[string]interface{}); ok {
				if automated, ok := syncPolicy["automated"].(map[string]interface{}); ok {
					entry.AutoSync = automated != nil
				}
			}
		}

		// Default values if empty
		if entry.SyncStatus == "" {
			entry.SyncStatus = "Unknown"
		}
		if entry.HealthStatus == "" {
			entry.HealthStatus = "Unknown"
		}

		// Check stale (last sync > 24h)
		if entry.LastSyncAt != "" {
			if t, err := time.Parse(time.RFC3339, entry.LastSyncAt); err == nil {
				if time.Since(t) > 24*time.Hour {
					entry.Stale = true
				}
			}
		}

		// Drift = OutOfSync with auto-sync
		if entry.SyncStatus == "OutOfSync" && entry.AutoSync {
			entry.Drift = true
		}

		entry.RiskLevel = assessGitOpsRisk(entry)
		apps = append(apps, entry)
	}

	return apps
}

// scanFluxResources scans for Flux CRDs (GitRepository, Kustomization, HelmRelease) using dynamic client.
func (s *Server) scanFluxResources(ctx context.Context, dynClient dynamic.Interface) []GitOpsAppEntry {
	var apps []GitOpsAppEntry

	if dynClient == nil {
		return apps
	}

	// Flux source toolkit.dev/v1, GitRepository
	fluxCRDs := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{
			gvr:  schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
			kind: "GitRepository",
		},
		{
			gvr:  schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
			kind: "Kustomization",
		},
		{
			gvr:  schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
			kind: "HelmRelease",
		},
	}

	for _, crd := range fluxCRDs {
		list, err := dynClient.Resource(crd.gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, item := range list.Items {
			obj := item.Object
			name := item.GetName()
			ns := item.GetNamespace()

			entry := GitOpsAppEntry{
				Name:      name,
				Namespace: ns,
				Tool:      "flux",
				Kind:      crd.kind,
			}

			// Extract conditions from status
			if status, ok := obj["status"].(map[string]interface{}); ok {
				if conditions, ok := status["conditions"].([]interface{}); ok {
					for _, c := range conditions {
						cond, ok := c.(map[string]interface{})
						if !ok {
							continue
						}
						condType, _ := cond["type"].(string)
						condStatus, _ := cond["status"].(string)
						condMessage, _ := cond["message"].(string)

						if condType == "Ready" {
							if condStatus == "True" {
								entry.SyncStatus = "Synced"
								entry.HealthStatus = "Healthy"
							} else if condStatus == "False" {
								entry.SyncStatus = "Failed"
								entry.HealthStatus = "Degraded"
							} else {
								entry.SyncStatus = "Unknown"
								entry.HealthStatus = "Unknown"
							}
						}
						if condType == "ReadyInSync" && condStatus == "False" {
							entry.SyncStatus = "OutOfSync"
						}
						_ = condMessage
					}
				}
				// Last handled revision
				if artifact, ok := status["artifact"].(map[string]interface{}); ok {
					if rev, ok := artifact["revision"].(string); ok {
						entry.TargetRevision = rev
					}
				}
			}

			// Extract spec for source
			if spec, ok := obj["spec"].(map[string]interface{}); ok {
				if crd.kind == "GitRepository" {
					if url, ok := spec["url"].(string); ok {
						entry.RepoURL = url
					}
				}
				// Flux always auto-syncs (no manual sync option)
				entry.AutoSync = true
			}

			// Default values
			if entry.SyncStatus == "" {
				entry.SyncStatus = "Unknown"
			}
			if entry.HealthStatus == "" {
				entry.HealthStatus = "Unknown"
			}

			// Stale check: Flux uses lastHandledReconcile
			if status, ok := obj["status"].(map[string]interface{}); ok {
				if lastReconcile, ok := status["lastHandledReconcile"].(string); ok {
					if t, err := time.Parse(time.RFC3339, lastReconcile); err == nil {
						if time.Since(t) > 24*time.Hour {
							entry.Stale = true
						}
						entry.LastSyncAt = lastReconcile
					}
				}
			}

			entry.RiskLevel = assessGitOpsRisk(entry)
			apps = append(apps, entry)
		}
	}

	return apps
}

// assessGitOpsRisk determines risk level for a GitOps application.
func assessGitOpsRisk(entry GitOpsAppEntry) string {
	if entry.SyncStatus == "Failed" || entry.HealthStatus == "Degraded" {
		return "critical"
	}
	if entry.SyncStatus == "OutOfSync" || entry.Drift {
		return "warning"
	}
	if entry.Stale {
		return "warning"
	}
	if entry.SyncStatus == "Unknown" || entry.HealthStatus == "Unknown" {
		return "info"
	}
	return "healthy"
}

// formatGitOpsSummary returns a human-readable summary for audit output.
func formatGitOpsSummary(r *GitOpsSyncResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GitOps sync audit: %d apps, %d healthy, %d out-of-sync, %d failed",
		r.Summary.TotalApps, r.Summary.HealthyApps, r.Summary.OutOfSyncApps, r.Summary.SyncFailedApps)
	if r.Summary.ArgoCDDetected {
		fmt.Fprintf(&b, " | ArgoCD: %d apps", r.Summary.ArgoCDApps)
	}
	if r.Summary.FluxDetected {
		fmt.Fprintf(&b, " | Flux: %d sources, %d kustomizations, %d helm releases",
			r.Summary.FluxSources, r.Summary.FluxKustomizations, r.Summary.FluxHelmReleases)
	}
	return b.String()
}

// Ensure we don't shadow the corev1 import (used for potential pod checks).
var _ corev1.PodPhase
