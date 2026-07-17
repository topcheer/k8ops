package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReliabilityScorecardResult is the per-service reliability posture scorecard.
// It aggregates multiple reliability signals into a single A-F grade per workload.
type ReliabilityScorecardResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ScorecardSummary    `json:"summary"`
	Workloads       []WorkloadScorecard `json:"workloads"`
	ClusterGrade    string              `json:"clusterGrade"` // A-F
	ClusterScore    int                 `json:"clusterScore"` // 0-100
	WeakestSignals  []WeakSignal        `json:"weakestSignals"`
	Distribution    GradeDistribution   `json:"distribution"`
	Recommendations []string            `json:"recommendations"`
}

// ScorecardSummary aggregates scoring statistics.
type ScorecardSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	GradeA            int `json:"gradeA"`            // >=90
	GradeB            int `json:"gradeB"`            // 80-89
	GradeC            int `json:"gradeC"`            // 70-79
	GradeD            int `json:"gradeD"`            // 60-69
	GradeF            int `json:"gradeF"`            // <60
	AtRiskWorkloads   int `json:"atRiskWorkloads"`   // grade D or F
	CriticalWorkloads int `json:"criticalWorkloads"` // grade F
}

// WorkloadScorecard scores one workload across multiple reliability dimensions.
type WorkloadScorecard struct {
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	Kind       string           `json:"kind"`  // Deployment, StatefulSet, DaemonSet
	Grade      string           `json:"grade"` // A, B, C, D, F
	Score      int              `json:"score"` // 0-100
	Replicas   int              `json:"replicas"`
	Dimensions []ScoreDimension `json:"dimensions"`
	Risks      []string         `json:"risks,omitempty"`
	TopFix     string           `json:"topFix"` // highest-impact improvement
}

// ScoreDimension scores one reliability dimension.
type ScoreDimension struct {
	Name        string `json:"name"`   // replication, probes, resources, pdb, security, limits, strategy
	Score       int    `json:"score"`  // 0-100 for this dimension
	Status      string `json:"status"` // good, warning, critical
	Description string `json:"description"`
}

// WeakSignal describes a cluster-wide reliability weakness.
type WeakSignal struct {
	Dimension string `json:"dimension"`
	Count     int    `json:"count"` // how many workloads fail this
	Impact    string `json:"impact"`
}

// GradeDistribution shows how many workloads fall into each grade.
type GradeDistribution struct {
	A int `json:"a"`
	B int `json:"b"`
	C int `json:"c"`
	D int `json:"d"`
	F int `json:"f"`
}

// handleReliabilityScorecard generates a per-workload reliability scorecard.
// GET /api/product/reliability-scorecard
func (s *Server) handleReliabilityScorecard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ReliabilityScorecardResult{ScannedAt: time.Now()}

	// Collect resources
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulSets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonSets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build PDB selector map
	pdbSelectors := buildPDBSelectors(pdbs)

	// Dimension counters for weak signal analysis
	dimFails := map[string]int{}

	// Score each workload
	var scorecards []WorkloadScorecard

	scoreDeployments := func(d appsv1.Deployment) {
		sc := scoreWorkload(d.Name, d.Namespace, "Deployment",
			d.Spec.Replicas, d.Status.AvailableReplicas,
			d.Spec.Template.Spec, d.Spec.Selector, pdbSelectors, d.Spec.Strategy)
		scorecards = append(scorecards, sc)
		for _, dim := range sc.Dimensions {
			if dim.Status == "critical" {
				dimFails[dim.Name]++
			}
		}
	}
	scoreStatefulSet := func(ss appsv1.StatefulSet) {
		sc := scoreWorkload(ss.Name, ss.Namespace, "StatefulSet",
			ss.Spec.Replicas, ss.Status.AvailableReplicas,
			ss.Spec.Template.Spec, ss.Spec.Selector, pdbSelectors, ss.Spec.UpdateStrategy)
		scorecards = append(scorecards, sc)
		for _, dim := range sc.Dimensions {
			if dim.Status == "critical" {
				dimFails[dim.Name]++
			}
		}
	}
	scoreDaemonSet := func(ds appsv1.DaemonSet) {
		// DaemonSets run on all nodes - replication dimension is different
		replicas := ds.Status.DesiredNumberScheduled
		available := ds.Status.NumberAvailable
		sc := scoreWorkload(ds.Name, ds.Namespace, "DaemonSet",
			&replicas, available,
			ds.Spec.Template.Spec, ds.Spec.Selector, pdbSelectors, ds.Spec.UpdateStrategy)
		scorecards = append(scorecards, sc)
		for _, dim := range sc.Dimensions {
			if dim.Status == "critical" {
				dimFails[dim.Name]++
			}
		}
	}

	if deployments != nil {
		for _, d := range deployments.Items {
			// Skip system namespaces for clarity
			if isSystemNSReliability(d.Namespace) {
				continue
			}
			scoreDeployments(d)
		}
	}
	if statefulSets != nil {
		for _, ss := range statefulSets.Items {
			if isSystemNSReliability(ss.Namespace) {
				continue
			}
			scoreStatefulSet(ss)
		}
	}
	if daemonSets != nil {
		for _, ds := range daemonSets.Items {
			if isSystemNSReliability(ds.Namespace) {
				continue
			}
			scoreDaemonSet(ds)
		}
	}

	// Sort by score ascending (worst first)
	sort.Slice(scorecards, func(i, j int) bool {
		return scorecards[i].Score < scorecards[j].Score
	})

	// Limit to top 50 worst
	if len(scorecards) > 50 {
		scorecards = scorecards[:50]
	}

	result.Workloads = scorecards

	// Compute summary
	result.Summary.TotalWorkloads = len(scorecards)
	for _, sc := range scorecards {
		switch sc.Grade {
		case "A":
			result.Summary.GradeA++
		case "B":
			result.Summary.GradeB++
		case "C":
			result.Summary.GradeC++
		case "D":
			result.Summary.GradeD++
			result.Summary.AtRiskWorkloads++
		case "F":
			result.Summary.GradeF++
			result.Summary.AtRiskWorkloads++
			result.Summary.CriticalWorkloads++
		}
	}
	result.Distribution = GradeDistribution{
		A: result.Summary.GradeA,
		B: result.Summary.GradeB,
		C: result.Summary.GradeC,
		D: result.Summary.GradeD,
		F: result.Summary.GradeF,
	}

	// Cluster score (average of all workloads)
	totalScore := 0
	for _, sc := range scorecards {
		totalScore += sc.Score
	}
	if len(scorecards) > 0 {
		result.ClusterScore = totalScore / len(scorecards)
	} else {
		result.ClusterScore = 100
	}
	result.ClusterGrade = scoreToGradeReliability(result.ClusterScore)

	// Weakest signals
	for dim, count := range dimFails {
		if count > 0 {
			result.WeakestSignals = append(result.WeakestSignals, WeakSignal{
				Dimension: dim,
				Count:     count,
				Impact:    dimensionImpact(dim),
			})
		}
	}
	sort.Slice(result.WeakestSignals, func(i, j int) bool {
		return result.WeakestSignals[i].Count > result.WeakestSignals[j].Count
	})

	// Recommendations
	result.Recommendations = generateScorecardRecs(result)

	writeJSON(w, result)
}

// scoreWorkload evaluates a workload across 7 reliability dimensions.
func scoreWorkload(name, namespace, kind string, replicas *int32, available int32,
	spec corev1.PodSpec, selector *metav1.LabelSelector,
	pdbSelectors []map[string]string, strategy interface{}) WorkloadScorecard {

	sc := WorkloadScorecard{Name: name, Namespace: namespace, Kind: kind}
	var dims []ScoreDimension
	var risks []string

	// ========================================
	// DIMENSION 1: Replication (HA) — replicas > 1
	// ========================================
	repScore := 100
	replicaCount := 0
	if replicas != nil {
		replicaCount = int(*replicas)
	}
	sc.Replicas = replicaCount

	if replicaCount < 2 && kind != "DaemonSet" {
		repScore = 20
		risks = append(risks, "single replica — no high availability")
	} else if replicaCount < 3 && kind != "DaemonSet" {
		repScore = 60
		risks = append(risks, "only 2 replicas — cannot tolerate 1 failure during rolling update")
	}

	// Check available vs desired
	if replicas != nil && available < *replicas {
		repScore -= 30
		risks = append(risks, fmt.Sprintf("only %d/%d replicas available", available, *replicas))
	}

	dims = append(dims, ScoreDimension{
		Name:        "replication",
		Score:       repScore,
		Status:      dimStatus(repScore),
		Description: fmt.Sprintf("%d replica(s), %d available", replicaCount, available),
	})

	// ========================================
	// DIMENSION 2: Health Probes — readiness + liveness configured
	// ========================================
	probeScore := 100
	hasReadiness := false
	hasLiveness := false
	hasStartup := false
	totalContainers := len(spec.Containers)

	for _, c := range spec.Containers {
		if c.ReadinessProbe != nil {
			hasReadiness = true
		} else {
			probeScore -= 20
		}
		if c.LivenessProbe != nil {
			hasLiveness = true
		} else {
			probeScore -= 10
		}
		if c.StartupProbe != nil {
			hasStartup = true
		}
	}

	if !hasReadiness {
		risks = append(risks, "missing readiness probe — traffic may route to unready pods")
	}
	if !hasLiveness && totalContainers > 0 {
		risks = append(risks, "missing liveness probe — dead containers won't restart automatically")
	}
	if !hasStartup && totalContainers > 0 {
		probeScore -= 5 // minor deduction
	}

	dims = append(dims, ScoreDimension{
		Name:        "probes",
		Score:       probeScore,
		Status:      dimStatus(probeScore),
		Description: fmt.Sprintf("readiness=%v liveness=%v startup=%v", hasReadiness, hasLiveness, hasStartup),
	})

	// ========================================
	// DIMENSION 3: Resource Management — requests and limits set
	// ========================================
	resScore := 100
	hasCPUReq := false
	hasMemReq := false
	hasCPULim := false
	hasMemLim := false

	for _, c := range spec.Containers {
		if c.Resources.Requests.Cpu() != nil && !c.Resources.Requests.Cpu().IsZero() {
			hasCPUReq = true
		} else {
			resScore -= 15
		}
		if c.Resources.Requests.Memory() != nil && !c.Resources.Requests.Memory().IsZero() {
			hasMemReq = true
		} else {
			resScore -= 15
		}
		if c.Resources.Limits.Cpu() != nil && !c.Resources.Limits.Cpu().IsZero() {
			hasCPULim = true
		}
		if c.Resources.Limits.Memory() != nil && !c.Resources.Limits.Memory().IsZero() {
			hasMemLim = true
		} else {
			resScore -= 10
		}
	}

	if !hasCPUReq || !hasMemReq {
		risks = append(risks, "missing resource requests — scheduler cannot place optimally")
	}
	if !hasMemLim {
		risks = append(risks, "missing memory limit — at risk for OOMKill of node")
	}

	dims = append(dims, ScoreDimension{
		Name:        "resources",
		Score:       resScore,
		Status:      dimStatus(resScore),
		Description: fmt.Sprintf("req: cpu=%v mem=%v, lim: cpu=%v mem=%v", hasCPUReq, hasMemReq, hasCPULim, hasMemLim),
	})

	// ========================================
	// DIMENSION 4: PDB Coverage — disruption protection
	// ========================================
	pdbScore := 0
	hasPDB := false
	if selector != nil {
		for _, pdbSel := range pdbSelectors {
			if labelsMatchSelector(selector.MatchLabels, pdbSel) {
				hasPDB = true
				break
			}
		}
	}
	if hasPDB {
		pdbScore = 100
	} else {
		pdbScore = 30
		if replicaCount > 1 {
			risks = append(risks, "no PDB — rolling updates may cause unexpected downtime")
		}
	}

	dims = append(dims, ScoreDimension{
		Name:        "pdb",
		Score:       pdbScore,
		Status:      dimStatus(pdbScore),
		Description: fmt.Sprintf("PDB coverage: %v", hasPDB),
	})

	// ========================================
	// DIMENSION 5: Security Context — non-privileged, non-root
	// ========================================
	secScore := 100
	hasNonRoot := false
	hasReadOnlyFS := false
	hasPrivileged := false

	for _, c := range spec.Containers {
		if c.SecurityContext != nil {
			if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				hasPrivileged = true
				secScore -= 40
			}
			if c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
				hasNonRoot = true
			} else {
				secScore -= 10
			}
			if c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				hasReadOnlyFS = true
			} else {
				secScore -= 5
			}
		} else {
			secScore -= 15
		}
	}

	if hasPrivileged {
		risks = append(risks, "privileged container — full node access")
	}

	dims = append(dims, ScoreDimension{
		Name:        "security",
		Score:       secScore,
		Status:      dimStatus(secScore),
		Description: fmt.Sprintf("nonRoot=%v readOnlyFS=%v privileged=%v", hasNonRoot, hasReadOnlyFS, hasPrivileged),
	})

	// ========================================
	// DIMENSION 6: Update Strategy — rolling update configured
	// ========================================
	strategyScore := 100
	switch s := strategy.(type) {
	case appsv1.DeploymentStrategy:
		if s.Type == appsv1.RecreateDeploymentStrategyType {
			strategyScore = 40
			risks = append(risks, "Recreate strategy — causes downtime during updates")
		}
	case appsv1.StatefulSetUpdateStrategy:
		if s.Type == appsv1.OnDeleteStatefulSetStrategyType {
			strategyScore = 50
			risks = append(risks, "OnDelete strategy — manual pod deletion required for updates")
		}
	}

	// Check termination grace period
	grace := spec.TerminationGracePeriodSeconds
	if grace != nil && *grace < 5 {
		strategyScore -= 15
		risks = append(risks, "very short termination grace period — may not drain cleanly")
	}

	dims = append(dims, ScoreDimension{
		Name:   "strategy",
		Score:  strategyScore,
		Status: dimStatus(strategyScore),
		Description: fmt.Sprintf("grace=%vs", func() int64 {
			if grace != nil {
				return *grace
			}
			return 30
		}()),
	})

	// ========================================
	// DIMENSION 7: Anti-Affinity / Topology Spread — distributed placement
	// ========================================
	affScore := 100
	hasAntiAffinity := false
	hasTopologySpread := false

	if spec.Affinity != nil && spec.Affinity.PodAntiAffinity != nil {
		hasAntiAffinity = true
	}
	if len(spec.TopologySpreadConstraints) > 0 {
		hasTopologySpread = true
	}

	if !hasAntiAffinity && !hasTopologySpread && replicaCount > 1 {
		affScore = 50
		risks = append(risks, "no anti-affinity or topology spread — all pods may land on same node")
	} else if !hasAntiAffinity && !hasTopologySpread {
		affScore = 80 // single replica, less critical
	}

	dims = append(dims, ScoreDimension{
		Name:        "affinity",
		Score:       affScore,
		Status:      dimStatus(affScore),
		Description: fmt.Sprintf("antiAffinity=%v topologySpread=%v", hasAntiAffinity, hasTopologySpread),
	})

	// ========================================
	// Compute overall score
	// ========================================
	totalScore := 0
	for _, d := range dims {
		totalScore += d.Score
	}
	overallScore := totalScore / len(dims)
	sc.Score = overallScore
	sc.Grade = scoreToGradeReliability(overallScore)
	sc.Dimensions = dims
	sc.Risks = risks

	// Determine top fix (highest-impact improvement)
	sc.TopFix = determineTopFix(dims)

	return sc
}

// buildPDBSelectors extracts match label selectors from PDBs.
func buildPDBSelectors(pdbs *policyv1.PodDisruptionBudgetList) []map[string]string {
	var selectors []map[string]string
	if pdbs == nil {
		return selectors
	}
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil {
			selectors = append(selectors, pdb.Spec.Selector.MatchLabels)
		}
	}
	return selectors
}

// labelsMatchSelector checks if workload labels match a PDB selector.
func labelsMatchSelector(workloadLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if workloadLabels[k] != v {
			return false
		}
	}
	return true
}

// isSystemNSReliability returns true for system namespaces.
func isSystemNSReliability(ns string) bool {
	return isSystemNS(ns)
}

// scoreToGradeReliability converts a 0-100 score to A-F grade.
func scoreToGradeReliability(score int) string {
	return scoreToGrade(score)
}

// dimStatus converts a dimension score to status.
func dimStatus(score int) string {
	switch {
	case score >= 70:
		return "good"
	case score >= 50:
		return "warning"
	default:
		return "critical"
	}
}

// dimensionImpact returns a human-readable impact description.
func dimensionImpact(dim string) string {
	impacts := map[string]string{
		"replication": "no high availability — single point of failure",
		"probes":      "no health detection — broken pods receive traffic",
		"resources":   "no resource boundaries — scheduling and OOM risk",
		"pdb":         "no disruption protection — updates cause downtime",
		"security":    "poor security posture — privilege escalation risk",
		"strategy":    "downtime-causing update strategy",
		"affinity":    "no distribution — all pods on one node",
	}
	if desc, ok := impacts[dim]; ok {
		return desc
	}
	return "reliability risk"
}

// determineTopFix returns the highest-impact improvement for a workload.
func determineTopFix(dims []ScoreDimension) string {
	// Priority order: replication > probes > resources > pdb > security > strategy > affinity
	priority := []string{"replication", "probes", "resources", "pdb", "security", "strategy", "affinity"}
	fixes := map[string]string{
		"replication": "increase replicas to >=3 for high availability",
		"probes":      "add readiness and liveness probes",
		"resources":   "set CPU/memory requests and limits",
		"pdb":         "add PodDisruptionBudget",
		"security":    "set runAsNonRoot and remove privileged flag",
		"strategy":    "use RollingUpdate strategy with proper grace period",
		"affinity":    "add pod anti-affinity or topology spread constraints",
	}
	for _, p := range priority {
		for _, d := range dims {
			if d.Name == p && d.Score < 70 {
				return fixes[p]
			}
		}
	}
	return "workload meets reliability baseline"
}

// generateScorecardRecs produces cluster-level recommendations.
func generateScorecardRecs(result ReliabilityScorecardResult) []string {
	var recs []string

	if result.Summary.CriticalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("URGENT: %d workload(s) scored grade F — immediate reliability risk", result.Summary.CriticalWorkloads))
	}
	if result.Summary.AtRiskWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) at risk (grade D/F) — prioritize reliability improvements", result.Summary.AtRiskWorkloads))
	}

	// Top 3 weakest signals
	for i, ws := range result.WeakestSignals {
		if i >= 3 {
			break
		}
		recs = append(recs, fmt.Sprintf("Cluster weakness: %s affects %d workload(s) — %s", ws.Dimension, ws.Count, ws.Impact))
	}

	if result.ClusterGrade == "A" || result.ClusterGrade == "B" {
		recs = append(recs, fmt.Sprintf("Cluster reliability grade %s (score %d/100) — well-configured", result.ClusterGrade, result.ClusterScore))
	} else if result.ClusterGrade == "C" {
		recs = append(recs, fmt.Sprintf("Cluster reliability grade %s (score %d/100) — room for improvement", result.ClusterGrade, result.ClusterScore))
	} else {
		recs = append(recs, fmt.Sprintf("Cluster reliability grade %s (score %d/100) — significant reliability gaps", result.ClusterGrade, result.ClusterScore))
	}

	if len(recs) == 1 && result.ClusterGrade == "A" {
		recs = append(recs, "All workloads meet reliability baseline — no critical improvements needed")
	}

	return recs
}

// Ensure imports are used
var _ = strings.Contains
