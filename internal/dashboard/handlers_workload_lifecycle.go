package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadLifecycleResult classifies workloads by lifecycle stage and operational maturity.
// It auto-detects development, staging, production, deprecated, and legacy workloads
// from labels, annotations, namespace patterns, resource patterns, and age to help
// prioritize operational attention and cleanup.
type WorkloadLifecycleResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         WLLifecycleSummary `json:"summary"`
	Workloads       []LifecycleEntry   `json:"workloads"`
	ByStage         []StageStat        `json:"byStage"`
	ByNamespace     []LifecycleNSStat  `json:"byNamespace"`
	CleanupTargets  []CleanupTarget    `json:"cleanupTargets"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

// WLLifecycleSummary aggregates lifecycle statistics.
type WLLifecycleSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	Production        int     `json:"production"`
	Staging           int     `json:"staging"`
	Development       int     `json:"development"`
	Deprecated        int     `json:"deprecated"`
	Legacy            int     `json:"legacy"`
	CleanupCandidates int     `json:"cleanupCandidates"`
	AvgAge            float64 `json:"avgAgeDays"`
	StaleWorkloads    int     `json:"staleWorkloads"`
}

// LifecycleEntry describes one workload's lifecycle classification.
type LifecycleEntry struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Kind       string   `json:"kind"`
	Stage      string   `json:"stage"`      // production, staging, development, deprecated, legacy
	Confidence int      `json:"confidence"` // 0-100
	AgeDays    int      `json:"ageDays"`
	Replicas   int      `json:"replicas"`
	HasHPA     bool     `json:"hasHPA"`
	HasPDB     bool     `json:"hasPDB"`
	Signals    []string `json:"signals"`
	RiskLevel  string   `json:"riskLevel"`
	Priority   string   `json:"priority"` // P0, P1, P2, P3
}

// StageStat per-stage statistics.
type StageStat struct {
	Stage      string  `json:"stage"`
	Count      int     `json:"count"`
	Pct        float64 `json:"pct"`
	AvgAgeDays float64 `json:"avgAgeDays"`
	WithPDB    int     `json:"withPDB"`
	WithHPA    int     `json:"withHPA"`
}

// LifecycleNSStat per-namespace lifecycle stats.
type LifecycleNSStat struct {
	Namespace      string `json:"namespace"`
	TotalWorkloads int    `json:"totalWorkloads"`
	Production     int    `json:"production"`
	Staging        int    `json:"staging"`
	Development    int    `json:"development"`
	Deprecated     int    `json:"deprecated"`
	Legacy         int    `json:"legacy"`
}

// CleanupTarget describes a workload recommended for cleanup.
type CleanupTarget struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Stage     string `json:"stage"`
	Reason    string `json:"reason"`
	AgeDays   int    `json:"ageDays"`
	Action    string `json:"action"` // delete, archive, scale-down
	Priority  int    `json:"priority"`
}

// handleWorkloadLifecycle handles GET /api/deployment/workload-lifecycle
func (s *Server) handleWorkloadLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := WorkloadLifecycleResult{ScannedAt: time.Now()}
	now := time.Now()

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build HPA and PDB target sets
	hpaTargets := map[string]bool{}
	for _, hpa := range hpas.Items {
		key := hpa.Namespace + "/" + hpa.Spec.ScaleTargetRef.Name
		hpaTargets[key] = true
	}

	pdbTargets := map[string]bool{}
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil {
			for _, pod := range pods.Items {
				if pod.Namespace == pdb.Namespace && podLabelsMatchSelector(pod.Labels, pdb.Spec.Selector) {
					pdbTargets[pod.Namespace+"/"+pod.OwnerReferences[0].Name] = true
				}
			}
		}
	}

	// Classify deployments
	var allEntries []LifecycleEntry
	nsStats := map[string]*LifecycleNSStat{}

	classifyDeployment := func(name, ns string, labels, annotations map[string]string, replicas int32, creationTime metav1.Time, kind string) LifecycleEntry {
		entry := LifecycleEntry{
			Name:      name,
			Namespace: ns,
			Kind:      kind,
			Replicas:  int(replicas),
		}

		if !creationTime.IsZero() {
			entry.AgeDays = int(now.Sub(creationTime.Time).Hours() / 24)
		}

		key := ns + "/" + name
		entry.HasHPA = hpaTargets[key]
		entry.HasPDB = pdbTargets[key]

		entry.Stage, entry.Confidence, entry.Signals = classifyLifecycleStage(name, ns, labels, annotations, entry.AgeDays, int(replicas))
		entry.Priority = stageToPriority(entry.Stage)
		entry.RiskLevel = stageToRisk(entry.Stage, entry.AgeDays)

		return entry
	}

	for _, dep := range deployments.Items {
		if strings.HasPrefix(dep.Namespace, "kube-") {
			continue
		}
		entry := classifyDeployment(dep.Name, dep.Namespace, dep.Labels, dep.Annotations, *dep.Spec.Replicas, dep.CreationTimestamp, "Deployment")
		allEntries = append(allEntries, entry)
	}

	for _, sts := range statefulsets.Items {
		if strings.HasPrefix(sts.Namespace, "kube-") {
			continue
		}
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		entry := classifyDeployment(sts.Name, sts.Namespace, sts.Labels, sts.Annotations, replicas, sts.CreationTimestamp, "StatefulSet")
		allEntries = append(allEntries, entry)
	}

	// Build summary and stats
	result.Summary.TotalWorkloads = len(allEntries)
	stageCounts := map[string]int{}
	stageAges := map[string][]float64{}
	stagePDB := map[string]int{}
	stageHPA := map[string]int{}
	totalAge := 0.0

	for _, entry := range allEntries {
		stageCounts[entry.Stage]++
		stageAges[entry.Stage] = append(stageAges[entry.Stage], float64(entry.AgeDays))
		totalAge += float64(entry.AgeDays)

		if entry.HasPDB {
			stagePDB[entry.Stage]++
		}
		if entry.HasHPA {
			stageHPA[entry.Stage]++
		}

		// Namespace stats
		ns := entry.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &LifecycleNSStat{Namespace: ns}
		}
		nsStats[ns].TotalWorkloads++
		switch entry.Stage {
		case "production":
			nsStats[ns].Production++
		case "staging":
			nsStats[ns].Staging++
		case "development":
			nsStats[ns].Development++
		case "deprecated":
			nsStats[ns].Deprecated++
		case "legacy":
			nsStats[ns].Legacy++
		}

		// Check for cleanup candidates
		if entry.Stage == "deprecated" || entry.Stage == "legacy" {
			result.Summary.CleanupCandidates++
			action := "scale-down"
			priority := 3
			if entry.Stage == "legacy" && entry.AgeDays > 180 {
				action = "delete"
				priority = 2
			}
			if entry.Stage == "deprecated" && entry.AgeDays > 90 {
				action = "archive"
				priority = 1
			}
			result.CleanupTargets = append(result.CleanupTargets, CleanupTarget{
				Name:      entry.Name,
				Namespace: entry.Namespace,
				Stage:     entry.Stage,
				Reason:    fmt.Sprintf("%s workload %d days old — %s", entry.Stage, entry.AgeDays, strings.Join(entry.Signals, "; ")),
				AgeDays:   entry.AgeDays,
				Action:    action,
				Priority:  priority,
			})
		}

		// Stale check: no updates in 90+ days
		if entry.AgeDays > 90 && entry.Stage != "production" {
			result.Summary.StaleWorkloads++
		}
	}

	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgAge = totalAge / float64(result.Summary.TotalWorkloads)
	}

	result.Summary.Production = stageCounts["production"]
	result.Summary.Staging = stageCounts["staging"]
	result.Summary.Development = stageCounts["development"]
	result.Summary.Deprecated = stageCounts["deprecated"]
	result.Summary.Legacy = stageCounts["legacy"]

	// Build stage stats
	for _, stage := range []string{"production", "staging", "development", "deprecated", "legacy"} {
		if stageCounts[stage] == 0 {
			continue
		}
		avgAge := 0.0
		if ages, ok := stageAges[stage]; ok && len(ages) > 0 {
			for _, a := range ages {
				avgAge += a
			}
			avgAge /= float64(len(ages))
		}
		pct := 0.0
		if result.Summary.TotalWorkloads > 0 {
			pct = float64(stageCounts[stage]) / float64(result.Summary.TotalWorkloads) * 100
		}
		result.ByStage = append(result.ByStage, StageStat{
			Stage:      stage,
			Count:      stageCounts[stage],
			Pct:        pct,
			AvgAgeDays: avgAge,
			WithPDB:    stagePDB[stage],
			WithHPA:    stageHPA[stage],
		})
	}

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalWorkloads > result.ByNamespace[j].TotalWorkloads
	})

	// Sort cleanup targets by priority
	sort.Slice(result.CleanupTargets, func(i, j int) bool {
		return result.CleanupTargets[i].Priority < result.CleanupTargets[j].Priority
	})

	// Limit workload entries to top 50
	if len(allEntries) > 50 {
		allEntries = allEntries[:50]
	}
	result.Workloads = allEntries

	// Compute health score
	result.HealthScore = computeLifecycleScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Generate recommendations
	result.Recommendations = generateWLLifecycleRecs(result.Summary, result.CleanupTargets, result.ByStage)

	writeJSON(w, result)
}

// classifyLifecycleStage determines the lifecycle stage of a workload.
func classifyLifecycleStage(name, ns string, labels, annotations map[string]string, ageDays, replicas int) (string, int, []string) {
	signals := []string{}
	scores := map[string]int{
		"production":  0,
		"staging":     0,
		"development": 0,
		"deprecated":  0,
		"legacy":      0,
	}

	combined := strings.ToLower(name + " " + ns)
	for k, v := range labels {
		combined += " " + strings.ToLower(k+"="+v)
	}
	for k, v := range annotations {
		combined += " " + strings.ToLower(k+"="+v)
	}

	// Namespace patterns
	nsLower := strings.ToLower(ns)
	switch {
	case strings.Contains(nsLower, "prod") && !strings.Contains(nsLower, "staging"):
		scores["production"] += 30
		signals = append(signals, "namespace contains 'prod'")
	case strings.Contains(nsLower, "staging") || strings.Contains(nsLower, "stage"):
		scores["staging"] += 25
		signals = append(signals, "namespace contains 'staging'")
	case strings.Contains(nsLower, "dev") || strings.Contains(nsLower, "test") || strings.Contains(nsLower, "sandbox"):
		scores["development"] += 25
		signals = append(signals, "namespace indicates development")
	}

	// Label-based detection
	for k, v := range labels {
		kvLower := strings.ToLower(k + "=" + v)
		switch {
		case strings.Contains(kvLower, "env=prod") || strings.Contains(kvLower, "environment=prod"):
			scores["production"] += 25
			signals = append(signals, fmt.Sprintf("label %s=%s", k, v))
		case strings.Contains(kvLower, "env=stag") || strings.Contains(kvLower, "environment=stag"):
			scores["staging"] += 20
			signals = append(signals, fmt.Sprintf("label %s=%s", k, v))
		case strings.Contains(kvLower, "env=dev") || strings.Contains(kvLower, "environment=dev"):
			scores["development"] += 20
			signals = append(signals, fmt.Sprintf("label %s=%s", k, v))
		}
	}

	// Replica-based detection
	if replicas >= 3 {
		scores["production"] += 15
		signals = append(signals, fmt.Sprintf("%d replicas suggests production", replicas))
	} else if replicas == 1 {
		scores["development"] += 10
		signals = append(signals, "single replica")
	}

	// Age-based legacy/deprecated detection
	if ageDays > 365 {
		scores["legacy"] += 20
		signals = append(signals, fmt.Sprintf("%d days old (>1 year)", ageDays))
	}
	if ageDays > 180 {
		scores["legacy"] += 10
	}

	// Name-based deprecated detection
	if strings.Contains(combined, "deprecated") || strings.Contains(combined, "legacy") || strings.Contains(combined, "old") || strings.Contains(combined, "v1-") || strings.Contains(combined, "backup-") {
		scores["deprecated"] += 25
		signals = append(signals, "name/labels suggest deprecation")
	}

	// Annotation-based deprecated detection
	if annotations != nil {
		if v, ok := annotations["deprecated"]; ok && strings.ToLower(v) == "true" {
			scores["deprecated"] += 50
			signals = append(signals, "annotation deprecated=true")
		}
		if v, ok := annotations["lifecycle.k8s.io/deprecated"]; ok && strings.ToLower(v) == "true" {
			scores["deprecated"] += 50
			signals = append(signals, "annotation lifecycle.k8s.io/deprecated=true")
		}
	}

	// Find best stage
	bestStage := "development" // default
	bestScore := 0
	for stage, score := range scores {
		if score > bestScore {
			bestScore = score
			bestStage = stage
		}
	}

	// If no signals at all, default to development with low confidence
	if bestScore == 0 {
		bestStage = "development"
		bestScore = 10
		signals = append(signals, "no lifecycle signals detected — defaulting to development")
	}

	confidence := minInt(bestScore, 100)
	return bestStage, confidence, signals
}

// stageToPriority maps lifecycle stage to operational priority.
func stageToPriority(stage string) string {
	switch stage {
	case "production":
		return "P0"
	case "staging":
		return "P1"
	case "development":
		return "P2"
	case "deprecated":
		return "P3"
	case "legacy":
		return "P3"
	default:
		return "P2"
	}
}

// stageToRisk maps lifecycle stage to risk level.
func stageToRisk(stage string, ageDays int) string {
	switch stage {
	case "production":
		return "critical"
	case "staging":
		return "medium"
	case "development":
		return "low"
	case "deprecated":
		if ageDays > 90 {
			return "medium"
		}
		return "low"
	case "legacy":
		if ageDays > 365 {
			return "high"
		}
		return "medium"
	default:
		return "low"
	}
}

// computeLifecycleScore computes a 0-100 lifecycle governance score.
func computeLifecycleScore(s WLLifecycleSummary) int {
	score := 100
	if s.TotalWorkloads == 0 {
		return score
	}
	// Penalize deprecated/legacy workloads
	cleanupRatio := float64(s.Deprecated+s.Legacy) / float64(s.TotalWorkloads)
	score -= int(cleanupRatio * 30)
	// Penalize stale workloads
	staleRatio := float64(s.StaleWorkloads) / float64(s.TotalWorkloads)
	score -= int(staleRatio * 20)
	// Penalize lack of production classification (too many dev with no prod)
	if s.Production == 0 && s.TotalWorkloads > 5 {
		score -= 15
	}
	// Penalize too many unclassified (default dev)
	if s.Development > 0 && s.Production == 0 && s.Staging == 0 {
		devRatio := float64(s.Development) / float64(s.TotalWorkloads)
		if devRatio > 0.8 {
			score -= 15
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateLifecycleRecs produces human-readable recommendations.
func generateWLLifecycleRecs(s WLLifecycleSummary, targets []CleanupTarget, stages []StageStat) []string {
	var recs []string

	if s.TotalWorkloads == 0 {
		recs = append(recs, "No workloads detected — lifecycle analysis not applicable")
		return recs
	}

	recs = append(recs, fmt.Sprintf("Lifecycle governance score: %d/100 (grade %s) across %d workloads",
		computeLifecycleScore(s), scoreToGrade(computeLifecycleScore(s)), s.TotalWorkloads))

	// Stage distribution
	for _, st := range stages {
		recs = append(recs, fmt.Sprintf("%s: %d workloads (%.0f%%), avg age %.0f days, PDB %d/%d, HPA %d/%d",
			st.Stage, st.Count, st.Pct, st.AvgAgeDays, st.WithPDB, st.Count, st.WithHPA, st.Count))
	}

	if s.CleanupCandidates > 0 {
		recs = append(recs, fmt.Sprintf("%d cleanup candidate(s) identified — deprecated/legacy workloads consuming resources", s.CleanupCandidates))
	}

	if s.StaleWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d stale workload(s) (>90 days old, non-production) — review for cleanup or promotion", s.StaleWorkloads))
	}

	if s.Production == 0 && s.TotalWorkloads > 5 {
		recs = append(recs, "No production-classified workloads detected — add env=production labels to critical workloads")
	}

	return recs
}

// podLabelsMatchSelector checks if pod labels match a label selector (simplified).
func podLabelsMatchSelector(podLabels map[string]string, selector *metav1.LabelSelector) bool {
	if selector == nil {
		return false
	}
	for key, val := range selector.MatchLabels {
		if podLabels[key] != val {
			return false
		}
	}
	return true
}
