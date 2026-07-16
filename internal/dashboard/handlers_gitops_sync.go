package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOpsSyncResult analyzes GitOps synchronization state: ArgoCD/Flux
// application health, sync status, and configuration drift detection.
type GitOpsSyncResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         GitOpsSummary       `json:"summary"`
	OutOfSyncApps   []OutOfSyncApp      `json:"outOfSyncApps"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type GitOpsSummary struct {
	HasArgoCD       bool `json:"hasArgoCD"`
	HasFlux         bool `json:"hasFlux"`
	TotalApps      int  `json:"totalApps"`
	SyncedApps     int  `json:"syncedApps"`
	HealthyApps    int  `json:"healthyApps"`
	OutOfSyncApps  int  `json:"outOfSyncApps"`
	OutOfSyncCount int  `json:"outOfSyncCount"`
	DegradedApps   int  `json:"degradedApps"`
	SyncFailedApps int  `json:"syncFailedApps"`
	ArgoCDDetected bool  `json:"argoCDDetected"`
	ArgoCDApps     int   `json:"argoCDApps"`
	FluxDetected   bool  `json:"fluxDetected"`
	FluxSources    int   `json:"fluxSources"`
	FluxKustomizations int `json:"fluxKustomizations"`
	StaleApps      int   `json:"staleApps"`
	NoAutoSyncApps int   `json:"noAutoSyncApps"`
	DriftDetected  int   `json:"driftDetected"`
}

// formatGitOpsSummary produces a human-readable summary string.
func formatGitOpsSummary(r *GitOpsSyncResult) string {
	s := r.Summary
	return fmt.Sprintf("total:%d synced:%d healthy:%d outOfSync:%d degraded:%d drift:%d",
		s.TotalApps, s.SyncedApps, s.HealthyApps, s.OutOfSyncCount, s.DegradedApps, s.DriftDetected)
}

type OutOfSyncApp struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	Severity  string `json:"severity"`
}

// GitOpsAppEntry represents a single GitOps-managed application entry.
type GitOpsAppEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	SyncStatus   string `json:"syncStatus"`
	HealthStatus string `json:"healthStatus"`
	Drift        bool   `json:"drift"`
	Stale        bool   `json:"stale"`
	Kind         string `json:"kind"`
	AutoSync     bool   `json:"autoSync"`
	RiskLevel    string `json:"riskLevel"`
	Tool         string `json:"tool"`
}

// assessGitOpsRisk evaluates risk level for a GitOps app entry.
func assessGitOpsRisk(entry GitOpsAppEntry) string {
	if entry.SyncStatus == "Failed" || entry.HealthStatus == "Degraded" {
		return "critical"
	}
	if entry.Drift || entry.SyncStatus == "OutOfSync" {
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

// GitOpsIssue represents a finding from the GitOps sync audit.
type GitOpsIssue struct {
	App       string `json:"app"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Issue     string `json:"issue"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// handleGitOpsSync analyzes GitOps synchronization state.
// GET /api/deployment/gitops-sync
func (s *Server) handleGitOpsSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GitOpsSyncResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Detect GitOps controllers
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := ""
			for _, c2 := range pod.Spec.Containers {
				imgLower += c2.Image + " "
			}
			_ = c
			_ = imgLower
		}
		for _, c := range pod.Spec.Containers {
			imgLower := toLower(c.Image)
			if contains(imgLower, "argocd") {
				result.Summary.HasArgoCD = true
			}
			if contains(imgLower, "flux") {
				result.Summary.HasFlux = true
			}
		}
	}

	// Find ArgoCD applications (via ConfigMaps or CRDs)
	for _, cm := range configmaps.Items {
		if contains(toLower(cm.Name), "argocd") || contains(toLower(cm.Name), "flux") {
			result.Summary.TotalApps++
		}
	}

	// Check for Helm secrets (Flux helm releases stored as secrets)
	for _, sec := range secrets.Items {
		if sec.Type == "helm.sh/release.v1" {
			result.Summary.TotalApps++
		}
	}

	// If no GitOps tool detected, count deployments as potentially-managed
	if !result.Summary.HasArgoCD && !result.Summary.HasFlux {
		for _, dep := range deployments.Items {
			if systemNS[dep.Namespace] { continue }
			result.Summary.TotalApps++
			// Check for sync annotations
			hasSyncAnno := false
			for k := range dep.Annotations {
				if contains(toLower(k), "argocd") || contains(toLower(k), "flux") {
					hasSyncAnno = true
					break
				}
			}
			if !hasSyncAnno {
				result.Summary.OutOfSyncCount++
				result.OutOfSyncApps = append(result.OutOfSyncApps, OutOfSyncApp{
					Name: dep.Name, Namespace: dep.Namespace,
					Tool: "none", Status: "manual-deploy", Severity: "medium",
				})
			}
		}
	}

	result.Summary.SyncedApps = result.Summary.TotalApps - result.Summary.OutOfSyncCount
	if result.Summary.SyncedApps < 0 { result.Summary.SyncedApps = 0 }

	// Score
	score := 30
	if result.Summary.HasArgoCD || result.Summary.HasFlux { score += 40 }
	if result.Summary.TotalApps > 0 {
		score += result.Summary.SyncedApps * 30 / result.Summary.TotalApps
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.OutOfSyncApps, func(i, j int) bool {
		return result.OutOfSyncApps[i].Severity > result.OutOfSyncApps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("GitOps sync: %d/100 (grade %s) — ArgoCD:%v Flux:%v apps:%d synced:%d", result.HealthScore, result.Grade, result.Summary.HasArgoCD, result.Summary.HasFlux, result.Summary.TotalApps, result.Summary.SyncedApps))
	if !result.Summary.HasArgoCD && !result.Summary.HasFlux {
		recs = append(recs, "No GitOps tool detected — deploy ArgoCD or Flux for declarative sync")
	}
	if result.Summary.OutOfSyncCount > 0 {
		recs = append(recs, fmt.Sprintf("%d apps not under GitOps control — adopt into GitOps for drift prevention", result.Summary.OutOfSyncCount))
	}
	if len(recs) == 1 { recs = append(recs, "GitOps sync is healthy — all apps declaratively managed") }
	result.Recommendations = recs

	writeJSON(w, result)
}

// toLower wraps strings.ToLower for readability.
func toLower(s string) string {
	return strings.ToLower(s)
}

// toLowerStr is an alias for toLower.
func toLowerStr(s string) string {
	return strings.ToLower(s)
}
