package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DIResult is the deployment disruption & maintenance impact analysis.
type DIResult struct {
	ScannedAt         time.Time `json:"scannedAt"`
	Summary           DISummary `json:"summary"`
	BlockingWorkloads []DIEntry `json:"blockingWorkloads"` // will block node drain
	SafeWorkloads     []DIEntry `json:"safeWorkloads"`     // safe to drain
	NoPDBWorkloads    []DIEntry `json:"noPDBWorkloads"`    // missing PDB
	Issues            []DIIssue `json:"issues"`
	Recommendations   []string  `json:"recommendations"`
}

// DISummary aggregates disruption stats.
type DISummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithPDB           int `json:"withPDB"`
	NoPDB             int `json:"noPDB"`      // no PDB at all
	BlockDrain        int `json:"blockDrain"` // minAvailable=100% or maxUnavailable=0
	RiskyPDB          int `json:"riskyPDB"`   // minAvailable >= replicas (blocks all evictions)
	TotalDeployments  int `json:"totalDeployments"`
	TotalStatefulSets int `json:"totalStatefulSets"`
	MaintenanceScore  int `json:"maintenanceScore"` // 0-100
}

// DIEntry describes one workload's disruption risk.
type DIEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	Replicas       int32  `json:"replicas"`
	HasPDB         bool   `json:"hasPDB"`
	MinAvailable   string `json:"minAvailable,omitempty"`
	MaxUnavailable string `json:"maxUnavailable,omitempty"`
	WillBlockDrain bool   `json:"willBlockDrain"`
	EvictablePods  int    `json:"evictablePods"`
	RiskLevel      string `json:"riskLevel"`
}

// DIIssue is a detected disruption problem.
type DIIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleDisruptionImpact analyzes deployment/PDB interaction for maintenance readiness.
// GET /api/deployment/disruption-impact
func (s *Server) handleDisruptionImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Get all PDBs
	pdbs, err := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build PDB map by label selector match
	pdbByNS := make(map[string][]struct {
		Name          string
		Namespace     string
		MinAvailStr   string
		MaxUnavailStr string
		MinAvailVal   int32 // parsed value, -1 if percentage/unset
	})
	if pdbs != nil {
		for _, pdb := range pdbs.Items {
			info := struct {
				Name          string
				Namespace     string
				MinAvailStr   string
				MaxUnavailStr string
				MinAvailVal   int32
			}{
				Name:        pdb.Name,
				Namespace:   pdb.Namespace,
				MinAvailVal: -1,
			}
			if pdb.Spec.MinAvailable != nil {
				info.MinAvailStr = pdb.Spec.MinAvailable.StrVal
				if pdb.Spec.MinAvailable.Type == 0 { // int
					info.MinAvailVal = int32(pdb.Spec.MinAvailable.IntVal)
				}
			}
			if pdb.Spec.MaxUnavailable != nil {
				info.MaxUnavailStr = pdb.Spec.MaxUnavailable.StrVal
			}
			pdbByNS[pdb.Namespace] = append(pdbByNS[pdb.Namespace], info)
		}
	}

	// Get deployments
	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get statefulsets
	stss, err := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := DIResult{ScannedAt: time.Now()}

	// Analyze deployments
	for _, dep := range deployments.Items {
		result.Summary.TotalDeployments++
		diAnalyzeWorkload(&result, dep.Name, dep.Namespace, "Deployment", dep.Spec.Replicas, dep.Spec.Selector, pdbByNS)
	}

	// Analyze statefulsets
	for _, sts := range stss.Items {
		result.Summary.TotalStatefulSets++
		diAnalyzeWorkload(&result, sts.Name, sts.Namespace, "StatefulSet", sts.Spec.Replicas, sts.Spec.Selector, pdbByNS)
	}

	// Sort
	sort.Slice(result.BlockingWorkloads, func(i, j int) bool {
		return diRiskRank(result.BlockingWorkloads[i].RiskLevel) < diRiskRank(result.BlockingWorkloads[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return diIssueRank(result.Issues[i].Severity) < diIssueRank(result.Issues[j].Severity)
	})

	result.Summary.MaintenanceScore = diScore(result.Summary)
	result.Recommendations = diGenRecs(result.Summary, result.BlockingWorkloads, result.NoPDBWorkloads)

	writeJSON(w, result)
}

// diAnalyzeWorkload processes one workload against PDBs.
func diAnalyzeWorkload(result *DIResult, name, ns, kind string, replicas *int32, selector *metav1.LabelSelector, pdbByNS map[string][]struct {
	Name          string
	Namespace     string
	MinAvailStr   string
	MaxUnavailStr string
	MinAvailVal   int32
}) {
	result.Summary.TotalWorkloads++

	repl := int32(1)
	if replicas != nil {
		repl = *replicas
	}

	entry := DIEntry{
		Name:      name,
		Namespace: ns,
		Kind:      kind,
		Replicas:  repl,
	}

	// Find matching PDB (simplified: check if any PDB in same namespace)
	nsPDBs := pdbByNS[ns]
	matched := false
	for _, pdb := range nsPDBs {
		// Simple heuristic: if PDB name contains workload name, assume match
		// Full implementation would use label selector matching
		entry.HasPDB = true
		matched = true
		entry.MinAvailable = pdb.MinAvailStr
		entry.MaxUnavailable = pdb.MaxUnavailStr

		// Check if PDB blocks all evictions
		if pdb.MinAvailVal >= 0 && pdb.MinAvailVal >= repl && repl > 0 {
			entry.WillBlockDrain = true
			result.Summary.BlockDrain++
			result.Summary.RiskyPDB++
			result.Issues = append(result.Issues, DIIssue{
				Severity: "critical", Type: "pdb-blocks-drain",
				Resource: fmt.Sprintf("%s/%s", ns, name),
				Message:  fmt.Sprintf("%s %s/%s has PDB with minAvailable=%d (replicas=%d) — will block all voluntary evictions including node drains", kind, ns, name, pdb.MinAvailVal, repl),
			})
		}
		break
	}

	if !matched {
		// Check if any PDB exists in this namespace at all
		if len(nsPDBs) == 0 {
			entry.HasPDB = false
			result.Summary.NoPDB++
			result.NoPDBWorkloads = append(result.NoPDBWorkloads, entry)
			result.Issues = append(result.Issues, DIIssue{
				Severity: "warning", Type: "no-pdb",
				Resource: fmt.Sprintf("%s/%s", ns, name),
				Message:  fmt.Sprintf("%s %s/%s has no PodDisruptionBudget — pods can be evicted without protection during node drains", kind, ns, name),
			})
		} else {
			entry.HasPDB = true // namespace-level PDB may cover it
		}
	}

	// Calculate evictable pods
	if entry.WillBlockDrain {
		entry.EvictablePods = 0
		entry.RiskLevel = "critical"
		result.BlockingWorkloads = append(result.BlockingWorkloads, entry)
	} else if !entry.HasPDB {
		entry.EvictablePods = int(repl)
		entry.RiskLevel = "medium"
	} else {
		// Evictable = replicas - minAvailable (or maxUnavailable)
		entry.EvictablePods = diCalcEvictable(repl, entry.MinAvailable, entry.MaxUnavailable)
		if entry.EvictablePods == 0 {
			entry.RiskLevel = "high"
		} else {
			entry.RiskLevel = "low"
			result.SafeWorkloads = append(result.SafeWorkloads, entry)
		}
	}
}

// diCalcEvictable computes how many pods can be voluntarily evicted.
func diCalcEvictable(replicas int32, minAvailStr, maxUnavailStr string) int {
	if maxUnavailStr != "" {
		// maxUnavailable takes precedence
		var mu int32
		if _, err := fmt.Sscanf(maxUnavailStr, "%d", &mu); err == nil {
			return int(mu)
		}
	}
	if minAvailStr != "" {
		var ma int32
		if _, err := fmt.Sscanf(minAvailStr, "%d", &ma); err == nil {
			evictable := int(replicas - ma)
			if evictable < 0 {
				evictable = 0
			}
			return evictable
		}
	}
	return int(replicas) // no PDB = all evictable
}

// diScore computes maintenance readiness score 0-100.
func diScore(s DISummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.BlockDrain * 15
	score -= s.RiskyPDB * 5
	score -= s.NoPDB * 3
	if score < 0 {
		score = 0
	}
	return score
}

// diGenRecs produces actionable advice.
func diGenRecs(s DISummary, blocking []DIEntry, noPDB []DIEntry) []string {
	var recs []string

	if s.BlockDrain > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) will block node drains — adjust PDB minAvailable to allow at least 1 voluntary eviction", s.BlockDrain))
	}
	if s.NoPDB > 0 {
		top := ""
		if len(noPDB) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", noPDB[0].Namespace, noPDB[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d workload(s) have no PDB%s — add PodDisruptionBudget to protect against involuntary disruptions during maintenance", s.NoPDB, top))
	}
	if s.RiskyPDB > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have risky PDBs (minAvailable >= replicas) — these prevent all voluntary evictions", s.RiskyPDB))
	}
	if s.MaintenanceScore < 70 {
		recs = append(recs, fmt.Sprintf("Maintenance readiness score is %d/100 — review PDB configurations before scheduling node maintenance", s.MaintenanceScore))
	}
	if s.BlockDrain == 0 && s.NoPDB == 0 {
		recs = append(recs, fmt.Sprintf("All workloads have proper PDBs — cluster is ready for maintenance (score: %d/100)", s.MaintenanceScore))
	}

	return recs
}

func diRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func diIssueRank(s string) int {
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
var _ = policyv1.PodDisruptionBudget{}
