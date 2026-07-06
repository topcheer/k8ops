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
	"k8s.io/apimachinery/pkg/labels"
)

// TSResult is the topology spread & pod distribution analysis.
type TSAuditResult struct {
	ScannedAt         time.Time    `json:"scannedAt"`
	Summary           TSSummary    `json:"summary"`
	ByController      []TSEntry    `json:"byController"`
	Concentrated      []TSEntry    `json:"concentrated"` // workloads concentrated on 1-2 nodes
	WellSpread        []TSEntry    `json:"wellSpread"`
	NodeLoadImbalance []TSNodeLoad `json:"nodeLoadImbalance"`
	NoConstraints     []TSEntry    `json:"noConstraints"`
	Issues            []TSIssue    `json:"issues"`
	Recommendations   []string     `json:"recommendations"`
}

// TSSummary aggregates topology spread statistics.
type TSSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithConstraints   int `json:"withConstraints"` // has topologySpreadConstraints
	NoConstraints     int `json:"noConstraints"`   // no constraints at all
	Concentrated      int `json:"concentrated"`    // >50% pods on 1 node
	WellSpread        int `json:"wellSpread"`      // evenly distributed
	AntiAffinitySet   int `json:"antiAffinitySet"` // has podAntiAffinity
	TotalNodes        int `json:"totalNodes"`
	MaxNodeLoad       int `json:"maxNodeLoad"` // most pods on a single node
	MinNodeLoad       int `json:"minNodeLoad"`
	DistributionScore int `json:"distributionScore"` // 0-100 (higher = better spread)
}

// TSEntry describes one workload's pod distribution.
type TSEntry struct {
	Name             string         `json:"name"`
	Namespace        string         `json:"namespace"`
	Kind             string         `json:"kind"` // Deployment / StatefulSet / DaemonSet
	Replicas         int32          `json:"replicas"`
	NodeDistribution map[string]int `json:"nodeDistribution"` // node → pod count
	HasTSC           bool           `json:"hasTopologySpreadConstraints"`
	HasAntiAffinity  bool           `json:"hasAntiAffinity"`
	MaxPerNode       int            `json:"maxPerNode"`
	UniqueNodes      int            `json:"uniqueNodes"`
	SpreadRatio      float64        `json:"spreadRatio"` // uniqueNodes / min(replicas, totalNodes)
	RiskLevel        string         `json:"riskLevel"`
}

// TSNodeLoad describes pod distribution across nodes.
type TSNodeLoad struct {
	NodeName        string `json:"nodeName"`
	PodCount        int    `json:"podCount"`
	IsUnschedulable bool   `json:"isUnschedulable"`
	Zone            string `json:"zone,omitempty"`
}

// TSIssue is a detected distribution problem.
type TSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleTopologySpread audits pod distribution and topology spread constraints.
// GET /api/operations/topology-spread
func (s *Server) handleTSAudit(w http.ResponseWriter, r *http.Request) {
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

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := TSAuditResult{ScannedAt: time.Now()}
	result.Summary.TotalNodes = len(nodes.Items)

	// Build node → zone map
	nodeZoneMap := make(map[string]string)
	for _, node := range nodes.Items {
		if zone, ok := node.Labels[corev1.LabelTopologyZone]; ok {
			nodeZoneMap[node.Name] = zone
		}
	}

	// Build pod → owner map
	type ownerKey struct {
		kind, name, namespace string
	}
	podOwners := make(map[string]ownerKey) // pod ns/name → owner
	nodePods := make(map[string]int)       // node → pod count

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Spec.NodeName != "" {
			nodePods[pod.Spec.NodeName]++
		}

		// Find owning controller
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" {
				// Strip hash to find deployment
				rsName := strings.TrimSuffix(ownerRef.Name, "")
				// Try to find matching deployment by checking ReplicaSet name prefix
				for _, dep := range deployments.Items {
					if strings.HasPrefix(ownerRef.Name, dep.Name+"-") {
						podOwners[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = ownerKey{"Deployment", dep.Name, pod.Namespace}
						break
					}
				}
				_ = rsName
			} else if ownerRef.Kind == "DaemonSet" || ownerRef.Kind == "StatefulSet" {
				podOwners[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = ownerKey{ownerRef.Kind, ownerRef.Name, pod.Namespace}
			}
		}
	}

	// Node load
	for _, node := range nodes.Items {
		load := TSNodeLoad{
			NodeName:        node.Name,
			PodCount:        nodePods[node.Name],
			IsUnschedulable: node.Spec.Unschedulable,
		}
		if zone, ok := nodeZoneMap[node.Name]; ok {
			load.Zone = zone
		}
		result.NodeLoadImbalance = append(result.NodeLoadImbalance, load)

		if load.PodCount > result.Summary.MaxNodeLoad {
			result.Summary.MaxNodeLoad = load.PodCount
		}
		if result.Summary.MinNodeLoad == 0 || (load.PodCount < result.Summary.MinNodeLoad && load.PodCount > 0) {
			result.Summary.MinNodeLoad = load.PodCount
		}
	}

	// Analyze deployments
	for _, dep := range deployments.Items {
		entry := tsAnalyzeDeployment(dep, pods.Items, nodeZoneMap, result.Summary.TotalNodes)
		result.Summary.TotalWorkloads++

		if entry.HasTSC {
			result.Summary.WithConstraints++
		} else {
			result.Summary.NoConstraints++
			result.NoConstraints = append(result.NoConstraints, entry)
		}

		if entry.HasAntiAffinity {
			result.Summary.AntiAffinitySet++
		}

		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.Summary.Concentrated++
			result.Concentrated = append(result.Concentrated, entry)
			if entry.MaxPerNode > 0 && entry.Replicas > 0 {
				pct := float64(entry.MaxPerNode) / float64(entry.Replicas) * 100
				if pct > 50 {
					result.Issues = append(result.Issues, TSIssue{
						Severity: "critical", Type: "pod-concentration",
						Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
						Message:  fmt.Sprintf("Deployment %s/%s has %.0f%% of replicas (%d/%d) on a single node — single-node failure risk", dep.Namespace, dep.Name, pct, entry.MaxPerNode, entry.Replicas),
					})
				}
			}
		} else if entry.RiskLevel == "low" {
			result.Summary.WellSpread++
			result.WellSpread = append(result.WellSpread, entry)
		}

		// Warn about no constraints for multi-replica
		if !entry.HasTSC && !entry.HasAntiAffinity && entry.Replicas >= 3 {
			result.Issues = append(result.Issues, TSIssue{
				Severity: "warning", Type: "no-spread-constraints",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has %d replicas but no topologySpreadConstraints or podAntiAffinity", dep.Namespace, dep.Name, entry.Replicas),
			})
		}

		result.ByController = append(result.ByController, entry)
	}

	// Sort
	sort.Slice(result.ByController, func(i, j int) bool {
		return tsRiskRank(result.ByController[i].RiskLevel) < tsRiskRank(result.ByController[j].RiskLevel)
	})
	sort.Slice(result.Concentrated, func(i, j int) bool {
		return result.Concentrated[i].MaxPerNode > result.Concentrated[j].MaxPerNode
	})
	sort.Slice(result.NodeLoadImbalance, func(i, j int) bool {
		return result.NodeLoadImbalance[i].PodCount > result.NodeLoadImbalance[j].PodCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return tsIssueRank(result.Issues[i].Severity) < tsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.DistributionScore = tsScore(result.Summary)
	result.Recommendations = tsGenRecs(result.Summary, result.Concentrated, result.NoConstraints)

	writeJSON(w, result)
}

// tsAnalyzeDeployment analyzes a single deployment's pod distribution.
func tsAnalyzeDeployment(dep appsv1.Deployment, allPods []corev1.Pod, nodeZoneMap map[string]string, totalNodes int) TSEntry {
	entry := TSEntry{
		Name:      dep.Name,
		Namespace: dep.Namespace,
		Kind:      "Deployment",
	}

	if dep.Spec.Replicas != nil {
		entry.Replicas = *dep.Spec.Replicas
	}

	// Check topology spread constraints
	if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
		entry.HasTSC = true
	}

	// Check pod anti-affinity
	if dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
		entry.HasAntiAffinity = true
	}

	// Find matching pods
	depLabels := labels.Set(dep.Spec.Selector.MatchLabels)
	nodeDist := make(map[string]int)

	for _, pod := range allPods {
		if pod.Namespace != dep.Namespace || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}
		// Match by selector labels
		if !depLabels.AsSelector().Matches(labels.Set(pod.Labels)) {
			// Also check owner reference
			matched := false
			for _, ownerRef := range pod.OwnerReferences {
				if strings.HasPrefix(ownerRef.Name, dep.Name+"-") {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		nodeDist[pod.Spec.NodeName]++
	}

	entry.NodeDistribution = nodeDist
	entry.UniqueNodes = len(nodeDist)

	for _, count := range nodeDist {
		if count > entry.MaxPerNode {
			entry.MaxPerNode = count
		}
	}

	// Spread ratio: how many unique nodes / expected
	expectedNodes := int(entry.Replicas)
	if totalNodes > 0 && totalNodes < expectedNodes {
		expectedNodes = totalNodes
	}
	if expectedNodes > 0 {
		entry.SpreadRatio = float64(entry.UniqueNodes) / float64(expectedNodes)
	}

	// Risk level
	entry.RiskLevel = tsAssessRisk(entry)

	return entry
}

// tsAssessRisk determines risk based on pod concentration.
func tsAssessRisk(entry TSEntry) string {
	if entry.Replicas < 2 {
		return "low" // single replica, no spread needed
	}
	if entry.Replicas > 0 {
		concentration := float64(entry.MaxPerNode) / float64(entry.Replicas)
		switch {
		case concentration > 0.7:
			return "critical"
		case concentration > 0.5:
			return "high"
		case concentration > 0.34:
			return "medium"
		default:
			return "low"
		}
	}
	return "low"
}

// tsScore computes 0-100.
func tsScore(s TSSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 0
	// Well-spread percentage
	spreadPct := float64(s.WellSpread) / float64(s.TotalWorkloads) * 100
	score = int(spreadPct)

	// Penalize concentration
	score -= s.Concentrated * 5

	// Node imbalance penalty
	if s.MaxNodeLoad > 0 && s.MinNodeLoad >= 0 {
		imbalance := float64(s.MaxNodeLoad-s.MinNodeLoad) / float64(s.MaxNodeLoad+1) * 100
		if imbalance > 70 {
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

// tsGenRecs produces actionable advice.
func tsGenRecs(s TSSummary, concentrated []TSEntry, noConstraints []TSEntry) []string {
	var recs []string

	if s.Concentrated > 0 {
		top := ""
		if len(concentrated) > 0 {
			c := concentrated[0]
			top = fmt.Sprintf(" (e.g. %s/%s: %d/%d replicas on one node)", c.Namespace, c.Name, c.MaxPerNode, c.Replicas)
		}
		recs = append(recs, fmt.Sprintf("%d workload(s) have pod concentration%s — add topologySpreadConstraints", s.Concentrated, top))
	}
	if s.NoConstraints > 0 {
		highRisk := 0
		for _, nc := range noConstraints {
			if nc.Replicas >= 3 {
				highRisk++
			}
		}
		if highRisk > 0 {
			recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) have NO topologySpreadConstraints or podAntiAffinity — at risk during node failure", highRisk))
		}
	}
	if s.AntiAffinitySet > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) use podAntiAffinity — good, consider also adding topologySpreadConstraints for zone-level spreading", s.AntiAffinitySet))
	}
	if s.MaxNodeLoad > 0 && s.MinNodeLoad >= 0 {
		imbalance := float64(s.MaxNodeLoad-s.MinNodeLoad) / float64(s.MaxNodeLoad+1) * 100
		if imbalance > 60 {
			recs = append(recs, fmt.Sprintf("Node load imbalance: max=%d, min=%d pods (%.0f%% difference) — consider rebalancing", s.MaxNodeLoad, s.MinNodeLoad, imbalance))
		}
	}
	if s.DistributionScore < 50 {
		recs = append(recs, fmt.Sprintf("Pod distribution score is %d/100 — pods are not well-spread across nodes", s.DistributionScore))
	}
	if s.Concentrated == 0 && s.NoConstraints == 0 {
		recs = append(recs, "All workloads are well-distributed across nodes — excellent fault tolerance")
	}

	return recs
}

func tsRiskRank(level string) int {
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

func tsIssueRank(s string) int {
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
