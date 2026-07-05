package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// PDBAuditResult is the PDB compliance & voluntary disruption risk analysis.
type PDBAuditResult struct {
	ScannedAt          time.Time          `json:"scannedAt"`
	Summary            PDBAuditSummary    `json:"summary"`
	ProtectedWorkloads []PDBEntry         `json:"protectedWorkloads"`
	Unprotected        []UnprotectedEntry `json:"unprotected"`
	Blockers           []PDBBlocker       `json:"blockers"` // PDBs that would block node drain
	DrainSimulation    []DrainImpact      `json:"drainSimulation"`
	Issues             []PDBIssue         `json:"issues"`
	Recommendations    []string           `json:"recommendations"`
}

// PDBAuditSummary aggregates PDB compliance.
type PDBAuditSummary struct {
	TotalDeployments        int `json:"totalDeployments"`
	TotalPDBs               int `json:"totalPDBs"`
	ProtectedCount          int `json:"protectedCount"`   // deployments with matching PDB
	UnprotectedCount        int `json:"unprotectedCount"` // multi-replica deployments without PDB
	BlockedCount            int `json:"blockedCount"`     // PDBs currently blocking disruptions
	ImpossibleCount         int `json:"impossibleCount"`  // PDBs that can never be satisfied
	TotalAllowedDisruptions int `json:"totalAllowedDisruptions"`
	HealthScore             int `json:"healthScore"`
}

// PDBEntry describes one PDB and its target workload.
type PDBEntry struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	MinAvailable       string `json:"minAvailable"`
	MaxUnavailable     string `json:"maxUnavailable"`
	Selector           string `json:"selector"`
	TargetKind         string `json:"targetKind"`
	TargetName         string `json:"targetName"`
	CurrentReplicas    int32  `json:"currentReplicas"`
	DesiredReplicas    int32  `json:"desiredReplicas"`
	AllowedDisruptions int32  `json:"allowedDisruptions"`
	UnhealthyReplicas  int32  `json:"unhealthyReplicas"`
	Status             string `json:"status"` // healthy / blocked / impossible
	RiskLevel          string `json:"riskLevel"`
}

// UnprotectedEntry describes a multi-replica deployment without a PDB.
type UnprotectedEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	RiskLevel string `json:"riskLevel"`
}

// PDBBlocker describes a PDB that would block voluntary disruption.
type PDBBlocker struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	TargetName         string `json:"targetName"`
	CurrentReplicas    int32  `json:"currentReplicas"`
	AllowedDisruptions int32  `json:"allowedDisruptions"`
	Reason             string `json:"reason"`
}

// DrainImpact simulates draining a node and its effect on workloads.
type DrainImpact struct {
	NodeName      string   `json:"nodeName"`
	AffectedPods  int      `json:"affectedPods"`
	BlockedByPDBs []string `json:"blockedByPDBs"`
	CanDrain      bool     `json:"canDrain"`
}

// PDBIssue is a detected PDB problem.
type PDBIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handlePDBAudit audits PDB compliance and voluntary disruption risk.
// GET /api/operations/pdb-audit
func (s *Server) handlePDBAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pdbs, err := rc.clientset.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := PDBAuditResult{ScannedAt: time.Now()}
	result.Summary.TotalDeployments = len(deployments.Items)

	// Build PDB → deployment matching
	depByNS := make(map[string][]appsv1.Deployment)
	for _, dep := range deployments.Items {
		depByNS[dep.Namespace] = append(depByNS[dep.Namespace], dep)
	}

	matchedDeps := make(map[string]bool) // ns/dep → has PDB

	for _, pdb := range pdbs.Items {
		entry := PDBEntry{
			Name:      pdb.Name,
			Namespace: pdb.Namespace,
		}

		// MinAvailable / MaxUnavailable
		if pdb.Spec.MinAvailable != nil {
			entry.MinAvailable = intStrToString(pdb.Spec.MinAvailable)
		}
		if pdb.Spec.MaxUnavailable != nil {
			entry.MaxUnavailable = intStrToString(pdb.Spec.MaxUnavailable)
		}

		// Selector
		entry.Selector = pdb.Spec.Selector.String()

		// Status
		entry.CurrentReplicas = int32(pdb.Status.CurrentHealthy)
		entry.DesiredReplicas = int32(pdb.Status.DesiredHealthy)
		entry.AllowedDisruptions = pdb.Status.DisruptionsAllowed
		entry.UnhealthyReplicas = int32(pdb.Status.CurrentHealthy) - int32(pdb.Status.ExpectedPods)

		// Match to deployment
		sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err == nil {
			for _, dep := range depByNS[pdb.Namespace] {
				if sel.Matches(labels.Set(dep.Spec.Selector.MatchLabels)) {
					entry.TargetKind = "Deployment"
					entry.TargetName = dep.Name
					matchedDeps[fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)] = true

					if entry.CurrentReplicas == 0 {
						entry.CurrentReplicas = *dep.Spec.Replicas
					}
					break
				}
			}
		}

		// Classify status
		entry.Status, entry.RiskLevel = classifyPDB(entry)
		result.Summary.TotalPDBs++
		result.Summary.TotalAllowedDisruptions += int(entry.AllowedDisruptions)

		switch entry.Status {
		case "blocked":
			result.Summary.BlockedCount++
			result.Blockers = append(result.Blockers, PDBBlocker{
				Name:               pdb.Name,
				Namespace:          pdb.Namespace,
				TargetName:         entry.TargetName,
				CurrentReplicas:    entry.CurrentReplicas,
				AllowedDisruptions: entry.AllowedDisruptions,
				Reason:             fmt.Sprintf("PDB allows %d disruptions — node drain will be blocked", entry.AllowedDisruptions),
			})
			result.Issues = append(result.Issues, PDBIssue{
				Severity: "warning", Type: "pdb-blocked",
				Resource: fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name),
				Message:  fmt.Sprintf("PDB %s/%s blocks disruption (allowed: %d) — drain operations will stall", pdb.Namespace, pdb.Name, entry.AllowedDisruptions),
			})
		case "impossible":
			result.Summary.ImpossibleCount++
			result.Issues = append(result.Issues, PDBIssue{
				Severity: "critical", Type: "pdb-impossible",
				Resource: fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name),
				Message:  fmt.Sprintf("PDB %s/%s is impossible to satisfy — minAvailable > current pods", pdb.Namespace, pdb.Name),
			})
		}

		result.ProtectedWorkloads = append(result.ProtectedWorkloads, entry)
	}

	// Find unprotected multi-replica deployments
	for _, dep := range deployments.Items {
		if dep.Spec.Replicas == nil || *dep.Spec.Replicas < 2 {
			continue
		}
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		if matchedDeps[key] {
			result.Summary.ProtectedCount++
			continue
		}
		result.Summary.UnprotectedCount++
		result.Unprotected = append(result.Unprotected, UnprotectedEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Replicas:  *dep.Spec.Replicas,
			RiskLevel: classifyUnprotectedRisk(*dep.Spec.Replicas),
		})
		if *dep.Spec.Replicas >= 3 {
			result.Issues = append(result.Issues, PDBIssue{
				Severity: "warning", Type: "no-pdb",
				Resource: key,
				Message:  fmt.Sprintf("Deployment %s has %d replicas but no PDB — voluntary disruption risk", key, *dep.Spec.Replicas),
			})
		} else {
			result.Issues = append(result.Issues, PDBIssue{
				Severity: "info", Type: "no-pdb",
				Resource: key,
				Message:  fmt.Sprintf("Deployment %s has %d replicas but no PDB — consider adding one", key, *dep.Spec.Replicas),
			})
		}
	}

	// Drain simulation per node
	podNodeMap := make(map[string][]string) // node → []pod ns/name
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == "Running" {
			podNodeMap[pod.Spec.NodeName] = append(podNodeMap[pod.Spec.NodeName], fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
		}
	}

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		affected := len(podNodeMap[node.Name])
		if affected == 0 {
			continue
		}

		impact := DrainImpact{
			NodeName:     node.Name,
			AffectedPods: affected,
			CanDrain:     true,
		}

		// Check if any PDB would block draining pods on this node
		for _, pdb := range pdbs.Items {
			if pdb.Status.DisruptionsAllowed <= 0 {
				// Check if any pod on this node matches this PDB's selector
				sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
				if err != nil {
					continue
				}
				for _, pod := range pods.Items {
					if pod.Spec.NodeName != node.Name {
						continue
					}
					if sel.Matches(labels.Set(pod.Labels)) {
						impact.BlockedByPDBs = append(impact.BlockedByPDBs, fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name))
						impact.CanDrain = false
						break
					}
				}
			}
		}

		result.DrainSimulation = append(result.DrainSimulation, impact)
	}

	// Sort
	sort.Slice(result.ProtectedWorkloads, func(i, j int) bool {
		return pdbRiskRank(result.ProtectedWorkloads[i].RiskLevel) < pdbRiskRank(result.ProtectedWorkloads[j].RiskLevel)
	})
	sort.Slice(result.Unprotected, func(i, j int) bool {
		return result.Unprotected[i].Replicas > result.Unprotected[j].Replicas
	})
	sort.Slice(result.DrainSimulation, func(i, j int) bool {
		return len(result.DrainSimulation[i].BlockedByPDBs) > len(result.DrainSimulation[j].BlockedByPDBs)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return pdbIssueRank(result.Issues[i].Severity) < pdbIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = calculatePDBScore(result.Summary)
	result.Recommendations = generatePDBRecs(result.Summary, result.Unprotected, result.Blockers)

	writeJSON(w, result)
}

// classifyPDB determines PDB status and risk.
func classifyPDB(entry PDBEntry) (status, risk string) {
	if entry.CurrentReplicas > 0 && entry.AllowedDisruptions < 0 {
		return "impossible", "critical"
	}
	if entry.AllowedDisruptions <= 0 && entry.CurrentReplicas > 0 {
		return "blocked", "high"
	}
	if entry.UnhealthyReplicas > 0 {
		return "healthy", "medium"
	}
	return "healthy", "low"
}

// classifyUnprotectedRisk determines risk for deployments without PDB.
func classifyUnprotectedRisk(replicas int32) string {
	switch {
	case replicas >= 5:
		return "high"
	case replicas >= 3:
		return "medium"
	default:
		return "low"
	}
}

// calculatePDBScore computes 0-100.
func calculatePDBScore(s PDBAuditSummary) int {
	totalWorkloads := s.ProtectedCount + s.UnprotectedCount
	if totalWorkloads == 0 {
		return 100
	}
	coverage := float64(s.ProtectedCount) / float64(totalWorkloads) * 100
	score := int(coverage)
	score -= s.BlockedCount * 10
	score -= s.ImpossibleCount * 20
	if score < 0 {
		score = 0
	}
	return score
}

// generatePDBRecs produces actionable advice.
func generatePDBRecs(s PDBAuditSummary, unprotected []UnprotectedEntry, blockers []PDBBlocker) []string {
	var recs []string

	if s.UnprotectedCount > 0 {
		highRisk := 0
		for _, u := range unprotected {
			if u.RiskLevel == "high" {
				highRisk++
			}
		}
		if highRisk > 0 {
			recs = append(recs, fmt.Sprintf("%d high-risk deployment(s) with >=5 replicas have NO PDB — add PDB to protect against voluntary disruptions", highRisk))
		}
		recs = append(recs, fmt.Sprintf("%d of %d multi-replica deployment(s) lack PDB coverage (%.0f%%)", s.UnprotectedCount, s.ProtectedCount+s.UnprotectedCount, float64(s.UnprotectedCount)/float64(s.ProtectedCount+s.UnprotectedCount)*100))
	}
	if s.ImpossibleCount > 0 {
		recs = append(recs, fmt.Sprintf("%d PDB(s) are impossible to satisfy (minAvailable > pods) — fix PDB spec or scale up workload", s.ImpossibleCount))
	}
	if s.BlockedCount > 0 {
		top := ""
		if len(blockers) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s blocks %s)", blockers[0].Namespace, blockers[0].Name, blockers[0].TargetName)
		}
		recs = append(recs, fmt.Sprintf("%d PDB(s) currently block voluntary disruptions%s — node drain will stall until pods are healthy", s.BlockedCount, top))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("PDB coverage health score is %d/100 — add PDBs for critical workloads", s.HealthScore))
	}
	if s.ProtectedCount > 0 && s.UnprotectedCount == 0 && s.BlockedCount == 0 {
		recs = append(recs, "All multi-replica deployments have PDB coverage — excellent voluntary disruption protection")
	}

	return recs
}

// intStrToString is defined in handlers_pdb.go

func pdbRiskRank(level string) int {
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

func pdbIssueRank(s string) int {
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

// Ensure imports used
var _ = strings.Contains
