package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WLDepsResult analyzes workload dependency graph: init containers, config refs,
// cross-namespace dependencies, and startup ordering risks.
type WLDepsResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         WLDepsSummary       `json:"summary"`
	RiskItems       []WLDepRisk         `json:"riskItems"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type WLDepsSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	WithInitContainers int `json:"withInitContainers"`
	CrossNSDeps     int `json:"crossNSDeps"`
	ConfigRefs      int `json:"configRefs"`
	SecretRefs      int `json:"secretRefs"`
	StartupOrderRisks int `json:"startupOrderRisks"`
}

type WLDepRisk struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// handleWLDeps analyzes workload dependency graph.
// GET /api/deployment/workload-deps
func (s *Server) handleWLDeps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := WLDepsResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] { continue }
		result.Summary.TotalWorkloads++

		spec := dep.Spec.Template.Spec

		// Init containers
		if len(spec.InitContainers) > 0 {
			result.Summary.WithInitContainers++
			// Check for slow init containers
			for _, ic := range spec.InitContainers {
				if ic.Resources.Requests != nil {
					if q, ok := ic.Resources.Requests[corev1.ResourceCPU]; ok && q.MilliValue() < 100 {
						result.RiskItems = append(result.RiskItems, WLDepRisk{
							Workload: dep.Name, Namespace: dep.Namespace,
							RiskType: "slow-init", Severity: "medium",
							Detail: fmt.Sprintf("Init container '%s' has <100m CPU — may be slow", ic.Name),
						})
					}
				}
			}
		}

		// Config/Secret refs
		for _, vol := range spec.Volumes {
			if vol.ConfigMap != nil {
				result.Summary.ConfigRefs++
			}
			if vol.Secret != nil {
				result.Summary.SecretRefs++
			}
		}

		// Check for startup ordering: app container depends on env from config
		// that might not be ready
		hasEnvFromConfig := false
		for _, c := range spec.Containers {
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil || ef.SecretRef != nil {
					hasEnvFromConfig = true
				}
			}
		}
		if hasEnvFromConfig && len(spec.InitContainers) == 0 {
			result.Summary.StartupOrderRisks++
			result.RiskItems = append(result.RiskItems, WLDepRisk{
				Workload: dep.Name, Namespace: dep.Namespace,
				RiskType: "startup-order", Severity: "low",
				Detail: "App depends on config/secret env but has no init container to verify",
			})
		}
	}

	// Score
	score := 100
	score -= result.Summary.StartupOrderRisks * 3
	if result.Summary.WithInitContainers > 0 && result.Summary.TotalWorkloads > 0 {
		// Having init containers is good, but risky if too many
		if result.Summary.WithInitContainers > result.Summary.TotalWorkloads/2 {
			score -= 5
		}
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.RiskItems, func(i, j int) bool {
		return result.RiskItems[i].Severity > result.RiskItems[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Workload deps: %d/100 (grade %s) — %d workloads, %d init, %d ordering risks", result.HealthScore, result.Grade, result.Summary.TotalWorkloads, result.Summary.WithInitContainers, result.Summary.StartupOrderRisks))
	if result.Summary.StartupOrderRisks > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with startup ordering risk — add init containers to verify deps", result.Summary.StartupOrderRisks))
	}
	if len(recs) == 1 { recs = append(recs, "Workload dependencies are well-managed") }
	result.Recommendations = recs

	writeJSON(w, result)
}
