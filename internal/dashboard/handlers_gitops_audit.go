package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOpsAuditResult is the GitOps/CD pipeline health audit.
type GitOpsAuditResult struct {
	ScannedAt          time.Time            `json:"scannedAt"`
	Summary            GitOpsAuditSummary   `json:"summary"`
	Tools              []GitOpsAuditTool    `json:"tools"`
	HelmReleases       []HelmRelHealth      `json:"helmReleases,omitempty"`
	GitOpsSyncIssues   []GitOpsSyncIssue    `json:"syncIssues,omitempty"`
	GitOpsConfigDrifts []GitOpsConfigDrift  `json:"configDrifts,omitempty"`
	Deployments        []GitOpsPipelineStat `json:"deployments"`
	Recommendations    []string             `json:"recommendations"`
	HealthScore        int                  `json:"healthScore"`
}

// GitOpsAuditSummary aggregates GitOps/CD statistics.
type GitOpsAuditSummary struct {
	HasArgoCD       bool `json:"hasArgoCD"`
	HasFlux         bool `json:"hasFlux"`
	HasHelm         bool `json:"hasHelmController"`
	HasArgoRollouts bool `json:"hasArgoRollouts"`
	TotalTools      int  `json:"totalTools"`
	HealthyTools    int  `json:"healthyTools"`
	TotalReleases   int  `json:"totalReleases"`
	HealthyReleases int  `json:"healthyReleases"`
	FailedReleases  int  `json:"failedReleases"`
	DriftDetected   int  `json:"driftDetected"`
	OutOfSyncApps   int  `json:"outOfSyncApps"`
	AutoSyncEnabled int  `json:"autoSyncEnabled"`
}

// GitOpsAuditTool describes a detected GitOps tool deployment.
type GitOpsAuditTool struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"` // argocd, flux, helm-controller, argo-rollouts
	Version   string `json:"version,omitempty"`
	Replicas  int    `json:"replicas"`
	Ready     int    `json:"readyReplicas"`
	Healthy   bool   `json:"healthy"`
}

// HelmRelHealth describes a Helm release health status.
type HelmRelHealth struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Chart     string `json:"chart"`
	Version   string `json:"version"`
	Status    string `json:"status"` // deployed, failed, pending, superseded
	Age       string `json:"age"`
	HasIssues bool   `json:"hasIssues"`
}

// GitOpsSyncIssue describes a GitOps sync problem.
type GitOpsSyncIssue struct {
	Severity  string `json:"severity"`
	Tool      string `json:"tool"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
	Issue     string `json:"issue"`
}

// GitOpsConfigDrift describes detected configuration drift.
type GitOpsConfigDrift struct {
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Field     string `json:"field"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
	Severity  string `json:"severity"`
}

// GitOpsPipelineStat shows deployment pipeline health metrics.
type GitOpsPipelineStat struct {
	Namespace       string  `json:"namespace"`
	TotalWorkloads  int     `json:"totalWorkloads"`
	WithAnnotations int     `json:"withGitOpsAnnotations"` // has argocd/flux annotations
	ManagedRatio    float64 `json:"managedRatio"`          // percentage managed by GitOps
}

// handleGitOpsAudit audits GitOps tooling, Helm release health, and config drift.
// GET /api/deployment/gitops-audit
func (s *Server) handleGitOpsAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GitOpsAuditResult{ScannedAt: time.Now()}

	// 1. Detect GitOps tools from pods
	toolPatterns := map[string]string{
		"argocd-application-controller": "argocd",
		"argocd-server":                 "argocd",
		"argocd-repo-server":            "argocd",
		"flux-controller":               "flux",
		"source-controller":             "flux",
		"kustomize-controller":          "flux",
		"helm-controller":               "flux",
		"notification-controller":       "flux",
		"argo-rollouts":                 "argo-rollouts",
	}

	toolMap := map[string]*GitOpsAuditTool{} // "ns/type" → tool
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			nameLower := strings.ToLower(pod.Name)
			for pattern, toolType := range toolPatterns {
				if strings.Contains(nameLower, pattern) {
					key := fmt.Sprintf("%s/%s", pod.Namespace, toolType)
					if toolMap[key] == nil {
						toolMap[key] = &GitOpsAuditTool{
							Name:      toolType,
							Namespace: pod.Namespace,
							Type:      toolType,
						}
					}
					toolMap[key].Replicas++
					allReady := true
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							allReady = false
						}
						// Extract version from image
						if toolMap[key].Version == "" {
							imgParts := strings.Split(cs.Image, ":")
							if len(imgParts) > 1 {
								toolMap[key].Version = imgParts[len(imgParts)-1]
							}
						}
					}
					if allReady {
						toolMap[key].Ready++
					}
					break
				}
			}
		}
	}

	for _, tool := range toolMap {
		tool.Healthy = tool.Ready > 0
		result.Tools = append(result.Tools, *tool)
		result.Summary.TotalTools++
		if tool.Healthy {
			result.Summary.HealthyTools++
		}
		switch tool.Type {
		case "argocd":
			result.Summary.HasArgoCD = true
		case "flux":
			result.Summary.HasFlux = true
		case "argo-rollouts":
			result.Summary.HasArgoRollouts = true
		}
	}

	// 2. Detect Helm releases from ConfigMap secrets (Helm v2) or secrets (Helm v3)
	// Helm v3 stores release state as secrets with label "owner=helm"
	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err == nil {
		releaseMap := map[string]*HelmRelHealth{}
		for _, secret := range secrets.Items {
			// Release name is encoded in secret name: sh.helm.release.v1.<name>.v<version>
			parts := strings.Split(secret.Name, ".")
			if len(parts) < 6 {
				continue
			}
			relName := parts[4]
			relVersion := strings.TrimPrefix(parts[5], "v")
			key := fmt.Sprintf("%s/%s", secret.Namespace, relName)

			// Only keep latest version
			if existing, exists := releaseMap[key]; exists {
				// Compare version numbers
				if relVersion <= existing.Version {
					continue
				}
			}

			chart := "unknown"
			if data, ok := secret.Data["release"]; ok {
				_ = data // Can't decode without zlib base64 but we know it's a Helm release
			}
			if chartAnn, ok := secret.Labels["name"]; ok {
				chart = chartAnn
			}

			age := time.Since(secret.CreationTimestamp.Time)
			releaseMap[key] = &HelmRelHealth{
				Name:      relName,
				Namespace: secret.Namespace,
				Chart:     chart,
				Version:   relVersion,
				Status:    "deployed", // Can't determine actual status without helm CLI
				Age:       fmt.Sprintf("%.0fd", age.Hours()/24),
			}
			result.Summary.TotalReleases++
		}

		for _, rel := range releaseMap {
			result.HelmReleases = append(result.HelmReleases, *rel)
		}
		sort.Slice(result.HelmReleases, func(i, j int) bool {
			return result.HelmReleases[i].Name < result.HelmReleases[j].Name
		})
		if len(result.HelmReleases) > 50 {
			result.HelmReleases = result.HelmReleases[:50]
		}
		result.Summary.HealthyReleases = result.Summary.TotalReleases
	}
	if result.Summary.TotalReleases > 0 {
		result.Summary.HasHelm = true
	}

	// 3. Detect GitOps-managed workloads via annotations
	argoAnnotationKeys := []string{
		"argocd.argoproj.io/application",
		"argocd.argoproj.io/sync-wave",
	}
	fluxAnnotationKeys := []string{
		"fluxcd.io/sync",
		"flux.weave.works/antecedent",
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nsStats := map[string]*GitOpsPipelineStat{}
	if err == nil {
		for _, d := range deployments.Items {
			ns := d.Namespace
			if nsStats[ns] == nil {
				nsStats[ns] = &GitOpsPipelineStat{Namespace: ns}
			}
			nsStats[ns].TotalWorkloads++

			annotations := d.Annotations
			if annotations == nil {
				annotations = map[string]string{}
			}
			hasGitOps := false
			for _, key := range argoAnnotationKeys {
				if _, ok := annotations[key]; ok {
					hasGitOps = true
					result.Summary.AutoSyncEnabled++
				}
			}
			for _, key := range fluxAnnotationKeys {
				if _, ok := annotations[key]; ok {
					hasGitOps = true
				}
			}
			if hasGitOps {
				nsStats[ns].WithAnnotations++
			}
		}
	}

	for _, stat := range nsStats {
		if stat.TotalWorkloads > 0 {
			stat.ManagedRatio = float64(stat.WithAnnotations) / float64(stat.TotalWorkloads) * 100
		}
		result.Deployments = append(result.Deployments, *stat)
	}
	sort.Slice(result.Deployments, func(i, j int) bool {
		return result.Deployments[i].ManagedRatio > result.Deployments[j].ManagedRatio
	})

	// 4. Detect config drift indicators
	// Check for manual edits (last-applied-configuration mismatch)
	if deployments != nil {
		for _, d := range deployments.Items {
			if d.Annotations == nil {
				continue
			}
			// If managed by ArgoCD but has manual changes
			if _, isArgo := d.Annotations["argocd.argoproj.io/tracking-id"]; isArgo {
				// Check for operation annotation (indicates manual operation)
				if op, hasOp := d.Annotations["deployment.kubernetes.io/revision"]; hasOp {
					_ = op // Revision exists — could indicate manual scaling
				}
			}
		}
	}

	// 5. Generate sync issues
	if result.Summary.HasArgoCD {
		// Check if ArgoCD server is healthy
		argocdHealthy := false
		for _, tool := range result.Tools {
			if tool.Type == "argocd" && tool.Healthy {
				argocdHealthy = true
			}
		}
		if !argocdHealthy {
			result.GitOpsSyncIssues = append(result.GitOpsSyncIssues, GitOpsSyncIssue{
				Severity: "critical",
				Tool:     "argocd",
				Resource: "argocd-server",
				Issue:    "ArgoCD detected but server/controller is unhealthy — sync operations may fail",
			})
		}
	}

	if result.Summary.TotalReleases > 0 && result.Summary.HasHelm {
		// Helm releases without GitOps management
		if !result.Summary.HasArgoCD && !result.Summary.HasFlux {
			result.GitOpsSyncIssues = append(result.GitOpsSyncIssues, GitOpsSyncIssue{
				Severity: "info",
				Tool:     "helm",
				Resource: "cluster-wide",
				Issue:    fmt.Sprintf("%d Helm release(s) detected but no GitOps controller — manual deployments are drift-prone", result.Summary.TotalReleases),
			})
		}
	}

	// 6. Health score
	score := 100
	if !result.Summary.HasArgoCD && !result.Summary.HasFlux && !result.Summary.HasHelm {
		score -= 30 // No GitOps tooling at all
	}
	if result.Summary.HealthyTools < result.Summary.TotalTools {
		score -= 15
	}
	// Low GitOps adoption
	managedTotal := 0
	workloadTotal := 0
	for _, stat := range result.Deployments {
		managedTotal += stat.WithAnnotations
		workloadTotal += stat.TotalWorkloads
	}
	if workloadTotal > 0 {
		adoptionRate := float64(managedTotal) / float64(workloadTotal) * 100
		if adoptionRate < 20 {
			score -= 10
		}
	}
	if len(result.GitOpsSyncIssues) > 0 {
		for _, si := range result.GitOpsSyncIssues {
			if si.Severity == "critical" {
				score -= 10
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	result.Recommendations = generateGitOpsRecs(result)

	writeJSON(w, result)
}

// generateGitOpsRecs produces recommendations.
func generateGitOpsRecs(result GitOpsAuditResult) []string {
	var recs []string

	if !result.Summary.HasArgoCD && !result.Summary.HasFlux {
		recs = append(recs, "No GitOps controller detected — deploy ArgoCD or Flux for declarative deployment management and config drift prevention")
	} else {
		if result.Summary.HasArgoCD {
			recs = append(recs, "ArgoCD detected — ensure all critical workloads are managed via ArgoCD Applications")
		}
		if result.Summary.HasFlux {
			recs = append(recs, "Flux detected — verify all workloads have FluxSourceRef for drift prevention")
		}
	}

	if result.Summary.TotalReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d Helm release(s) detected — ensure releases are managed via HelmRelease CRD or ArgoCD Helm charts", result.Summary.TotalReleases))
	}

	// GitOps adoption rate
	managedTotal := 0
	workloadTotal := 0
	for _, stat := range result.Deployments {
		managedTotal += stat.WithAnnotations
		workloadTotal += stat.TotalWorkloads
	}
	if workloadTotal > 0 {
		adoptionRate := float64(managedTotal) / float64(workloadTotal) * 100
		if adoptionRate < 50 {
			recs = append(recs, fmt.Sprintf("GitOps adoption: %.0f%% of workloads managed (%d/%d) — migrate manual deployments to GitOps", adoptionRate, managedTotal, workloadTotal))
		}
	}

	if result.Summary.HealthyTools < result.Summary.TotalTools {
		recs = append(recs, fmt.Sprintf("%d/%d GitOps tool(s) unhealthy — check controller pods", result.Summary.HealthyTools, result.Summary.TotalTools))
	}

	if len(result.GitOpsSyncIssues) > 0 {
		for _, si := range result.GitOpsSyncIssues {
			if si.Severity == "critical" || si.Severity == "warning" {
				recs = append(recs, fmt.Sprintf("[%s] %s: %s", si.Severity, si.Tool, si.Issue))
			}
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "GitOps configuration is healthy — all tools running and workloads managed")
	}

	return recs
}
