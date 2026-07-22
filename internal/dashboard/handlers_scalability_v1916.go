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
// v19.16 — Scalability & HA Dimension (Round 5)
// 1. API Server QPS Throttle Risk
// 2. Pod Density Optimizer
// 3. Resource Overcommit Forecast
// ============================================================

// ---------------------------------------------------------------
// 1. API Server QPS Throttle Risk — estimate API server load
// ---------------------------------------------------------------

type APIThrottleResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         APIThrottleSummary `json:"summary"`
	ByNamespace     []APIThrottleNS    `json:"byNamespace"`
	HighConsumers   []APIThrottleNS    `json:"highConsumers"`
	Recommendations []string           `json:"recommendations"`
}

type APIThrottleSummary struct {
	TotalControllers  int `json:"totalControllers"`
	EstimatedQPS      int `json:"estimatedQPS"`
	APIWorkers        int `json:"estimatedAPIWorkers"`
	RiskScore         int `json:"riskScore"`
	HighQPSNamespaces int `json:"highQPSNamespaces"`
	TotalWatchEvents  int `json:"estimatedWatchEventsPerMin"`
}

type APIThrottleNS struct {
	Namespace  string `json:"namespace"`
	PodCount   int    `json:"podCount"`
	ConfigMaps int    `json:"configMaps"`
	Secrets    int    `json:"secrets"`
	Services   int    `json:"services"`
	EstQPS     int    `json:"estimatedQPS"`
	RiskLevel  string `json:"riskLevel"`
}

func (s *Server) handleAPIThrottleRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := APIThrottleResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	nsData := map[string]*APIThrottleNS{}
	totalPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		totalPods++
		nsE, ok := nsData[pod.Namespace]
		if !ok {
			nsE = &APIThrottleNS{Namespace: pod.Namespace}
			nsData[pod.Namespace] = nsE
		}
		nsE.PodCount++
	}
	for _, cm := range cms.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		nsE, ok := nsData[cm.Namespace]
		if !ok {
			nsE = &APIThrottleNS{Namespace: cm.Namespace}
			nsData[cm.Namespace] = nsE
		}
		nsE.ConfigMaps++
	}
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		nsE, ok := nsData[sec.Namespace]
		if !ok {
			nsE = &APIThrottleNS{Namespace: sec.Namespace}
			nsData[sec.Namespace] = nsE
		}
		nsE.Secrets++
	}
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		nsE, ok := nsData[svc.Namespace]
		if !ok {
			nsE = &APIThrottleNS{Namespace: svc.Namespace}
			nsData[svc.Namespace] = nsE
		}
		nsE.Services++
	}

	// Estimate QPS: each pod ~0.5 QPS for controller reconciliation + 0.1 per resource
	for _, ns := range nsData {
		ns.EstQPS = ns.PodCount/2 + ns.ConfigMaps/10 + ns.Secrets/10 + ns.Services
		result.Summary.EstimatedQPS += ns.EstQPS
		result.Summary.TotalWatchEvents += ns.PodCount * 5
		if ns.EstQPS > 50 {
			ns.RiskLevel = "high"
			result.Summary.HighQPSNamespaces++
			result.HighConsumers = append(result.HighConsumers, *ns)
		} else if ns.EstQPS > 20 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	result.Summary.TotalControllers = totalPods / 50 // rough estimate
	result.Summary.APIWorkers = totalPods / 100
	if result.Summary.APIWorkers < 1 {
		result.Summary.APIWorkers = 1
	}

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].EstQPS > result.ByNamespace[j].EstQPS
	})

	// Score: lower QPS = better
	if result.Summary.EstimatedQPS > 500 {
		result.HealthScore = 30
	} else if result.Summary.EstimatedQPS > 200 {
		result.HealthScore = 60
	} else if result.Summary.EstimatedQPS > 100 {
		result.HealthScore = 80
	} else {
		result.HealthScore = 95
	}
	result.Summary.RiskScore = 100 - result.HealthScore
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildAPIThrottleRecs1916(&result)
	writeJSON(w, result)
}

func buildAPIThrottleRecs1916(r *APIThrottleResult) []string {
	recs := []string{fmt.Sprintf("API throttle risk: est %d QPS across %d namespaces (%d high-consumer), %d watch events/min",
		r.Summary.EstimatedQPS, len(r.ByNamespace), r.Summary.HighQPSNamespaces,
		r.Summary.TotalWatchEvents)}
	if r.Summary.HighQPSNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d high-QPS namespaces - consider reducing controller polling intervals", r.Summary.HighQPSNamespaces))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Pod Density Optimizer — optimal bin-packing recommendations
// ---------------------------------------------------------------

type PodDensityResult1916 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PodDensitySummary `json:"summary"`
	ByNode          []PodDensityNode  `json:"byNode"`
	Optimization    []PodDensityOpt   `json:"optimizations"`
	Recommendations []string          `json:"recommendations"`
}

type PodDensitySummary struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	MaxPodLimit    int     `json:"maxPodLimitPerNode"`
	DensityPct     int     `json:"avgDensityPct"`
	UnderutilNodes int     `json:"underutilizedNodes"`
	OverutilNodes  int     `json:"overutilizedNodes"`
	WasteScore     int     `json:"wasteScore"`
}

type PodDensityNode struct {
	Node       string `json:"node"`
	PodCount   int    `json:"podCount"`
	MaxPods    int    `json:"maxPods"`
	DensityPct int    `json:"densityPct"`
	CPUmUsed   int    `json:"cpuMilliUsed"`
	CPUmCap    int    `json:"cpuMilliCap"`
	Status     string `json:"status"`
}

type PodDensityOpt struct {
	Type    string `json:"type"`
	Node    string `json:"node,omitempty"`
	Action  string `json:"action"`
	Savings string `json:"estimatedSavings"`
}

func (s *Server) handlePodDensityOpt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PodDensityResult1916{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nodePods := map[string]int{}
	nodeCPUUsed := map[string]int{}
	nodeCPUCap := map[string]int{}
	nodeMaxPods := map[string]int{}

	totalPods := 0
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		if !isSystemNamespace(pod.Namespace) {
			totalPods++
		}
		nodePods[pod.Spec.NodeName]++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				nodeCPUUsed[pod.Spec.NodeName] += int(qty.MilliValue())
			}
		}
	}

	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++
		maxPods := 110 // default kubelet max-pods
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			maxPods = int(qty.Value())
		}
		nodeMaxPods[node.Name] = maxPods
		result.Summary.MaxPodLimit = maxPods

		cpuCap := 0
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			cpuCap = int(qty.MilliValue())
		}
		nodeCPUCap[node.Name] = cpuCap

		density := 0
		if maxPods > 0 {
			density = nodePods[node.Name] * 100 / maxPods
		}
		cpuDensity := 0
		if cpuCap > 0 {
			cpuDensity = nodeCPUUsed[node.Name] * 100 / cpuCap
		}

		entry := PodDensityNode{
			Node: node.Name, PodCount: nodePods[node.Name],
			MaxPods: maxPods, DensityPct: density,
			CPUmUsed: nodeCPUUsed[node.Name], CPUmCap: cpuCap,
		}

		if density < 30 && cpuDensity < 30 {
			entry.Status = "underutilized"
			result.Summary.UnderutilNodes++
			result.Optimization = append(result.Optimization, PodDensityOpt{
				Type: "consolidate", Node: node.Name,
				Action:  "Node underutilized (<30% pod density) - consider consolidation",
				Savings: "~50% node cost",
			})
		} else if density > 80 || cpuDensity > 80 {
			entry.Status = "overutilized"
			result.Summary.OverutilNodes++
			result.Optimization = append(result.Optimization, PodDensityOpt{
				Type: "expand", Node: node.Name,
				Action:  "Node overutilized (>80%) - add capacity to prevent scheduling failures",
				Savings: "prevents pod pending",
			})
		} else {
			entry.Status = "optimal"
		}

		result.ByNode = append(result.ByNode, entry)
	}

	result.Summary.TotalPods = totalPods
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = float64(totalPods) / float64(result.Summary.TotalNodes)
		result.Summary.DensityPct = int(result.Summary.AvgPodsPerNode) * 100 / result.Summary.MaxPodLimit
	}

	// Waste score: underutilized nodes = waste
	result.Summary.WasteScore = result.Summary.UnderutilNodes * 30
	if result.Summary.WasteScore > 100 {
		result.Summary.WasteScore = 100
	}

	// Health: fewer under/over-utilized = better
	result.HealthScore = 100 - result.Summary.WasteScore - result.Summary.OverutilNodes*20
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildPodDensityRecs1916(&result)
	writeJSON(w, result)
}

func buildPodDensityRecs1916(r *PodDensityResult1916) []string {
	recs := []string{fmt.Sprintf("Pod density: %d nodes, %d pods (%.1f/node), avg density %d%%, %d underutil, %d overutil",
		r.Summary.TotalNodes, r.Summary.TotalPods, r.Summary.AvgPodsPerNode,
		r.Summary.DensityPct, r.Summary.UnderutilNodes, r.Summary.OverutilNodes)}
	if r.Summary.UnderutilNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d underutilized nodes - consolidate workloads to reduce node count", r.Summary.UnderutilNodes))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Resource Overcommit Forecast — overcommit ratio & risk
// ---------------------------------------------------------------

type OvercommitForecastResult1916 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         OvercommitSummary1916 `json:"summary"`
	ByNamespace     []OvercommitNSEntry   `json:"byNamespace"`
	RiskWorkloads   []OvercommitRiskEntry `json:"riskWorkloads"`
	Recommendations []string              `json:"recommendations"`
}

type OvercommitSummary1916 struct {
	TotalRequestsCPUm int     `json:"totalRequestsCPUm"`
	TotalLimitsCPUm   int     `json:"totalLimitsCPUm"`
	OvercommitRatio   float64 `json:"cpuOvercommitRatio"`
	MemOvercommit     float64 `json:"memOvercommitRatio"`
	UnboundedCount    int     `json:"unboundedWorkloads"`
	AtRiskCount       int     `json:"atRiskWorkloads"`
	TotalCapacity     int     `json:"totalCapacityCPUm"`
}

type OvercommitNSEntry struct {
	Namespace string  `json:"namespace"`
	ReqCPUm   int     `json:"reqCPUm"`
	LimitCPUm int     `json:"limitCPUm"`
	Ratio     float64 `json:"overcommitRatio"`
	RiskLevel string  `json:"riskLevel"`
}

type OvercommitRiskEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleOvercommitForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := OvercommitForecastResult1916{ScannedAt: time.Now()}

	// Get cluster capacity
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	totalCapCPUm := 0
	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			totalCapCPUm += int(qty.MilliValue())
		}
	}
	result.Summary.TotalCapacity = totalCapCPUm

	// Aggregate requests vs limits
	nsReqCPUm := map[string]int{}
	nsLimitCPUm := map[string]int{}
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue()) * int(replicas)
				nsReqCPUm[dep.Namespace] += m
				result.Summary.TotalRequestsCPUm += m
			}
			if qty, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue()) * int(replicas)
				nsLimitCPUm[dep.Namespace] += m
				result.Summary.TotalLimitsCPUm += m
			} else {
				result.Summary.UnboundedCount++
				result.RiskWorkloads = append(result.RiskWorkloads, OvercommitRiskEntry{
					Name: dep.Name, Namespace: dep.Namespace,
					Issue:     "no CPU limit set - can consume unbounded CPU",
					RiskLevel: "high",
				})
			}
		}
	}

	// Per-NS overcommit ratio
	for ns, req := range nsReqCPUm {
		limit := nsLimitCPUm[ns]
		ratio := 0.0
		if req > 0 {
			ratio = float64(limit) / float64(req)
		}
		entry := OvercommitNSEntry{
			Namespace: ns, ReqCPUm: req, LimitCPUm: limit, Ratio: ratio,
		}
		if ratio > 5 {
			entry.RiskLevel = "high"
			result.Summary.AtRiskCount++
		} else if ratio > 3 {
			entry.RiskLevel = "medium"
		} else {
			entry.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, entry)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Ratio > result.ByNamespace[j].Ratio
	})

	// Global overcommit ratio
	if result.Summary.TotalRequestsCPUm > 0 {
		result.Summary.OvercommitRatio = float64(result.Summary.TotalLimitsCPUm) / float64(result.Summary.TotalRequestsCPUm)
	}
	if totalCapCPUm > 0 {
		result.Summary.OvercommitRatio = float64(result.Summary.TotalLimitsCPUm) / float64(totalCapCPUm)
	}

	// Score: lower overcommit = better
	if result.Summary.OvercommitRatio > 5 {
		result.HealthScore = 20
	} else if result.Summary.OvercommitRatio > 3 {
		result.HealthScore = 40
	} else if result.Summary.OvercommitRatio > 2 {
		result.HealthScore = 70
	} else {
		result.HealthScore = 90
	}
	if result.Summary.UnboundedCount > 0 {
		result.HealthScore -= 10
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildOvercommitRecs1916(&result)
	writeJSON(w, result)
}

func buildOvercommitRecs1916(r *OvercommitForecastResult1916) []string {
	recs := []string{fmt.Sprintf("Overcommit forecast: ratio %.1fx, %d unbounded, %d at-risk, capacity %dm CPU",
		r.Summary.OvercommitRatio, r.Summary.UnboundedCount,
		r.Summary.AtRiskCount, r.Summary.TotalCapacity)}
	if r.Summary.UnboundedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without CPU limits - set limits to prevent resource starvation", r.Summary.UnboundedCount))
	}
	if r.Summary.OvercommitRatio > 3 {
		recs = append(recs, fmt.Sprintf("CPU overcommit ratio %.1fx is high - add nodes or reduce limits", r.Summary.OvercommitRatio))
	}
	return recs
}
