package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HAResult is the HA & single-point-of-failure analysis.
type HAResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	Summary         HASummary   `json:"summary"`
	SingleReplicas  []HAEntry   `json:"singleReplicas"` // replicas=1
	NoPDB           []HAEntry   `json:"noPDB"`          // multi-replica, no PDB
	NoAntiAffinity  []HAEntry   `json:"noAntiAffinity"` // multi-replica, no anti-affinity
	SingleNodePods  []HAEntry   `json:"SingleNodePods"` // all pods on one node
	NoReadiness     []HAEntry   `json:"noReadiness"`    // no readiness probe = slow failover
	AllEntries      []HAEntry   `json:"allEntries"`
	ByNamespace     []HANSEntry `json:"byNamespace"`
	Issues          []HAIssue   `json:"issues"`
	Recommendations []string    `json:"recommendations"`
}

// HASummary aggregates HA statistics.
type HASummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	SingleReplicas   int `json:"singleReplicas"` // replicas=1
	MultiReplica     int `json:"multiReplica"`
	HasPDB           int `json:"hasPDB"`
	NoPDB            int `json:"noPDB"` // multi-replica without PDB
	HasAntiAffinity  int `json:"hasAntiAffinity"`
	NoAntiAffinity   int `json:"noAntiAffinity"`   // multi-replica without anti-affinity
	SingleNodeSpread int `json:"singleNodeSpread"` // all pods on same node
	HasReadiness     int `json:"hasReadiness"`
	NoReadiness      int `json:"noReadiness"`
	HAScore          int `json:"haScore"` // 0-100
}

// HAEntry describes one workload's HA posture.
type HAEntry struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	Replicas          int32    `json:"replicas"`
	ReadyReplicas     int32    `json:"readyReplicas"`
	HasPDB            bool     `json:"hasPDB"`
	HasAntiAffinity   bool     `json:"hasAntiAffinity"`
	HasTopologySpread bool     `json:"hasTopologySpread"`
	NodeSpread        int      `json:"nodeSpread"` // unique nodes hosting pods
	HasReadiness      bool     `json:"hasReadiness"`
	SPOFRisks         []string `json:"spofRisks,omitempty"`
	RiskLevel         string   `json:"riskLevel"`
}

// HANSEntry per-namespace HA stats.
type HANSEntry struct {
	Namespace      string `json:"namespace"`
	WorkloadCount  int    `json:"workloadCount"`
	SingleReplicas int    `json:"singleReplicas"`
	NoPDB          int    `json:"noPDB"`
	NoAntiAffinity int    `json:"noAntiAffinity"`
	HAScore        int    `json:"haScore"`
}

// HAIssue is a detected HA problem.
type HAIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleHASPOFDetector detects single points of failure across workloads.
// GET /api/scalability/ha-audit
func (s *Server) handleHASPOFDetector(w http.ResponseWriter, r *http.Request) {
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

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build PDB target map
	var pdbs []policyv1.PodDisruptionBudget
	if rc.clientset.PolicyV1() != nil {
		pdbList, err := rc.clientset.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			pdbs = pdbList.Items
		}
	}

	// Build workload → has PDB map
	pdbTargets := make(map[string]bool) // namespace/name or namespace/selector
	for _, pdb := range pdbs {
		if pdb.Spec.Selector != nil && len(pdb.Spec.Selector.MatchLabels) > 0 {
			key := fmt.Sprintf("%s/selector", pdb.Namespace)
			pdbTargets[key] = true
		}
		key := fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name)
		pdbTargets[key] = true
	}

	// Check if namespace has any PDB
	nsHasPDB := make(map[string]bool)
	for _, pdb := range pdbs {
		nsHasPDB[pdb.Namespace] = true
	}

	// Build pod → node map per workload
	type workloadPods struct {
		pods  []corev1.Pod
		nodes map[string]int
	}
	wpMap := make(map[string]*workloadPods)

	for _, pod := range pods.Items {
		// Derive owner workload name from labels
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" {
				// Strip hash suffix
				wlName = ref.Name
			}
		}
		if wlName == "" {
			for _, ref := range pod.OwnerReferences {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}

		key := fmt.Sprintf("%s/%s", pod.Namespace, wlName)
		wp, ok := wpMap[key]
		if !ok {
			wp = &workloadPods{nodes: make(map[string]int)}
			wpMap[key] = wp
		}
		wp.pods = append(wp.pods, pod)
		if pod.Spec.NodeName != "" {
			wp.nodes[pod.Spec.NodeName]++
		}
	}

	result := HAResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*HANSEntry)

	for _, dep := range deployments.Items {
		result.Summary.TotalWorkloads++

		replicas := int32(1) // default
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		entry := HAEntry{
			Name:          dep.Name,
			Namespace:     dep.Namespace,
			Kind:          "Deployment",
			Replicas:      replicas,
			ReadyReplicas: dep.Status.ReadyReplicas,
		}

		// Check PDB (namespace has any PDB = approximated)
		entry.HasPDB = nsHasPDB[dep.Namespace]

		// Check anti-affinity
		affinity := dep.Spec.Template.Spec.Affinity
		if affinity != nil && affinity.PodAntiAffinity != nil {
			entry.HasAntiAffinity = true
			result.Summary.HasAntiAffinity++
		} else {
			entry.HasAntiAffinity = false
		}

		// Check topology spread constraints
		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			entry.HasTopologySpread = true
		}

		// Check readiness probe
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
				break
			}
		}

		// Node spread from pods
		wpKey := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		if wp, ok := wpMap[wpKey]; ok {
			entry.NodeSpread = len(wp.nodes)
			if replicas > 1 && entry.NodeSpread <= 1 {
				result.Summary.SingleNodeSpread++
				entry.SPOFRisks = append(entry.SPOFRisks, fmt.Sprintf("All %d pods on single node — node failure kills workload", replicas))
				result.SingleNodePods = append(result.SingleNodePods, entry)
			}
		}

		// SPOF risk assessment
		if replicas <= 1 {
			result.Summary.SingleReplicas++
			entry.SPOFRisks = append(entry.SPOFRisks, "Single replica — no redundancy, any restart causes downtime")
			result.SingleReplicas = append(result.SingleReplicas, entry)
			result.Issues = append(result.Issues, HAIssue{
				Severity: "critical", Type: "single-replica",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has 1 replica — single point of failure", dep.Namespace, dep.Name),
			})
		} else {
			result.Summary.MultiReplica++
			if !entry.HasPDB {
				result.Summary.NoPDB++
				entry.SPOFRisks = append(entry.SPOFRisks, "No PDB — voluntary disruptions can kill all pods simultaneously")
				result.NoPDB = append(result.NoPDB, entry)
				result.Issues = append(result.Issues, HAIssue{
					Severity: "warning", Type: "no-pdb",
					Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
					Message:  fmt.Sprintf("Deployment %s/%s has %d replicas but no PDB — disruptions can kill all pods", dep.Namespace, dep.Name, replicas),
				})
			} else {
				result.Summary.HasPDB++
			}
			if !entry.HasAntiAffinity {
				result.Summary.NoAntiAffinity++
				entry.SPOFRisks = append(entry.SPOFRisks, "No pod anti-affinity — scheduler may place all pods on one node")
				result.NoAntiAffinity = append(result.NoAntiAffinity, entry)
			}
		}

		if !entry.HasReadiness {
			result.Summary.NoReadiness++
			entry.SPOFRisks = append(entry.SPOFRisks, "No readiness probe — failover is slow, traffic hits unhealthy pods")
			result.NoReadiness = append(result.NoReadiness, entry)
		} else {
			result.Summary.HasReadiness++
		}

		entry.RiskLevel = haAssessRisk(entry)
		result.AllEntries = append(result.AllEntries, entry)

		// Namespace tracking
		nsStat := haGetOrCreateNS(nsMap, dep.Namespace)
		nsStat.WorkloadCount++
		if replicas <= 1 {
			nsStat.SingleReplicas++
		}
		if replicas > 1 && !entry.HasPDB {
			nsStat.NoPDB++
		}
		if replicas > 1 && !entry.HasAntiAffinity {
			nsStat.NoAntiAffinity++
		}
	}

	// Finalize namespace scores
	for _, nsStat := range nsMap {
		nsStat.HAScore = haNSScore(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.AllEntries, func(i, j int) bool {
		return haRiskRank(result.AllEntries[i].RiskLevel) < haRiskRank(result.AllEntries[j].RiskLevel)
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].SingleReplicas > result.ByNamespace[j].SingleReplicas
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return haIssueRank(result.Issues[i].Severity) < haIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HAScore = haScore(result.Summary)
	result.Recommendations = haGenRecs(result.Summary, result.SingleReplicas, result.NoPDB)

	writeJSON(w, result)
}

// haAssessRisk determines risk level.
func haAssessRisk(entry HAEntry) string {
	if entry.Replicas <= 1 {
		return "critical"
	}
	if entry.NodeSpread <= 1 && entry.Replicas > 1 {
		return "critical"
	}
	if !entry.HasPDB {
		return "high"
	}
	if !entry.HasAntiAffinity && !entry.HasTopologySpread {
		return "medium"
	}
	if !entry.HasReadiness {
		return "medium"
	}
	return "low"
}

// haScore computes 0-100.
func haScore(s HASummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.SingleReplicas * 15
	score -= s.SingleNodeSpread * 12
	score -= s.NoPDB * 6
	score -= s.NoAntiAffinity * 3
	score -= s.NoReadiness * 4
	if score < 0 {
		score = 0
	}
	return score
}

// haNSScore computes namespace HA score.
func haNSScore(ns HANSEntry) int {
	if ns.WorkloadCount == 0 {
		return 100
	}
	score := 100
	score -= ns.SingleReplicas * 15
	score -= ns.NoPDB * 6
	score -= ns.NoAntiAffinity * 3
	if score < 0 {
		score = 0
	}
	return score
}

// haGenRecs produces actionable advice.
func haGenRecs(s HASummary, singleReplicas []HAEntry, noPDB []HAEntry) []string {
	var recs []string

	if s.SingleReplicas > 0 {
		top := ""
		if len(singleReplicas) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", singleReplicas[0].Namespace, singleReplicas[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d workload(s) have single replica%s — scale to 2+ for zero-downtime deployments", s.SingleReplicas, top))
	}
	if s.NoPDB > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) lack PDB — add PodDisruptionBudget to protect during voluntary disruptions", s.NoPDB))
	}
	if s.NoAntiAffinity > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) lack pod anti-affinity — pods may co-locate on one node", s.NoAntiAffinity))
	}
	if s.SingleNodeSpread > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have all pods on a single node — add podAntiAffinity or topologySpreadConstraints", s.SingleNodeSpread))
	}
	if s.NoReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) lack readiness probes — failover is slow, add readinessProbe for traffic gating", s.NoReadiness))
	}
	if s.HAScore < 60 {
		recs = append(recs, fmt.Sprintf("HA score is %d/100 — multiple single points of failure detected", s.HAScore))
	}
	if s.SingleReplicas == 0 && s.NoPDB == 0 && s.SingleNodeSpread == 0 {
		recs = append(recs, "No single points of failure detected — good HA posture")
	}

	return recs
}

func haGetOrCreateNS(m map[string]*HANSEntry, ns string) *HANSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &HANSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func haRiskRank(level string) int {
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

func haIssueRank(s string) int {
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

var _ = appsv1.DeploymentSpec{}
