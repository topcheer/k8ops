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

// CriticalityResult scores workload business criticality based on multi-dimensional
// signals: replica count, PDB presence, HPA presence, resource requests, traffic
// exposure (Service/Ingress), age stability, and namespace patterns. Workloads
// are classified into tiers (Tier-0 critical through Tier-3 best-effort) to help
// prioritize operational attention and define SLA targets.
type CriticalityResult struct {
	ScannedAt       time.Time    `json:"scannedAt"`
	Summary         CritSummary  `json:"summary"`
	Workloads       []CritEntry  `json:"workloads"`
	ByTier          []TierStat   `json:"byTier"`
	ByNamespace     []CritNSStat `json:"byNamespace"`
	SLAMatrix       []SLATier    `json:"slaMatrix"`
	HealthScore     int          `json:"healthScore"`
	Grade           string       `json:"grade"`
	Recommendations []string     `json:"recommendations"`
}

// CritSummary aggregates criticality statistics.
type CritSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	Tier0Critical   int `json:"tier0Critical"`
	Tier1Important  int `json:"tier1Important"`
	Tier2Standard   int `json:"tier2Standard"`
	Tier3BestEffort int `json:"tier3BestEffort"`
	WithPDB         int `json:"withPDB"`
	WithHPA         int `json:"withHPA"`
	WithIngress     int `json:"withIngress"`
	HAWorkloads     int `json:"haWorkloads"` // replicas >= 3
}

// CritEntry describes one workload's criticality assessment.
type CritEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Kind         string   `json:"kind"`
	Tier         string   `json:"tier"`  // Tier-0, Tier-1, Tier-2, Tier-3
	Score        int      `json:"score"` // 0-100
	Replicas     int      `json:"replicas"`
	HasPDB       bool     `json:"hasPDB"`
	HasHPA       bool     `json:"hasHPA"`
	HasIngress   bool     `json:"hasIngress"`
	HasService   bool     `json:"hasService"`
	CPURequest   float64  `json:"cpuRequest"`
	MemRequestGB float64  `json:"memRequestGB"`
	AgeDays      int      `json:"ageDays"`
	Signals      []string `json:"signals"`
	SLATarget    string   `json:"slaTarget"`
}

// TierStat per-tier statistics.
type TierStat struct {
	Tier    string  `json:"tier"`
	Count   int     `json:"count"`
	Pct     float64 `json:"pct"`
	WithPDB int     `json:"withPDB"`
	WithHPA int     `json:"withHPA"`
}

// CritNSStat per-namespace criticality stats.
type CritNSStat struct {
	Namespace string `json:"namespace"`
	Total     int    `json:"totalWorkloads"`
	Tier0     int    `json:"tier0"`
	Tier1     int    `json:"tier1"`
	Tier2     int    `json:"tier2"`
	Tier3     int    `json:"tier3"`
}

// SLATier defines SLA targets per tier.
type SLATier struct {
	Tier               string  `json:"tier"`
	Name               string  `json:"name"`
	AvailabilityTarget float64 `json:"availabilityTarget"`
	RTOTarget          string  `json:"rtoTarget"`
	MTTRTarget         string  `json:"mttrTarget"`
	Description        string  `json:"description"`
}

// handleCriticality handles GET /api/product/workload-criticality
func (s *Server) handleCriticality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CriticalityResult{ScannedAt: time.Now()}
	now := time.Now()

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build lookup maps
	svcSet := map[string]bool{} // ns/name
	ingressSet := map[string]bool{}
	hpaSet := map[string]bool{}
	pdbNameSet := map[string]bool{}

	for _, svc := range services.Items {
		if len(svc.Spec.Selector) > 0 {
			svcSet[svc.Namespace] = true // namespace-level: has at least one service
		}
	}
	for _, ing := range ingresses.Items {
		ingressSet[ing.Namespace] = true
	}
	for _, hpa := range hpas.Items {
		hpaSet[hpa.Namespace+"/"+hpa.Spec.ScaleTargetRef.Name] = true
	}
	for _, pdb := range pdbs.Items {
		pdbNameSet[pdb.Namespace] = true
	}

	// Classify each workload
	classify := func(name, ns string, kind string, replicas int32, creationTime metav1.Time, labels map[string]string, podSpec corev1.PodSpec) CritEntry {
		entry := CritEntry{
			Name: name, Namespace: ns, Kind: kind,
			Replicas: int(replicas),
		}

		if !creationTime.IsZero() {
			entry.AgeDays = int(now.Sub(creationTime.Time).Hours() / 24)
		}

		key := ns + "/" + name
		entry.HasHPA = hpaSet[key]
		entry.HasService = svcSet[ns]
		entry.HasIngress = ingressSet[ns]
		entry.HasPDB = pdbNameSet[ns]

		// Resource requests
		for _, c := range podSpec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					entry.CPURequest += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					entry.MemRequestGB += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}

		// Compute criticality score
		entry.Score, entry.Signals = computeCritScore(entry)

		// Assign tier
		switch {
		case entry.Score >= 70:
			entry.Tier = "Tier-0"
			entry.SLATarget = "99.99%"
		case entry.Score >= 50:
			entry.Tier = "Tier-1"
			entry.SLATarget = "99.9%"
		case entry.Score >= 30:
			entry.Tier = "Tier-2"
			entry.SLATarget = "99.5%"
		default:
			entry.Tier = "Tier-3"
			entry.SLATarget = "best-effort"
		}

		return entry
	}

	var allEntries []CritEntry
	nsStats := map[string]*CritNSStat{}

	process := func(entry CritEntry) {
		if isSystemNamespace(entry.Namespace) {
			return
		}
		allEntries = append(allEntries, entry)

		ns := entry.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &CritNSStat{Namespace: ns}
		}
		nsStats[ns].Total++
		switch entry.Tier {
		case "Tier-0":
			nsStats[ns].Tier0++
		case "Tier-1":
			nsStats[ns].Tier1++
		case "Tier-2":
			nsStats[ns].Tier2++
		default:
			nsStats[ns].Tier3++
		}
	}

	for _, dep := range deployments.Items {
		entry := classify(dep.Name, dep.Namespace, "Deployment", *dep.Spec.Replicas, dep.CreationTimestamp, dep.Labels, dep.Spec.Template.Spec)
		process(entry)
	}
	for _, sts := range statefulsets.Items {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		entry := classify(sts.Name, sts.Namespace, "StatefulSet", replicas, sts.CreationTimestamp, sts.Labels, sts.Spec.Template.Spec)
		process(entry)
	}
	for _, ds := range daemonsets.Items {
		entry := classify(ds.Name, ds.Namespace, "DaemonSet", ds.Status.DesiredNumberScheduled, ds.CreationTimestamp, ds.Labels, ds.Spec.Template.Spec)
		process(entry)
	}

	// Sort by score descending
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Score > allEntries[j].Score
	})
	if len(allEntries) > 50 {
		allEntries = allEntries[:50]
	}
	result.Workloads = allEntries

	// Summary
	result.Summary.TotalWorkloads = len(allEntries)
	for _, e := range allEntries {
		switch e.Tier {
		case "Tier-0":
			result.Summary.Tier0Critical++
		case "Tier-1":
			result.Summary.Tier1Important++
		case "Tier-2":
			result.Summary.Tier2Standard++
		default:
			result.Summary.Tier3BestEffort++
		}
		if e.HasPDB {
			result.Summary.WithPDB++
		}
		if e.HasHPA {
			result.Summary.WithHPA++
		}
		if e.HasIngress {
			result.Summary.WithIngress++
		}
		if e.Replicas >= 3 {
			result.Summary.HAWorkloads++
		}
	}

	// By tier stats
	for _, tier := range []string{"Tier-0", "Tier-1", "Tier-2", "Tier-3"} {
		count := 0
		withPDB := 0
		withHPA := 0
		for _, e := range allEntries {
			if e.Tier == tier {
				count++
				if e.HasPDB {
					withPDB++
				}
				if e.HasHPA {
					withHPA++
				}
			}
		}
		if count == 0 {
			continue
		}
		pct := 0.0
		if result.Summary.TotalWorkloads > 0 {
			pct = float64(count) / float64(result.Summary.TotalWorkloads) * 100
		}
		result.ByTier = append(result.ByTier, TierStat{Tier: tier, Count: count, Pct: pct, WithPDB: withPDB, WithHPA: withHPA})
	}

	// Namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Tier0 > result.ByNamespace[j].Tier0
	})

	// SLA matrix
	result.SLAMatrix = []SLATier{
		{Tier: "Tier-0", Name: "Critical", AvailabilityTarget: 99.99, RTOTarget: "<5min", MTTRTarget: "<15min", Description: "Business-critical, customer-facing services"},
		{Tier: "Tier-1", Name: "Important", AvailabilityTarget: 99.9, RTOTarget: "<15min", MTTRTarget: "<30min", Description: "Important internal services with moderate impact"},
		{Tier: "Tier-2", Name: "Standard", AvailabilityTarget: 99.5, RTOTarget: "<1h", MTTRTarget: "<2h", Description: "Standard workloads with limited blast radius"},
		{Tier: "Tier-3", Name: "Best-effort", AvailabilityTarget: 95.0, RTOTarget: "<4h", MTTRTarget: "<8h", Description: "Dev/test/non-critical workloads"},
	}

	// Score
	result.HealthScore = computeCritHealthScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateCritRecs(result)

	writeJSON(w, result)
}

// computeCritScore computes a 0-100 criticality score.
func computeCritScore(e CritEntry) (int, []string) {
	score := 0
	var signals []string

	// Replicas (max 30 points)
	if e.Replicas >= 5 {
		score += 30
		signals = append(signals, fmt.Sprintf("%d replicas (high HA)", e.Replicas))
	} else if e.Replicas >= 3 {
		score += 25
		signals = append(signals, fmt.Sprintf("%d replicas (HA)", e.Replicas))
	} else if e.Replicas >= 2 {
		score += 15
	} else {
		score += 5
	}

	// PDB presence (15 points)
	if e.HasPDB {
		score += 15
		signals = append(signals, "has PDB")
	}

	// HPA presence (10 points)
	if e.HasHPA {
		score += 10
		signals = append(signals, "has HPA")
	}

	// Ingress exposure (15 points)
	if e.HasIngress {
		score += 15
		signals = append(signals, "externally accessible")
	} else if e.HasService {
		score += 8
		signals = append(signals, "has Service")
	}

	// Resource commitment (10 points)
	if e.CPURequest >= 2 {
		score += 10
		signals = append(signals, fmt.Sprintf("%.1f CPU cores requested", e.CPURequest))
	} else if e.CPURequest >= 1 {
		score += 7
	} else if e.CPURequest > 0 {
		score += 3
	}

	// Age stability (10 points)
	if e.AgeDays > 90 {
		score += 10
		signals = append(signals, fmt.Sprintf("stable for %d days", e.AgeDays))
	} else if e.AgeDays > 30 {
		score += 7
	} else if e.AgeDays > 7 {
		score += 3
	}

	// Namespace name heuristic (10 points)
	nsLower := strings.ToLower(e.Namespace)
	if strings.Contains(nsLower, "prod") && !strings.Contains(nsLower, "staging") {
		score += 10
		signals = append(signals, "production namespace")
	} else if strings.Contains(nsLower, "stag") {
		score += 5
	}

	if score > 100 {
		score = 100
	}
	return score, signals
}

// computeCritHealthScore evaluates how well criticality is managed.
func computeCritHealthScore(s CritSummary) int {
	score := 100
	if s.TotalWorkloads == 0 {
		return score
	}
	// Tier-0 without PDB is critical gap
	if s.Tier0Critical > 0 {
		pdbGap := s.Tier0Critical - s.WithPDB
		if pdbGap > 0 {
			score -= minInt(pdbGap*5, 25)
		}
	}
	// Tier-0/1 without HPA
	if s.Tier0Critical+s.Tier1Important > 0 {
		hpaGap := (s.Tier0Critical + s.Tier1Important) - s.WithHPA
		if hpaGap > 0 {
			score -= minInt(hpaGap*3, 15)
		}
	}
	// Non-HA critical workloads
	if s.Tier0Critical > 0 && s.HAWorkloads == 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateCritRecs produces recommendations.
func generateCritRecs(r CriticalityResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Criticality assessment: %d workloads — Tier-0: %d, Tier-1: %d, Tier-2: %d, Tier-3: %d (score %d/100)",
		r.Summary.TotalWorkloads, r.Summary.Tier0Critical, r.Summary.Tier1Important, r.Summary.Tier2Standard, r.Summary.Tier3BestEffort, r.HealthScore))

	if r.Summary.Tier0Critical > 0 {
		pdbGap := r.Summary.Tier0Critical - r.Summary.WithPDB
		if pdbGap > 0 {
			recs = append(recs, fmt.Sprintf("%d Tier-0 workload(s) without PDB — add PDBs for critical services", pdbGap))
		}
		hpaGap := r.Summary.Tier0Critical - r.Summary.WithHPA
		if hpaGap > 0 {
			recs = append(recs, fmt.Sprintf("%d Tier-0 workload(s) without HPA — add autoscaling for demand spikes", hpaGap))
		}
	}

	for _, ts := range r.ByTier {
		recs = append(recs, fmt.Sprintf("%s (%s): %d workloads, PDB %d/%d, HPA %d/%d",
			ts.Tier, tierName(ts.Tier), ts.Count, ts.WithPDB, ts.Count, ts.WithHPA, ts.Count))
	}

	return recs
}

// tierName returns human-readable tier name.
func tierName(tier string) string {
	switch tier {
	case "Tier-0":
		return "Critical"
	case "Tier-1":
		return "Important"
	case "Tier-2":
		return "Standard"
	default:
		return "Best-effort"
	}
}

// Suppress unused import
var _ appsv1.Deployment = appsv1.Deployment{}
var _ = strings.ToLower
