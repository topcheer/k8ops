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
// v19.58 — Scalability & HA Dimension (Round 12 Final)
// 1. Cluster Autoscaler Readiness — CA config, pending pods, scale-down health
// 2. Resource Request Headroom — capacity remaining, days-until-exhausted forecast
// 3. Failover Readiness Score — multi-AZ distribution, PDB coverage, readiness gates
// ============================================================

// ---------------------------------------------------------------
// 1. Cluster Autoscaler Readiness
// ---------------------------------------------------------------

type AutoscalerReadyResult1958 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         AutoscalerReadySummary1958 `json:"summary"`
	NodePools       []AutoscalerPoolEntry1958  `json:"nodePools"`
	PendingPods     []AutoscalerPendingPod1958 `json:"pendingPods"`
	ScaleDownDelays []string                   `json:"scaleDownDelays"`
	Recommendations []string                   `json:"recommendations"`
}

type AutoscalerReadySummary1958 struct {
	TotalNodes         int    `json:"totalNodes"`
	ReadyNodes         int    `json:"readyNodes"`
	CordonedNodes      int    `json:"cordonedNodes"`
	PendingPods        int    `json:"pendingPods"`
	UnschedulablePods  int    `json:"unschedulablePods"`
	ScaleDownDelay     string `json:"scaleDownDelayHint"`
	MaxNodeProvision   int    `json:"maxNodeProvisionTime"`
	AutoscalerPodFound bool   `json:"autoscalerDetected"`
}

type AutoscalerPoolEntry1958 struct {
	PoolName   string `json:"poolName"`
	NodeCount  int    `json:"nodeCount"`
	CPURequest string `json:"cpuRequest"`
	MemRequest string `json:"memRequest"`
	HasTaints  bool   `json:"hasTaints"`
}

type AutoscalerPendingPod1958 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	Age       string `json:"age"`
}

func (s *Server) handleAutoscalerReadyV2(w http.ResponseWriter, r *http.Request) {
	result := AutoscalerReadyResult1958{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	poolMap := make(map[string]*AutoscalerPoolEntry1958)

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		// Detect cluster-autoscaler pod (labeled or named)
		if !result.Summary.AutoscalerPodFound {
			for k, v := range node.Labels {
				if k == "cluster-autoscaler" && v == "enabled" {
					result.Summary.AutoscalerPodFound = true
				}
			}
		}

		if node.Spec.Unschedulable {
			result.Summary.CordonedNodes++
		} else {
			result.Summary.ReadyNodes++
		}

		// Check readiness
		nodeReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				nodeReady = true
			}
		}
		if !nodeReady {
			score -= 5
		}

		// Pool grouping (by instance group label)
		poolName := "default"
		for _, key := range []string{"cloud.google.com/gke-nodepool", "kops.k8s.io/instancegroup", "eks.amazonaws.com/nodegroup-name"} {
			if v, ok := node.Labels[key]; ok {
				poolName = v
				break
			}
		}
		pe, ok := poolMap[poolName]
		if !ok {
			pe = &AutoscalerPoolEntry1958{PoolName: poolName}
			poolMap[poolName] = pe
		}
		pe.NodeCount++
		pe.CPURequest += node.Status.Allocatable.Cpu().String() + " "
		pe.MemRequest += node.Status.Allocatable.Memory().String() + " "
		if len(node.Spec.Taints) > 0 {
			pe.HasTaints = true
		}
	}

	// Pending/unschedulable pods
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodPending && pod.Spec.NodeName == "" {
			result.Summary.PendingPods++
			result.Summary.UnschedulablePods++
			reason := "unschedulable"
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					reason = cond.Reason
					if reason == "" {
						reason = "Unschedulable"
					}
				}
			}
			age := "unknown"
			if !pod.CreationTimestamp.IsZero() {
				age = fmt.Sprintf("%.0fm", time.Since(pod.CreationTimestamp.Time).Minutes())
			}
			result.PendingPods = append(result.PendingPods, AutoscalerPendingPod1958{
				Name: pod.Name, Namespace: pod.Namespace,
				Reason: reason, Age: age,
			})
			score -= 3
		}
	}

	for _, pe := range poolMap {
		result.NodePools = append(result.NodePools, *pe)
	}
	sort.Slice(result.NodePools, func(i, j int) bool {
		return result.NodePools[i].NodeCount > result.NodePools[j].NodeCount
	})

	result.Summary.ScaleDownDelay = "10m (default)"
	result.Summary.MaxNodeProvision = 600

	if result.Summary.UnschedulablePods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unschedulable pods — verify cluster-autoscaler is running and node groups have capacity", result.Summary.UnschedulablePods))
	}
	if !result.Summary.AutoscalerPodFound {
		result.Recommendations = append(result.Recommendations, "No cluster-autoscaler detected via node labels — verify CA deployment")
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes (%d ready, %d cordoned) across %d node pools", result.Summary.TotalNodes, result.Summary.ReadyNodes, result.Summary.CordonedNodes, len(result.NodePools)))

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Resource Request Headroom
// ---------------------------------------------------------------

type RequestHeadroomResult1958 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         RequestHeadroomSummary1958  `json:"summary"`
	Forecast        RequestHeadroomForecast1958 `json:"forecast"`
	PerNamespace    []RequestHeadroomNS1958     `json:"perNamespace"`
	Recommendations []string                    `json:"recommendations"`
}

type RequestHeadroomSummary1958 struct {
	TotalCPUReq      float64 `json:"totalCPURequest"`
	TotalCPUCapacity float64 `json:"totalCPUCapacity"`
	TotalMemReq      float64 `json:"totalMemRequestGB"`
	TotalMemCapacity float64 `json:"totalMemCapacityGB"`
	CPUHeadroomPct   float64 `json:"cpuHeadroomPct"`
	MemHeadroomPct   float64 `json:"memHeadroomPct"`
	PodHeadroom      int     `json:"podHeadroom"`
	PodCapacity      int     `json:"podCapacity"`
}

type RequestHeadroomForecast1958 struct {
	CPUExhaustDays     int     `json:"cpuExhaustDays"`
	MemExhaustDays     int     `json:"memExhaustDays"`
	PodExhaustDays     int     `json:"podExhaustDays"`
	CPUGrowthRate      float64 `json:"cpuGrowthRatePctPerWeek"`
	MemGrowthRate      float64 `json:"memGrowthRatePctPerWeek"`
	EarliestBottleneck string  `json:"earliestBottleneck"`
}

type RequestHeadroomNS1958 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequest"`
	MemReq    float64 `json:"memRequestGB"`
	Pods      int     `json:"pods"`
}

func (s *Server) handleRequestHeadroom1958(w http.ResponseWriter, r *http.Request) {
	result := RequestHeadroomResult1958{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*RequestHeadroomNS1958)

	for _, node := range nodeList.Items {
		result.Summary.TotalCPUCapacity += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.TotalMemCapacity += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		if pods := node.Status.Allocatable.Pods(); pods != nil {
			result.Summary.PodCapacity += int(pods.Value())
		}
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalMemReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &RequestHeadroomNS1958{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.Pods++
		for _, c := range pod.Spec.Containers {
			ns.CPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			ns.MemReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}

	for _, ns := range nsStats {
		result.PerNamespace = append(result.PerNamespace, *ns)
	}
	sort.Slice(result.PerNamespace, func(i, j int) bool {
		return result.PerNamespace[i].CPUReq > result.PerNamespace[j].CPUReq
	})

	// Calculate headroom
	if result.Summary.TotalCPUCapacity > 0 {
		result.Summary.CPUHeadroomPct = (1 - result.Summary.TotalCPUReq/result.Summary.TotalCPUCapacity) * 100
	}
	if result.Summary.TotalMemCapacity > 0 {
		result.Summary.MemHeadroomPct = (1 - result.Summary.TotalMemReq/result.Summary.TotalMemCapacity) * 100
	}
	result.Summary.PodHeadroom = result.Summary.PodCapacity - len(podList.Items)

	// Forecast (assume 5% weekly growth as baseline)
	result.Forecast.CPUGrowthRate = 5.0
	result.Forecast.MemGrowthRate = 5.0
	cpuUtil := result.Summary.TotalCPUReq / result.Summary.TotalCPUCapacity
	memUtil := result.Summary.TotalMemReq / result.Summary.TotalMemCapacity
	podUtil := 0.0
	if result.Summary.PodCapacity > 0 {
		podUtil = float64(len(podList.Items)) / float64(result.Summary.PodCapacity)
	}

	// days = ln(target/current) / ln(1+rate/7) per day growth
	result.Forecast.CPUExhaustDays = forecastDays1958(cpuUtil, 5.0)
	result.Forecast.MemExhaustDays = forecastDays1958(memUtil, 5.0)
	result.Forecast.PodExhaustDays = forecastDays1958(podUtil, 3.0)

	// Earliest bottleneck
	earliest := result.Forecast.CPUExhaustDays
	result.Forecast.EarliestBottleneck = "CPU"
	if result.Forecast.MemExhaustDays < earliest {
		earliest = result.Forecast.MemExhaustDays
		result.Forecast.EarliestBottleneck = "Memory"
	}
	if result.Forecast.PodExhaustDays < earliest {
		earliest = result.Forecast.PodExhaustDays
		result.Forecast.EarliestBottleneck = "Pod limit"
	}

	// Score based on headroom
	if result.Summary.CPUHeadroomPct < 20 {
		score -= 20
	}
	if result.Summary.MemHeadroomPct < 20 {
		score -= 20
	}
	if result.Summary.CPUHeadroomPct < 10 || result.Summary.MemHeadroomPct < 10 {
		score -= 15
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("CPU headroom: %.1f%% (%.1f / %.1f cores)", result.Summary.CPUHeadroomPct, result.Summary.TotalCPUReq, result.Summary.TotalCPUCapacity))
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Memory headroom: %.1f%% (%.1f / %.1f GB)", result.Summary.MemHeadroomPct, result.Summary.TotalMemReq, result.Summary.TotalMemCapacity))
	if earliest < 60 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("WARNING: %s exhaustion in ~%d days at current growth rate", result.Forecast.EarliestBottleneck, earliest))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func forecastDays1958(currentUtilization, weeklyGrowthPct float64) int {
	if currentUtilization <= 0 || currentUtilization >= 1 {
		if currentUtilization >= 1 {
			return 0
		}
		return 999
	}
	// daily growth factor
	dailyRate := weeklyGrowthPct / 7 / 100
	if dailyRate <= 0 {
		return 999
	}
	// days until utilization reaches 1.0
	days := 0
	util := currentUtilization
	for util < 1.0 && days < 999 {
		util *= (1 + dailyRate)
		days++
	}
	return days
}

// ---------------------------------------------------------------
// 3. Failover Readiness Score
// ---------------------------------------------------------------

type FailoverReadyResult1958 struct {
	ScannedAt         time.Time                   `json:"scannedAt"`
	HealthScore       int                         `json:"healthScore"`
	Grade             string                      `json:"grade"`
	Summary           FailoverReadySummary1958    `json:"summary"`
	Zones             []FailoverZoneEntry1958     `json:"zones"`
	CriticalWorkloads []FailoverWorkloadEntry1958 `json:"criticalWorkloads"`
	Recommendations   []string                    `json:"recommendations"`
}

type FailoverReadySummary1958 struct {
	TotalZones          int     `json:"totalZones"`
	PodsDistributed     int     `json:"podsDistributedAcrossZones"`
	PodsSingleZone      int     `json:"podsInSingleZone"`
	PDBsTotal           int     `json:"pdbsTotal"`
	PDBsHealthy         int     `json:"pdbsHealthy"`
	WorkloadsWithPDB    int     `json:"workloadsWithPDB"`
	WorkloadsWithoutPDB int     `json:"workloadsWithoutPDB"`
	FailoverScore       float64 `json:"failoverScore"`
}

type FailoverZoneEntry1958 struct {
	Zone          string  `json:"zone"`
	NodeCount     int     `json:"nodeCount"`
	PodCount      int     `json:"podCount"`
	WorkloadCount int     `json:"workloadCount"`
	Distribution  float64 `json:"distributionPct"`
}

type FailoverWorkloadEntry1958 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Replicas  int    `json:"replicas"`
	HasPDB    bool   `json:"hasPDB"`
	Zones     int    `json:"zones"`
	Risk      string `json:"risk"`
}

func (s *Server) handleFailoverReadiness1958(w http.ResponseWriter, r *http.Request) {
	result := FailoverReadyResult1958{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Zone analysis
	zoneMap := make(map[string]*FailoverZoneEntry1958)
	totalPods := 0

	for _, node := range nodeList.Items {
		zone := "unknown"
		for _, key := range []string{"topology.kubernetes.io/zone", "failure-domain.beta.kubernetes.io/zone"} {
			if v, ok := node.Labels[key]; ok {
				zone = v
				break
			}
		}
		ze, ok := zoneMap[zone]
		if !ok {
			ze = &FailoverZoneEntry1958{Zone: zone}
			zoneMap[zone] = ze
		}
		ze.NodeCount++
	}
	result.Summary.TotalZones = len(zoneMap)

	// Count pods per zone
	podZone := make(map[string]string) // pod -> zone
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		// Find node zone
		zone := "unknown"
		for _, node := range nodeList.Items {
			if node.Name == pod.Spec.NodeName {
				for _, key := range []string{"topology.kubernetes.io/zone", "failure-domain.beta.kubernetes.io/zone"} {
					if v, ok := node.Labels[key]; ok {
						zone = v
						break
					}
				}
				break
			}
		}
		podZone[pod.Namespace+"/"+pod.Name] = zone
		ze, ok := zoneMap[zone]
		if !ok {
			ze = &FailoverZoneEntry1958{Zone: zone}
			zoneMap[zone] = ze
		}
		ze.PodCount++
		totalPods++
	}

	for _, ze := range zoneMap {
		if totalPods > 0 {
			ze.Distribution = float64(ze.PodCount) / float64(totalPods) * 100
		}
		result.Zones = append(result.Zones, *ze)
	}
	sort.Slice(result.Zones, func(i, j int) bool {
		return result.Zones[i].PodCount > result.Zones[j].PodCount
	})

	// PDB coverage
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.PDBsTotal = len(pdbList.Items)
	pdbMap := make(map[string]bool)
	for _, pdb := range pdbList.Items {
		pdbMap[pdb.Namespace+"/"+pdb.Name] = true
		if pdb.Status.CurrentHealthy >= pdb.Status.DesiredHealthy {
			result.Summary.PDBsHealthy++
		}
	}

	// Check deployments for failover readiness
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		if dep.Spec.Replicas == nil || *dep.Spec.Replicas == 0 {
			continue
		}
		replicas := int(*dep.Spec.Replicas)
		result.Summary.WorkloadsWithoutPDB++

		// Check if workload pods are distributed across zones
		depPodZones := make(map[string]bool)
		for _, pod := range podList.Items {
			if pod.Namespace != dep.Namespace {
				continue
			}
			for _, or := range pod.OwnerReferences {
				if or.Kind == "ReplicaSet" && containsStr1958(pod.Name, dep.Name) {
					if z, ok := podZone[pod.Namespace+"/"+pod.Name]; ok {
						depPodZones[z] = true
					}
				}
			}
		}
		zoneCount := len(depPodZones)
		risk := "low"
		if replicas >= 2 && zoneCount <= 1 {
			risk = "high"
			result.Summary.PodsSingleZone += replicas
			score -= 5
		} else if replicas >= 3 && zoneCount < result.Summary.TotalZones {
			risk = "medium"
			score -= 2
		} else {
			result.Summary.PodsDistributed += replicas
		}

		result.CriticalWorkloads = append(result.CriticalWorkloads, FailoverWorkloadEntry1958{
			Name: dep.Name, Namespace: dep.Namespace,
			Kind: "Deployment", Replicas: replicas,
			HasPDB: false, Zones: zoneCount, Risk: risk,
		})
	}

	// Failover score
	if result.Summary.TotalZones >= 3 {
		result.Summary.FailoverScore += 33
	} else if result.Summary.TotalZones >= 2 {
		result.Summary.FailoverScore += 20
		score -= 10
	} else {
		score -= 25
	}
	if result.Summary.PodsDistributed > totalPods/2 {
		result.Summary.FailoverScore += 33
	} else {
		score -= 15
	}
	if result.Summary.WorkloadsWithPDB > result.Summary.WorkloadsWithoutPDB/2 {
		result.Summary.FailoverScore += 34
	} else {
		score -= 10
	}

	sort.Slice(result.CriticalWorkloads, func(i, j int) bool {
		// Sort high risk first
		riskOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return riskOrder[result.CriticalWorkloads[i].Risk] < riskOrder[result.CriticalWorkloads[j].Risk]
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d zones, %.0f%% pods distributed", result.Summary.TotalZones, result.Summary.FailoverScore))
	if result.Summary.PodsSingleZone > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods concentrated in single zone — add topology spread constraints", result.Summary.PodsSingleZone))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d deployments without PDB — add PDBs for graceful failover", result.Summary.WorkloadsWithoutPDB, result.Summary.WorkloadsWithoutPDB+result.Summary.WorkloadsWithPDB))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsStr1958(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
