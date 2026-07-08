package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WMResult is the workload maturity analysis.
type WMResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         WMSummary `json:"summary"`
	ByWorkload      []WMEntry `json:"byWorkload"`
	LowMaturity     []WMEntry `json:"lowMaturity"`
	Issues          []WMIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// WMSummary aggregates maturity stats.
type WMSummary struct {
	TotalWorkloads     int     `json:"totalWorkloads"`
	HasResources       int     `json:"hasResources"`
	HasProbes          int     `json:"hasProbes"`
	HasReplicas        int     `json:"hasReplicas"` // >1 replica
	HasPDB             int     `json:"hasPDB"`
	HasAntiAffinity    int     `json:"hasAntiAffinity"`
	HasSecurityCtx     int     `json:"hasSecurityCtx"`
	HasRevisionHistory int     `json:"hasRevisionHistory"`
	AvgMaturityScore   float64 `json:"avgMaturityScore"`
}

// WMEntry describes one workload's maturity.
type WMEntry struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	Kind          string    `json:"kind"`
	Replicas      int32     `json:"replicas"`
	Checks        []WMCheck `json:"checks"`
	MaturityScore int       `json:"maturityScore"` // 0-100
	RiskLevel     string    `json:"riskLevel"`
}

// WMCheck describes one best-practice check.
type WMCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Weight int    `json:"weight"`
}

// WMIssue is a detected maturity problem.
type WMIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleWorkloadMaturity scores deployments against K8s best practices.
// GET /api/deployment/workload-maturity
func (s *Server) handleWorkloadMaturity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get PDBs for coverage check
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbNSMap := make(map[string]int) // namespace → PDB count
	if pdbs != nil {
		for _, pdb := range pdbs.Items {
			pdbNSMap[pdb.Namespace]++
		}
	}

	result := WMResult{ScannedAt: time.Now()}
	var totalScore int

	for _, dep := range deployments.Items {
		result.Summary.TotalWorkloads++

		entry := WMEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
		}

		repl := int32(1)
		if dep.Spec.Replicas != nil {
			repl = *dep.Spec.Replicas
		}
		entry.Replicas = repl

		// Check 1: Has resource requests/limits
		hasResources := true
		if len(dep.Spec.Template.Spec.Containers) > 0 {
			for _, c := range dep.Spec.Template.Spec.Containers {
				if c.Resources.Requests.Cpu().IsZero() || c.Resources.Requests.Memory().IsZero() {
					hasResources = false
					break
				}
			}
		} else {
			hasResources = false
		}
		entry.Checks = append(entry.Checks, WMCheck{Name: "resource-requests", Pass: hasResources, Weight: 15})
		if hasResources {
			result.Summary.HasResources++
		}

		// Check 2: Has readiness+liveness probes
		hasProbes := true
		if len(dep.Spec.Template.Spec.Containers) > 0 {
			for _, c := range dep.Spec.Template.Spec.Containers {
				if c.ReadinessProbe == nil || c.LivenessProbe == nil {
					hasProbes = false
					break
				}
			}
		} else {
			hasProbes = false
		}
		entry.Checks = append(entry.Checks, WMCheck{Name: "probes", Pass: hasProbes, Weight: 15})
		if hasProbes {
			result.Summary.HasProbes++
		}

		// Check 3: Multi-replica (>1)
		hasMultiReplica := repl > 1
		entry.Checks = append(entry.Checks, WMCheck{Name: "multi-replica", Pass: hasMultiReplica, Weight: 15})
		if hasMultiReplica {
			result.Summary.HasReplicas++
		}

		// Check 4: Has PDB
		hasPDB := pdbNSMap[dep.Namespace] > 0
		entry.Checks = append(entry.Checks, WMCheck{Name: "pdb", Pass: hasPDB, Weight: 10})
		if hasPDB {
			result.Summary.HasPDB++
		}

		// Check 5: Anti-affinity / topology spread
		hasAntiAffinity := false
		if dep.Spec.Template.Spec.Affinity != nil &&
			dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAffinity = true
		}
		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			hasAntiAffinity = true
		}
		entry.Checks = append(entry.Checks, WMCheck{Name: "anti-affinity", Pass: hasAntiAffinity, Weight: 15})
		if hasAntiAffinity {
			result.Summary.HasAntiAffinity++
		}

		// Check 6: Security context
		hasSecCtx := true
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.SecurityContext == nil {
				hasSecCtx = false
				break
			}
		}
		entry.Checks = append(entry.Checks, WMCheck{Name: "security-context", Pass: hasSecCtx, Weight: 10})
		if hasSecCtx {
			result.Summary.HasSecurityCtx++
		}

		// Check 7: Revision history limit >= 3
		hasRevHistory := false
		if dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit >= 3 {
			hasRevHistory = true
		}
		entry.Checks = append(entry.Checks, WMCheck{Name: "revision-history", Pass: hasRevHistory, Weight: 10})
		if hasRevHistory {
			result.Summary.HasRevisionHistory++
		}

		// Check 8: Has labels
		hasLabels := len(dep.Spec.Template.Labels) > 0
		entry.Checks = append(entry.Checks, WMCheck{Name: "labels", Pass: hasLabels, Weight: 10})

		// Calculate maturity score
		score := 0
		for _, c := range entry.Checks {
			if c.Pass {
				score += c.Weight
			}
		}
		entry.MaturityScore = score
		totalScore += score

		// Risk level
		if score < 50 {
			entry.RiskLevel = "high"
			result.LowMaturity = append(result.LowMaturity, entry)
			result.Issues = append(result.Issues, WMIssue{
				Severity: "warning", Type: "low-maturity",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has maturity score %d/100 — missing best practices", dep.Namespace, dep.Name, score),
			})
		} else if score < 80 {
			entry.RiskLevel = "medium"
		} else {
			entry.RiskLevel = "low"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgMaturityScore = float64(totalScore) / float64(result.Summary.TotalWorkloads)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].MaturityScore < result.ByWorkload[j].MaturityScore
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return wmIssueRank(result.Issues[i].Severity) < wmIssueRank(result.Issues[j].Severity)
	})

	result.Recommendations = wmGenRecs(result.Summary)

	writeJSON(w, result)
}

// wmGenRecs produces actionable advice.
func wmGenRecs(s WMSummary) []string {
	var recs []string

	if s.TotalWorkloads == 0 {
		return recs
	}

	if s.HasResources < s.TotalWorkloads {
		missing := s.TotalWorkloads - s.HasResources
		recs = append(recs, fmt.Sprintf("%d/%d workloads missing resource requests — add CPU/memory requests for proper scheduling", missing, s.TotalWorkloads))
	}
	if s.HasProbes < s.TotalWorkloads {
		missing := s.TotalWorkloads - s.HasProbes
		recs = append(recs, fmt.Sprintf("%d/%d workloads missing readiness/liveness probes — add probes for proper health checking", missing, s.TotalWorkloads))
	}
	if s.HasReplicas < s.TotalWorkloads {
		missing := s.TotalWorkloads - s.HasReplicas
		recs = append(recs, fmt.Sprintf("%d/%d workloads have single replica — add replicas for high availability", missing, s.TotalWorkloads))
	}
	if s.HasAntiAffinity < s.TotalWorkloads {
		missing := s.TotalWorkloads - s.HasAntiAffinity
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack anti-affinity/topology spread — add podAntiAffinity for cross-node distribution", missing, s.TotalWorkloads))
	}
	if s.HasSecurityCtx < s.TotalWorkloads {
		missing := s.TotalWorkloads - s.HasSecurityCtx
		recs = append(recs, fmt.Sprintf("%d/%d workloads missing security context — add securityContext for Pod Security Standards compliance", missing, s.TotalWorkloads))
	}
	if s.AvgMaturityScore < 70 {
		recs = append(recs, fmt.Sprintf("Average maturity score is %.1f/100 — review deployment best practices", s.AvgMaturityScore))
	}
	if s.AvgMaturityScore >= 80 {
		recs = append(recs, fmt.Sprintf("Workload maturity is excellent (avg: %.1f/100) — good operational posture", s.AvgMaturityScore))
	}

	return recs
}

func wmIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

var _ = appsv1.Deployment{}
var _ = corev1.PodSpec{}
var _ = strings.Contains
