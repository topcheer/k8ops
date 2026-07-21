package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v18.99 — Scalability & HA Dimension
// 1. Resource Request Efficiency
// 2. Pod Bin-Packing Score
// 3. Multi-Zone HA Readiness
// ============================================================

// ---------------------------------------------------------------
// 1. Resource Request Efficiency — analyzes request vs actual usage
// ---------------------------------------------------------------

type RequestEfficiencyResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	HealthScore      int               `json:"healthScore"`
	Grade            string            `json:"grade"`
	Summary          RequestEffSummary `json:"summary"`
	OverProvisioned  []RequestEffEntry `json:"overProvisioned"`
	UnderProvisioned []RequestEffEntry `json:"underProvisioned"`
	ByNamespace      []NSRequestEntry  `json:"byNamespace"`
	Recommendations  []string          `json:"recommendations"`
}

type RequestEffSummary struct {
	TotalContainers  int `json:"totalContainers"`
	WithRequests     int `json:"withRequests"`
	WithoutRequests  int `json:"withoutRequests"`
	WithLimits       int `json:"withLimits"`
	WithoutLimits    int `json:"withoutLimits"`
	TotalCPUm        int `json:"totalCPUm"`
	TotalMemMB       int `json:"totalMemMB"`
	OverProvisioned  int `json:"overProvisioned"`
	UnderProvisioned int `json:"underProvisioned"`
}

type RequestEffEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	CPUm       int    `json:"cpuMilli"`
	MemMB      int    `json:"memMB"`
	LimitCPUm  int    `json:"limitCpuMilli"`
	LimitMemMB int    `json:"limitMemMB"`
	RiskLevel  string `json:"riskLevel"`
	Issue      string `json:"issue"`
}

type NSRequestEntry struct {
	Namespace  string `json:"namespace"`
	PodCount   int    `json:"podCount"`
	CPUm       int    `json:"cpuMilli"`
	MemMB      int    `json:"memMB"`
	NoRequests int    `json:"noRequests"`
}

func (s *Server) handleRequestEfficiency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RequestEfficiencyResult{ScannedAt: time.Now()}

	nsMap := map[string]*NSRequestEntry{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &NSRequestEntry{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.PodCount++

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			entry := RequestEffEntry{
				Name: dep.Name, Namespace: dep.Namespace, Container: c.Name,
			}

			reqCPUm := 0
			reqMemMB := 0
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPUm = int(qty.MilliValue())
				entry.CPUm = reqCPUm
				result.Summary.TotalCPUm += reqCPUm
				result.Summary.WithRequests++
				nsE.CPUm += reqCPUm
			} else {
				result.Summary.WithoutRequests++
				nsE.NoRequests++
				entry.Issue = "no CPU request"
				entry.RiskLevel = "high"
				result.UnderProvisioned = append(result.UnderProvisioned, entry)
				continue
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				reqMemMB = int(qty.Value() / (1024 * 1024))
				entry.MemMB = reqMemMB
				result.Summary.TotalMemMB += reqMemMB
				nsE.MemMB += reqMemMB
			}

			// Limits
			if qty, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				entry.LimitCPUm = int(qty.MilliValue())
				result.Summary.WithLimits++
			} else {
				result.Summary.WithoutLimits++
			}
			if qty, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				entry.LimitMemMB = int(qty.Value() / (1024 * 1024))
			}

			// Classify: over-provisioned (very high request) or under (no limits)
			if reqCPUm > 2000 {
				entry.RiskLevel = "medium"
				entry.Issue = fmt.Sprintf("high CPU request: %dm (verify actual usage)", reqCPUm)
				result.Summary.OverProvisioned++
				result.OverProvisioned = append(result.OverProvisioned, entry)
			} else if reqMemMB > 4096 {
				entry.RiskLevel = "medium"
				entry.Issue = fmt.Sprintf("high memory request: %dMB (verify actual usage)", reqMemMB)
				result.Summary.OverProvisioned++
				result.OverProvisioned = append(result.OverProvisioned, entry)
			} else if entry.LimitCPUm == 0 && entry.LimitMemMB == 0 {
				entry.RiskLevel = "medium"
				entry.Issue = "no resource limits set - can consume unlimited resources"
				result.Summary.UnderProvisioned++
				result.UnderProvisioned = append(result.UnderProvisioned, entry)
			}
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CPUm > result.ByNamespace[j].CPUm
	})

	// Score
	if result.Summary.TotalContainers > 0 {
		reqPct := result.Summary.WithRequests * 100 / result.Summary.TotalContainers
		limitPct := result.Summary.WithLimits * 100 / result.Summary.TotalContainers
		result.HealthScore = (reqPct + limitPct) / 2
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildReqEffRecs1899(&result)
	writeJSON(w, result)
}

func buildReqEffRecs1899(result *RequestEfficiencyResult) []string {
	recs := []string{
		fmt.Sprintf("Request efficiency: %d containers (%d with requests, %d with limits), %d over-provisioned",
			result.Summary.TotalContainers, result.Summary.WithRequests,
			result.Summary.WithLimits, result.Summary.OverProvisioned),
	}
	if result.Summary.WithoutRequests > 0 {
		recs = append(recs, fmt.Sprintf("%d containers without resource requests - scheduler cannot make good placement decisions", result.Summary.WithoutRequests))
	}
	if result.Summary.WithoutLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d containers without resource limits - risk of resource exhaustion", result.Summary.WithoutLimits))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Pod Bin-Packing Score — node resource packing efficiency
// ---------------------------------------------------------------

type BinPackingResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         BinPackSummary      `json:"summary"`
	NodeScores      []BinPackNodeEntry  `json:"nodeScores"`
	FragReport      []FragmentEntry1899 `json:"fragmentation"`
	Recommendations []string            `json:"recommendations"`
}

type BinPackSummary struct {
	TotalNodes        int `json:"totalNodes"`
	AvgCPUUsage       int `json:"avgCPUUsagePct"`
	AvgMemUsage       int `json:"avgMemUsagePct"`
	AvgPodUsage       int `json:"avgPodUsagePct"`
	BinPackingScore   int `json:"binPackingScore"`
	FragmentedNodes   int `json:"fragmentedNodes"`
	TotalPods         int `json:"totalPods"`
	TotalCapacityPods int `json:"totalCapacityPods"`
}

type BinPackNodeEntry struct {
	Node        string `json:"node"`
	CPUUsagePct int    `json:"cpuUsagePct"`
	MemUsagePct int    `json:"memUsagePct"`
	PodUsagePct int    `json:"podUsagePct"`
	PackScore   int    `json:"packScore"`
	Status      string `json:"status"`
}

type FragmentEntry1899 struct {
	Node        string `json:"node"`
	WastedCPUm  int    `json:"wastedCpuMilli"`
	WastedMemMB int    `json:"wastedMemMB"`
	WastedPods  int    `json:"wastedPods"`
}

func (s *Server) handleBinPackingScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := BinPackingResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Per-node resource tracking
	nodeReqCPUm := map[string]int{}
	nodeReqMemMB := map[string]int{}
	nodePodCount := map[string]int{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nodePodCount[pod.Spec.NodeName]++
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				nodeReqCPUm[pod.Spec.NodeName] += int(qty.MilliValue())
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				nodeReqMemMB[pod.Spec.NodeName] += int(qty.Value() / (1024 * 1024))
			}
		}
	}

	totalCPUPct := 0
	totalMemPct := 0
	totalPodPct := 0

	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++

		allocCPUm := 0
		allocMemMB := 0
		allocPods := 0
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			allocCPUm = int(qty.MilliValue())
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			allocMemMB = int(qty.Value() / (1024 * 1024))
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			allocPods = int(qty.Value())
		}
		result.Summary.TotalCapacityPods += allocPods

		reqCPUm := nodeReqCPUm[node.Name]
		reqMemMB := nodeReqMemMB[node.Name]
		podCount := nodePodCount[node.Name]

		cpuPct := 0
		memPct := 0
		podPct := 0
		if allocCPUm > 0 {
			cpuPct = reqCPUm * 100 / allocCPUm
		}
		if allocMemMB > 0 {
			memPct = reqMemMB * 100 / allocMemMB
		}
		if allocPods > 0 {
			podPct = podCount * 100 / allocPods
		}

		totalCPUPct += cpuPct
		totalMemPct += memPct
		totalPodPct += podPct

		// Pack score: how efficiently resources are used (higher = better packing)
		packScore := (cpuPct + memPct) / 2
		status := "balanced"
		if cpuPct > 85 || memPct > 85 {
			status = "overloaded"
		} else if cpuPct < 30 && memPct < 30 {
			status = "underutilized"
			result.Summary.FragmentedNodes++
			result.FragReport = append(result.FragReport, FragmentEntry1899{
				Node:        node.Name,
				WastedCPUm:  allocCPUm - reqCPUm,
				WastedMemMB: allocMemMB - reqMemMB,
				WastedPods:  allocPods - podCount,
			})
		}

		result.NodeScores = append(result.NodeScores, BinPackNodeEntry{
			Node: node.Name, CPUUsagePct: cpuPct, MemUsagePct: memPct,
			PodUsagePct: podPct, PackScore: packScore, Status: status,
		})
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUUsage = totalCPUPct / result.Summary.TotalNodes
		result.Summary.AvgMemUsage = totalMemPct / result.Summary.TotalNodes
		result.Summary.AvgPodUsage = totalPodPct / result.Summary.TotalNodes
		result.Summary.BinPackingScore = (result.Summary.AvgCPUUsage + result.Summary.AvgMemUsage) / 2
	}

	result.HealthScore = result.Summary.BinPackingScore
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildBinPackRecs1899(&result)
	writeJSON(w, result)
}

func buildBinPackRecs1899(result *BinPackingResult) []string {
	recs := []string{
		fmt.Sprintf("Bin-packing: %d nodes, avg CPU %d%%, avg Mem %d%%, pack score %d/100, %d fragmented",
			result.Summary.TotalNodes, result.Summary.AvgCPUUsage,
			result.Summary.AvgMemUsage, result.Summary.BinPackingScore,
			result.Summary.FragmentedNodes),
	}
	if result.Summary.FragmentedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d underutilized nodes - consider consolidating workloads or scaling down", result.Summary.FragmentedNodes))
	}
	if result.Summary.AvgPodUsage > 80 {
		recs = append(recs, fmt.Sprintf("Pod density at %d%% - approaching max pods per node limit", result.Summary.AvgPodUsage))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Multi-Zone HA Readiness — fault domain analysis
// ---------------------------------------------------------------

type MultiZoneHAResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         MultiZoneSummary      `json:"summary"`
	ZoneSpread      []ZoneSpreadEntry1899 `json:"zoneSpread"`
	AtRiskWorkloads []ZoneRiskEntry       `json:"atRiskWorkloads"`
	Recommendations []string              `json:"recommendations"`
}

type MultiZoneSummary struct {
	TotalNodes     int            `json:"totalNodes"`
	Zones          int            `json:"zones"`
	NodePerZone    map[string]int `json:"nodePerZone"`
	TotalWorkloads int            `json:"totalWorkloads"`
	SingleZoneWL   int            `json:"singleZoneWorkloads"`
	MultiZoneWL    int            `json:"multiZoneWorkloads"`
	NoZoneAffinity int            `json:"noZoneAffinity"`
}

type ZoneSpreadEntry1899 struct {
	Zone      string `json:"zone"`
	NodeCount int    `json:"nodeCount"`
	PodCount  int    `json:"podCount"`
}

type ZoneRiskEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	ZonesUsed int    `json:"zonesUsed"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

func (s *Server) handleMultiZoneHA(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := MultiZoneHAResult{ScannedAt: time.Now()}

	// Map nodes to zones
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	nodeZone := map[string]string{}
	zoneNodeCount := map[string]int{}
	zonePodCount := map[string]int{}

	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = node.Labels["failure-domain.beta.kubernetes.io/zone"]
		}
		if zone == "" {
			zone = "unknown"
		}
		nodeZone[node.Name] = zone
		zoneNodeCount[zone]++
	}
	result.Summary.Zones = len(zoneNodeCount)
	result.Summary.NodePerZone = zoneNodeCount

	// Pod distribution by zone
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		zone := nodeZone[pod.Spec.NodeName]
		zonePodCount[zone]++
	}

	for zone, nc := range zoneNodeCount {
		result.ZoneSpread = append(result.ZoneSpread, ZoneSpreadEntry1899{
			Zone: zone, NodeCount: nc, PodCount: zonePodCount[zone],
		})
	}
	sort.Slice(result.ZoneSpread, func(i, j int) bool {
		return result.ZoneSpread[i].PodCount > result.ZoneSpread[j].PodCount
	})

	// Analyze workloads for zone spread
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		// Check topology spread constraints
		hasSpread := len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0
		// Check pod anti-affinity for zones
		hasZoneAntiAffinity := false
		if dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasZoneAntiAffinity = true
		}

		// Count distinct zones for pods in this deployment
		wlZones := map[string]bool{}
		for _, pod := range pods.Items {
			if pod.Namespace == dep.Namespace && pod.Spec.NodeName != "" {
				// Check if this pod belongs to this deployment by label
				// Simple check: same namespace + name prefix
				if podOwnerIs1899(pod.OwnerReferences, dep.Name) {
					wlZones[nodeZone[pod.Spec.NodeName]] = true
				}
			}
		}
		zonesUsed := len(wlZones)

		entry := ZoneRiskEntry{
			Name: dep.Name, Namespace: dep.Namespace,
			Replicas: replicas, ZonesUsed: zonesUsed,
		}

		switch {
		case replicas <= 1:
			entry.RiskLevel = "high"
			entry.Issue = "single replica - no zone redundancy"
			result.Summary.SingleZoneWL++
		case hasSpread || hasZoneAntiAffinity:
			entry.RiskLevel = "low"
			result.Summary.MultiZoneWL++
		case zonesUsed <= 1 && result.Summary.Zones > 1:
			entry.RiskLevel = "high"
			entry.Issue = fmt.Sprintf("all %d replicas in single zone - add topologySpreadConstraints", replicas)
			result.Summary.SingleZoneWL++
			result.AtRiskWorkloads = append(result.AtRiskWorkloads, entry)
		case !hasSpread && !hasZoneAntiAffinity:
			entry.RiskLevel = "medium"
			entry.Issue = "no zone anti-affinity - pods may concentrate in single zone"
			result.Summary.NoZoneAffinity++
			result.AtRiskWorkloads = append(result.AtRiskWorkloads, entry)
		default:
			entry.RiskLevel = "low"
			result.Summary.MultiZoneWL++
		}
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		multiPct := result.Summary.MultiZoneWL * 100 / result.Summary.TotalWorkloads
		result.HealthScore = multiPct
	}
	// If single zone, cap score
	if result.Summary.Zones <= 1 {
		result.HealthScore = result.HealthScore / 2
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildMultiZoneRecs1899(&result)
	writeJSON(w, result)
}

func podOwnerIs1899(refs []metav1.OwnerReference, name string) bool {
	for _, ref := range refs {
		if ref.Name == name {
			return true
		}
	}
	return false
}

func buildMultiZoneRecs1899(result *MultiZoneHAResult) []string {
	recs := []string{
		fmt.Sprintf("Multi-zone HA: %d zones, %d nodes, %d workloads (%d multi-zone, %d single-zone)",
			result.Summary.Zones, result.Summary.TotalNodes,
			result.Summary.TotalWorkloads, result.Summary.MultiZoneWL,
			result.Summary.SingleZoneWL),
	}
	if result.Summary.Zones <= 1 {
		recs = append(recs, "Single-zone cluster - no zone redundancy. Add nodes in different zones for HA")
	}
	if result.Summary.SingleZoneWL > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads concentrated in single zone - add topologySpreadConstraints", result.Summary.SingleZoneWL))
	}
	if result.Summary.NoZoneAffinity > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without zone anti-affinity - pods may land in same zone after restart", result.Summary.NoZoneAffinity))
	}
	return recs
}
