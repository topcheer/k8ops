package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TopoSpreadResult is the topology spread constraint validation analysis.
type TopoSpreadResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         TopoSpreadSummary `json:"summary"`
	ByWorkload      []TopoSpreadEntry `json:"byWorkload"`
	Violations      []TopoViolation   `json:"violations"`
	DomainAnalysis  []DomainStat      `json:"domainAnalysis"`
	Recommendations []string          `json:"recommendations"`
}

// TopoSpreadSummary aggregates topology spread compliance.
type TopoSpreadSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	WithSpread       int `json:"withSpreadConstraints"`
	WithoutSpread    int `json:"withoutSpreadConstraints"`
	ViolationCount   int `json:"violationCount"`
	DomainSkewIssues int `json:"domainSkewIssues"`
	HealthScore      int `json:"healthScore"`
}

// TopoSpreadEntry describes one workload's topology spread config.
type TopoSpreadEntry struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	WorkloadType      string `json:"workloadType"`
	Replicas          int32  `json:"replicas"`
	HasConstraints    bool   `json:"hasConstraints"`
	ConstraintCount   int    `json:"constraintCount"`
	MaxSkew           int32  `json:"maxSkew,omitempty"`
	TopologyKey       string `json:"topologyKey,omitempty"`
	WhenUnsatisfiable string `json:"whenUnsatisfiable,omitempty"`
	RiskLevel         string `json:"riskLevel"`
}

// TopoViolation is a detected spread constraint violation.
type TopoViolation struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// DomainStat shows pod distribution across a topology domain.
type DomainStat struct {
	TopologyKey string         `json:"topologyKey"`
	Domains     map[string]int `json:"domains"`
	MaxSkew     int            `json:"maxSkew"`
}

// handleTopologySpreadAudit validates pod topology spread constraints across workloads.
// GET /api/product/topology-spread
func (s *Server) handleTopologySpreadAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node label map for domain analysis
	nodeLabels := map[string]map[string]string{}
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	for _, node := range nodes.Items {
		nodeLabels[node.Name] = node.Labels
	}

	now := time.Now()
	result := TopoSpreadResult{ScannedAt: now}

	// Process Deployments
	for _, dep := range deployments.Items {
		result.Summary.TotalWorkloads++
		entry := analyzeSpreadFromPodTemplate(
			dep.Name, dep.Namespace, "Deployment",
			dep.Spec.Replicas, &dep.Spec.Template.Spec,
		)
		processEntry(&entry, &result)
	}

	// Process StatefulSets
	for _, ss := range statefulsets.Items {
		result.Summary.TotalWorkloads++
		entry := analyzeSpreadFromPodTemplate(
			ss.Name, ss.Namespace, "StatefulSet",
			ss.Spec.Replicas, &ss.Spec.Template.Spec,
		)
		processEntry(&entry, &result)
	}

	// Process DaemonSets (always on every node, spread is less relevant but still checked)
	for _, ds := range daemonsets.Items {
		result.Summary.TotalWorkloads++
		entry := analyzeSpreadFromPodTemplate(
			ds.Name, ds.Namespace, "DaemonSet",
			nil, &ds.Spec.Template.Spec,
		)
		processEntry(&entry, &result)
	}

	// Analyze actual pod distribution across topology domains
	result.DomainAnalysis = analyzePodDomains(pods.Items, nodeLabels)

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].RiskLevel < result.ByWorkload[j].RiskLevel
	})
	sort.Slice(result.Violations, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Violations[i].Severity] < sevOrder[result.Violations[j].Severity]
	})
	if len(result.Violations) > 30 {
		result.Violations = result.Violations[:30]
	}

	result.Summary.HealthScore = topoSpreadScore(result.Summary)
	result.Recommendations = topoSpreadRecommendations(&result)

	writeJSON(w, result)
}

// analyzeSpreadFromPodTemplate extracts topology spread info from a pod template spec.
func analyzeSpreadFromPodTemplate(name, ns, wlType string, replicas *int32, spec *corev1.PodSpec) TopoSpreadEntry {
	entry := TopoSpreadEntry{
		Name:         name,
		Namespace:    ns,
		WorkloadType: wlType,
	}
	if replicas != nil {
		entry.Replicas = *replicas
	}

	if len(spec.TopologySpreadConstraints) > 0 {
		entry.HasConstraints = true
		entry.ConstraintCount = len(spec.TopologySpreadConstraints)
		c := spec.TopologySpreadConstraints[0]
		entry.MaxSkew = c.MaxSkew
		entry.TopologyKey = c.TopologyKey
		entry.WhenUnsatisfiable = string(c.WhenUnsatisfiable)
		entry.RiskLevel = "low"

		// Validate constraint quality
		if c.MaxSkew == 0 {
			entry.RiskLevel = "medium"
		}
		if c.TopologyKey == "" {
			entry.RiskLevel = "high"
		}
	} else {
		entry.HasConstraints = false
		// Multi-replica workloads without spread constraints are at risk
		if replicas != nil && *replicas > 1 {
			entry.RiskLevel = "medium"
			if *replicas >= 3 {
				entry.RiskLevel = "high"
			}
		} else {
			entry.RiskLevel = "low"
		}
	}

	return entry
}

// processEntry adds an entry to the result and generates violations.
func processEntry(entry *TopoSpreadEntry, result *TopoSpreadResult) {
	if entry.HasConstraints {
		result.Summary.WithSpread++
	} else {
		result.Summary.WithoutSpread++
	}

	result.ByWorkload = append(result.ByWorkload, *entry)

	// Generate violations
	if !entry.HasConstraints && entry.Replicas >= 2 {
		result.Summary.ViolationCount++
		result.Violations = append(result.Violations, TopoViolation{
			Name:      entry.Name,
			Namespace: entry.Namespace,
			Issue:     fmt.Sprintf("%s has %d replicas but no topology spread constraints — pods may concentrate on one node/zone", entry.WorkloadType, entry.Replicas),
			Severity:  entry.RiskLevel,
		})
	}

	if entry.HasConstraints && entry.WhenUnsatisfiable == "ScheduleAnyway" {
		result.Violations = append(result.Violations, TopoViolation{
			Name:      entry.Name,
			Namespace: entry.Namespace,
			Issue:     "whenUnsatisfiable=ScheduleAnyway allows skew beyond maxSkew — use DoNotSchedule for strict enforcement",
			Severity:  "low",
		})
	}
}

// analyzePodDomains analyzes actual pod distribution across topology domains.
func analyzePodDomains(pods []corev1.Pod, nodeLabels map[string]map[string]string) []DomainStat {
	// Group by topology.kubernetes.io/zone and kubernetes.io/hostname
	zoneDist := map[string]int{}
	hostDist := map[string]int{}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue
		}
		labels, ok := nodeLabels[nodeName]
		if !ok {
			continue
		}

		zone := labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = "unknown"
		}
		zoneDist[zone]++

		hostDist[nodeName]++
	}

	var stats []DomainStat
	if len(zoneDist) > 0 {
		stats = append(stats, DomainStat{
			TopologyKey: "topology.kubernetes.io/zone",
			Domains:     zoneDist,
			MaxSkew:     computeSkew(zoneDist),
		})
	}
	if len(hostDist) > 0 {
		stats = append(stats, DomainStat{
			TopologyKey: "kubernetes.io/hostname",
			Domains:     hostDist,
			MaxSkew:     computeSkew(hostDist),
		})
	}
	return stats
}

// computeSkew calculates the max skew (max - min) across domains.
func computeSkew(dist map[string]int) int {
	if len(dist) == 0 {
		return 0
	}
	min, max := int(^uint(0)>>1), 0
	for _, v := range dist {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return max - min
}

// topoSpreadScore computes a 0-100 health score.
func topoSpreadScore(s TopoSpreadSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}

	score := 100

	noSpreadRatio := float64(s.WithoutSpread) / float64(s.TotalWorkloads)
	score -= int(noSpreadRatio * 40)

	if s.ViolationCount > 0 {
		score -= min(30, s.ViolationCount*5)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// topoSpreadRecommendations generates actionable recommendations.
func topoSpreadRecommendations(r *TopoSpreadResult) []string {
	var recs []string

	if r.Summary.WithoutSpread > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d workload(s) have no topology spread constraints — add constraints with maxSkew and topologyKey for high availability",
			r.Summary.WithoutSpread,
		))
	}

	for _, ds := range r.DomainAnalysis {
		if ds.MaxSkew > 2 {
			recs = append(recs, fmt.Sprintf(
				"Pod distribution skew is %d across %s — rebalance pods for better fault tolerance",
				ds.MaxSkew, ds.TopologyKey,
			))
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Topology spread constraints are well configured — workloads are properly distributed")
	}

	return recs
}
