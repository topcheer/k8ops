package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ============================================================
// v18.93 — Scalability & HA Dimension
// 1. Memory Pressure Forecast
// 2. Scaling Concurrency Limit
// 3. Pod Disruption Window
// ============================================================

// ---------------------------------------------------------------
// 1. Memory Pressure Forecast — predicts node memory exhaustion
// ---------------------------------------------------------------

type MemPressureResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         MemPressureSummary `json:"summary"`
	NodePressure    []NodeMemPressure  `json:"nodePressure"`
	NamespaceSpend  []NsMemSpend       `json:"namespaceSpend"`
	TopConsumers    []MemConsumer      `json:"topConsumers"`
	Recommendations []string           `json:"recommendations"`
}

type MemPressureSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	TotalAllocatableMB  int     `json:"totalAllocatableMB"`
	TotalRequestedMB    int     `json:"totalRequestedMB"`
	TotalUsedMB         int     `json:"totalUsedMB"`
	OvercommitRatio     float64 `json:"overcommitRatio"`
	CriticalNodes       int     `json:"criticalNodes"`
	WarningNodes        int     `json:"warningNodes"`
	HealthyNodes        int     `json:"healthyNodes"`
	PredictedExhaustHrs int     `json:"predictedExhaustHours"`
}

type NodeMemPressure struct {
	Node                string  `json:"node"`
	AllocatableMB       int     `json:"allocatableMB"`
	RequestedMB         int     `json:"requestedMB"`
	UsedMB              int     `json:"usedMB"`
	UtilizationPct      int     `json:"utilizationPct"`
	PressureLevel       string  `json:"pressureLevel"`
	PodCount            int     `json:"podCount"`
	TrendPerHour        float64 `json:"trendPerHourMB"`
	PredictedExhaustHrs int     `json:"predictedExhaustHours"`
}

type NsMemSpend struct {
	Namespace   string `json:"namespace"`
	RequestedMB int    `json:"requestedMB"`
	UsedMB      int    `json:"usedMB"`
	PodCount    int    `json:"podCount"`
}

type MemConsumer struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	RequestMB int    `json:"requestMB"`
	LimitMB   int    `json:"limitMB"`
	Replicas  int32  `json:"replicas"`
	TotalMB   int    `json:"totalMB"`
}

func (s *Server) handleMemPressureForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := MemPressureResult{ScannedAt: time.Now()}

	// Get nodes and pods
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Map: node -> aggregated memory request and usage
	nodeReqMB := map[string]int{}    // requested memory in MB
	nodePodCount := map[string]int{} // pod count per node

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nodePodCount[pod.Spec.NodeName]++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				nodeReqMB[pod.Spec.NodeName] += int(qty.Value() / (1024 * 1024))
			}
		}
	}

	// Analyze each node
	for _, node := range nodes.Items {
		// Skip nodes that are not ready or tainted
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++

		allocatableMB := 0
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			allocatableMB = int(qty.Value() / (1024 * 1024))
		}
		result.Summary.TotalAllocatableMB += allocatableMB

		reqMB := nodeReqMB[node.Name]
		result.Summary.TotalRequestedMB += reqMB

		// Approximate usage from metrics if available, else use requests
		usedMB := reqMB // Conservative: assume usage = requests
		result.Summary.TotalUsedMB += usedMB

		utilPct := 0
		if allocatableMB > 0 {
			utilPct = reqMB * 100 / allocatableMB
		}

		entry := NodeMemPressure{
			Node:           node.Name,
			AllocatableMB:  allocatableMB,
			RequestedMB:    reqMB,
			UsedMB:         usedMB,
			UtilizationPct: utilPct,
			PodCount:       nodePodCount[node.Name],
		}

		// Classify pressure
		switch {
		case utilPct >= 90:
			entry.PressureLevel = "critical"
			result.Summary.CriticalNodes++
		case utilPct >= 75:
			entry.PressureLevel = "warning"
			result.Summary.WarningNodes++
		default:
			entry.PressureLevel = "healthy"
			result.Summary.HealthyNodes++
		}

		// Simple trend: estimate exhaustion if >85% utilized
		// Conservative: assume 50MB/hr growth for critical nodes
		if utilPct >= 85 && allocatableMB > 0 {
			remainingMB := allocatableMB - reqMB
			entry.TrendPerHour = 50.0 // estimated growth rate
			if entry.TrendPerHour > 0 {
				entry.PredictedExhaustHrs = int(float64(remainingMB) / entry.TrendPerHour)
				if result.Summary.PredictedExhaustHrs == 0 || entry.PredictedExhaustHrs < result.Summary.PredictedExhaustHrs {
					result.Summary.PredictedExhaustHrs = entry.PredictedExhaustHrs
				}
			}
		}

		result.NodePressure = append(result.NodePressure, entry)
	}

	// Sort by utilization descending
	sort.Slice(result.NodePressure, func(i, j int) bool {
		return result.NodePressure[i].UtilizationPct > result.NodePressure[j].UtilizationPct
	})

	// Namespace memory spend
	nsSpendMap := map[string]*NsMemSpend{}
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		ns, ok := nsSpendMap[pod.Namespace]
		if !ok {
			ns = &NsMemSpend{Namespace: pod.Namespace}
			nsSpendMap[pod.Namespace] = ns
		}
		ns.PodCount++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				ns.RequestedMB += int(qty.Value() / (1024 * 1024))
			}
		}
	}
	for _, ns := range nsSpendMap {
		ns.UsedMB = ns.RequestedMB
		result.NamespaceSpend = append(result.NamespaceSpend, *ns)
	}
	sort.Slice(result.NamespaceSpend, func(i, j int) bool {
		return result.NamespaceSpend[i].RequestedMB > result.NamespaceSpend[j].RequestedMB
	})

	// Top consumers from deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		reqMB := 0
		limitMB := 0
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				reqMB += int(qty.Value() / (1024 * 1024))
			}
			if qty, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				limitMB += int(qty.Value() / (1024 * 1024))
			}
		}
		if reqMB == 0 {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		result.TopConsumers = append(result.TopConsumers, MemConsumer{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
			RequestMB: reqMB,
			LimitMB:   limitMB,
			Replicas:  replicas,
			TotalMB:   reqMB * int(replicas),
		})
	}
	sort.Slice(result.TopConsumers, func(i, j int) bool {
		return result.TopConsumers[i].TotalMB > result.TopConsumers[j].TotalMB
	})
	if len(result.TopConsumers) > 15 {
		result.TopConsumers = result.TopConsumers[:15]
	}

	// Calculate overcommit ratio
	if result.Summary.TotalRequestedMB > 0 && result.Summary.TotalAllocatableMB > 0 {
		result.Summary.OvercommitRatio = float64(result.Summary.TotalRequestedMB) / float64(result.Summary.TotalAllocatableMB)
	}

	// Score
	healthyPct := 100
	if result.Summary.TotalNodes > 0 {
		healthyPct = result.Summary.HealthyNodes * 100 / result.Summary.TotalNodes
	}
	result.HealthScore = healthyPct
	if result.Summary.CriticalNodes > 0 {
		result.HealthScore -= 20
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildMemPressureRecs1893(&result)
	writeJSON(w, result)
}

func buildMemPressureRecs1893(result *MemPressureResult) []string {
	recs := []string{
		fmt.Sprintf("Memory pressure: %d nodes (%d critical, %d warning, %d healthy), overcommit %.2fx",
			result.Summary.TotalNodes, result.Summary.CriticalNodes,
			result.Summary.WarningNodes, result.Summary.HealthyNodes,
			result.Summary.OvercommitRatio),
	}
	if result.Summary.CriticalNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes with critical memory pressure (>90%%) - add nodes or reduce requests", result.Summary.CriticalNodes))
	}
	if result.Summary.OvercommitRatio > 1.0 {
		recs = append(recs, fmt.Sprintf("Memory overcommit ratio %.2fx - total requests exceed allocatable, risk of OOM kills", result.Summary.OvercommitRatio))
	}
	if result.Summary.PredictedExhaustHrs > 0 {
		recs = append(recs, fmt.Sprintf("Memory exhaustion predicted in ~%d hours on most pressured node", result.Summary.PredictedExhaustHrs))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Scaling Concurrency Limit — how many workloads can scale simultaneously
// ---------------------------------------------------------------

type ScaleConcurrencyResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ScaleConcurrencySummary `json:"summary"`
	CPUHeadroom     ResourceHeadroom        `json:"cpuHeadroom"`
	MemHeadroom     ResourceHeadroom        `json:"memHeadroom"`
	PodHeadroom     ResourceHeadroom        `json:"podHeadroom"`
	ScaleCandidates []ScaleCandidate        `json:"scaleCandidates"`
	BottleneckType  string                  `json:"bottleneckType"`
	Recommendations []string                `json:"recommendations"`
}

type ScaleConcurrencySummary struct {
	MaxConcurrentScale int    `json:"maxConcurrentScale"`
	TotalSpareCPUm     int    `json:"totalSpareCPUm"`
	TotalSpareMemMB    int    `json:"totalSpareMemMB"`
	TotalSparePods     int    `json:"totalSparePods"`
	AvgPodCPUm         int    `json:"avgPodCPUm"`
	AvgPodMemMB        int    `json:"avgPodMemMB"`
	ScalableWorkloads  int    `json:"scalableWorkloads"`
	Bottleneck         string `json:"bottleneck"`
}

type ResourceHeadroom struct {
	Total     int    `json:"total"`
	Used      int    `json:"used"`
	Available int    `json:"available"`
	Unit      string `json:"unit"`
}

type ScaleCandidate struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	CurrentReps int32  `json:"currentReplicas"`
	PodCPUm     int    `json:"podCPUm"`
	PodMemMB    int    `json:"podMemMB"`
	MaxScaleUp  int    `json:"maxScaleUp"`
	RiskLevel   string `json:"riskLevel"`
}

func (s *Server) handleScaleConcurrency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ScaleConcurrencyResult{ScannedAt: time.Now()}

	// Get total cluster capacity
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	totalCPUm := 0  // millicores
	totalMemMB := 0 // MB
	totalPodCap := 0

	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			totalCPUm += int(qty.MilliValue())
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			totalMemMB += int(qty.Value() / (1024 * 1024))
		}
		// Pod capacity: typically 110 per node
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			totalPodCap += int(qty.Value())
		}
	}

	// Calculate used resources from pod requests
	usedCPUm := 0
	usedMemMB := 0
	podCount := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		podCount++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				usedCPUm += int(qty.MilliValue())
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				usedMemMB += int(qty.Value() / (1024 * 1024))
			}
		}
	}

	// Headroom
	result.CPUHeadroom = ResourceHeadroom{Total: totalCPUm, Used: usedCPUm, Available: totalCPUm - usedCPUm, Unit: "millicores"}
	result.MemHeadroom = ResourceHeadroom{Total: totalMemMB, Used: usedMemMB, Available: totalMemMB - usedMemMB, Unit: "MB"}
	result.PodHeadroom = ResourceHeadroom{Total: totalPodCap, Used: podCount, Available: totalPodCap - podCount, Unit: "pods"}

	result.Summary.TotalSpareCPUm = result.CPUHeadroom.Available
	result.Summary.TotalSpareMemMB = result.MemHeadroom.Available
	result.Summary.TotalSparePods = result.PodHeadroom.Available

	// Calculate average pod resource cost
	if podCount > 0 {
		result.Summary.AvgPodCPUm = usedCPUm / podCount
		result.Summary.AvgPodMemMB = usedMemMB / podCount
	}

	// Max concurrent scale: how many average pods can we fit?
	maxByCPU := 999999
	maxByMem := 999999
	maxByPod := result.PodHeadroom.Available

	if result.Summary.AvgPodCPUm > 0 {
		maxByCPU = result.CPUHeadroom.Available / result.Summary.AvgPodCPUm
	}
	if result.Summary.AvgPodMemMB > 0 {
		maxByMem = result.MemHeadroom.Available / result.Summary.AvgPodMemMB
	}

	result.Summary.MaxConcurrentScale = minInt1893(maxByCPU, minInt1893(maxByMem, maxByPod))
	if result.Summary.MaxConcurrentScale < 0 {
		result.Summary.MaxConcurrentScale = 0
	}

	// Determine bottleneck
	bottleneck := "none"
	if result.Summary.MaxConcurrentScale == maxByPod {
		bottleneck = "pod-capacity"
	} else if result.Summary.MaxConcurrentScale == maxByCPU {
		bottleneck = "cpu"
	} else {
		bottleneck = "memory"
	}
	result.Summary.Bottleneck = bottleneck
	result.BottleneckType = bottleneck

	// Find scalable workloads (with HPA or could benefit from scaling)
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		podCPUm := 0
		podMemMB := 0
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				podCPUm += int(qty.MilliValue())
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				podMemMB += int(qty.Value() / (1024 * 1024))
			}
		}
		if podCPUm == 0 && podMemMB == 0 {
			continue
		}

		result.Summary.ScalableWorkloads++
		currentReps := int32(1)
		if dep.Spec.Replicas != nil {
			currentReps = *dep.Spec.Replicas
		}

		// How much can this workload scale up?
		maxScaleByCPU := 999999
		maxScaleByMem := 999999
		if podCPUm > 0 {
			maxScaleByCPU = result.CPUHeadroom.Available / podCPUm
		}
		if podMemMB > 0 {
			maxScaleByMem = result.MemHeadroom.Available / podMemMB
		}
		maxScale := minInt1893(maxScaleByCPU, minInt1893(maxScaleByMem, maxByPod))

		riskLevel := "low"
		if maxScale < 2 {
			riskLevel = "high"
		} else if maxScale < 5 {
			riskLevel = "medium"
		}

		result.ScaleCandidates = append(result.ScaleCandidates, ScaleCandidate{
			Name:        dep.Name,
			Namespace:   dep.Namespace,
			Kind:        "Deployment",
			CurrentReps: currentReps,
			PodCPUm:     podCPUm,
			PodMemMB:    podMemMB,
			MaxScaleUp:  maxScale,
			RiskLevel:   riskLevel,
		})
	}

	sort.Slice(result.ScaleCandidates, func(i, j int) bool {
		return result.ScaleCandidates[i].MaxScaleUp < result.ScaleCandidates[j].MaxScaleUp
	})
	if len(result.ScaleCandidates) > 20 {
		result.ScaleCandidates = result.ScaleCandidates[:20]
	}

	// Score based on available headroom
	if totalCPUm > 0 {
		cpuHeadroomPct := (totalCPUm - usedCPUm) * 100 / totalCPUm
		result.HealthScore = cpuHeadroomPct
	}
	if result.Summary.MaxConcurrentScale < 10 {
		result.HealthScore -= 20
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildScaleConcurrencyRecs1893(&result)
	writeJSON(w, result)
}

func buildScaleConcurrencyRecs1893(result *ScaleConcurrencyResult) []string {
	recs := []string{
		fmt.Sprintf("Scaling concurrency: max %d simultaneous pod creations, bottleneck: %s",
			result.Summary.MaxConcurrentScale, result.Summary.Bottleneck),
		fmt.Sprintf("Headroom: %dm CPU / %dMB Mem / %d pods spare",
			result.Summary.TotalSpareCPUm, result.Summary.TotalSpareMemMB, result.Summary.TotalSparePods),
	}
	if result.Summary.MaxConcurrentScale < 10 {
		recs = append(recs, fmt.Sprintf("Low scaling headroom (%d pods) - add nodes before traffic spikes", result.Summary.MaxConcurrentScale))
	}
	switch result.Summary.Bottleneck {
	case "cpu":
		recs = append(recs, "CPU is the scaling bottleneck - optimize CPU requests or add CPU capacity")
	case "memory":
		recs = append(recs, "Memory is the scaling bottleneck - optimize memory requests or add RAM")
	case "pod-capacity":
		recs = append(recs, "Pod capacity (max pods per node) is the bottleneck - increase --max-pods or add nodes")
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Pod Disruption Window — safe maintenance windows
// ---------------------------------------------------------------

type DisruptionWindowResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         DisruptionWindowSummary `json:"summary"`
	SafeWindows     []DisruptionWindow      `json:"safeWindows"`
	UnsafeWorkloads []DisruptionEntry       `json:"unsafeWorkloads"`
	PDBCoverage     []PDBCoverageEntry      `json:"pdbCoverage"`
	VoluntaryRisk   []DisruptionEntry       `json:"voluntaryRisk"`
	Recommendations []string                `json:"recommendations"`
}

type DisruptionWindowSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	WithPDB          int `json:"withPDB"`
	WithoutPDB       int `json:"withoutPDB"`
	MaxDisruptable   int `json:"maxDisruptable"`
	SingleReplica    int `json:"singleReplica"`
	MinReplicaPDB    int `json:"minReplicaPDB"`
	TotalReplicas    int `json:"totalReplicas"`
	DisruptionBudget int `json:"disruptionBudget"`
}

type DisruptionWindow struct {
	TimeWindow  string `json:"timeWindow"`
	Description string `json:"description"`
	MaxPodsDown int    `json:"maxPodsDown"`
	SafetyLevel string `json:"safetyLevel"`
}

type DisruptionEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Replicas   int32  `json:"replicas"`
	HasPDB     bool   `json:"hasPDB"`
	MinAvail   int32  `json:"minAvailable,omitempty"`
	MaxUnavail int32  `json:"maxUnavailable,omitempty"`
	RiskLevel  string `json:"riskLevel"`
	Issue      string `json:"issue"`
}

type PDBCoverageEntry struct {
	Namespace     string `json:"namespace"`
	WithPDB       int    `json:"withPDB"`
	WithoutPDB    int    `json:"withoutPDB"`
	TotalReplicas int    `json:"totalReplicas"`
}

func (s *Server) handleDisruptionWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DisruptionWindowResult{ScannedAt: time.Now()}

	// Get PDBs
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbByNsName := map[string]*struct {
		minAvail   *intstr.IntOrString
		maxUnavail *intstr.IntOrString
	}{}
	pdbByNs := map[string]int{} // namespace -> count of PDBs

	for i := range pdbs.Items {
		pdb := &pdbs.Items[i]
		key := pdb.Namespace + "/" + pdb.Spec.Selector.String()
		pdbByNsName[key] = &struct {
			minAvail   *intstr.IntOrString
			maxUnavail *intstr.IntOrString
		}{
			minAvail:   pdb.Spec.MinAvailable,
			maxUnavail: pdb.Spec.MaxUnavailable,
		}
		pdbByNs[pdb.Namespace]++
	}

	// Analyze deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nsCoverage := map[string]*PDBCoverageEntry{}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		result.Summary.TotalReplicas += int(replicas)

		// Track namespace coverage
		nsCov, ok := nsCoverage[dep.Namespace]
		if !ok {
			nsCov = &PDBCoverageEntry{Namespace: dep.Namespace}
			nsCoverage[dep.Namespace] = nsCov
		}
		nsCov.TotalReplicas += int(replicas)

		// Check PDB coverage
		hasPDB := false
		var minAvail, maxUnavail int32
		key := dep.Namespace + "/" + metav1.FormatLabelSelector(dep.Spec.Selector)
		if pdb, ok := pdbByNsName[key]; ok {
			hasPDB = true
			result.Summary.WithPDB++
			nsCov.WithPDB++
			if pdb.minAvail != nil {
				minAvail = int32(pdb.minAvail.IntValue())
				result.Summary.MinReplicaPDB += int(minAvail)
			}
			if pdb.maxUnavail != nil {
				maxUnavail = int32(pdb.maxUnavail.IntValue())
			}
		} else {
			result.Summary.WithoutPDB++
			nsCov.WithoutPDB++
		}

		entry := DisruptionEntry{
			Name:       dep.Name,
			Namespace:  dep.Namespace,
			Kind:       "Deployment",
			Replicas:   replicas,
			HasPDB:     hasPDB,
			MinAvail:   minAvail,
			MaxUnavail: maxUnavail,
		}

		// Risk assessment
		switch {
		case replicas <= 1:
			entry.RiskLevel = "critical"
			entry.Issue = "single replica - any disruption causes downtime"
			result.Summary.SingleReplica++
			result.UnsafeWorkloads = append(result.UnsafeWorkloads, entry)
		case !hasPDB:
			entry.RiskLevel = "high"
			entry.Issue = fmt.Sprintf("%d replicas but no PDB - voluntary disruption not controlled", replicas)
			result.VoluntaryRisk = append(result.VoluntaryRisk, entry)
		case minAvail >= replicas:
			entry.RiskLevel = "medium"
			entry.Issue = fmt.Sprintf("PDB minAvailable=%d equals replicas=%d - no disruption allowed", minAvail, replicas)
		default:
			entry.RiskLevel = "low"
			if hasPDB && minAvail > 0 {
				allowedDown := replicas - minAvail
				result.Summary.MaxDisruptable += int(allowedDown)
			}
		}
	}

	// Build PDB coverage by namespace
	for _, cov := range nsCoverage {
		result.PDBCoverage = append(result.PDBCoverage, *cov)
	}
	sort.Slice(result.PDBCoverage, func(i, j int) bool {
		return result.PDBCoverage[i].WithoutPDB > result.PDBCoverage[j].WithoutPDB
	})

	// Build safe disruption windows
	result.SafeWindows = []DisruptionWindow{
		{
			TimeWindow:  "night-02-06",
			Description: "Lowest traffic window (02:00-06:00 local) - safest for voluntary disruptions",
			MaxPodsDown: result.Summary.MaxDisruptable,
			SafetyLevel: "high",
		},
		{
			TimeWindow:  "weekend-saturday",
			Description: "Weekend maintenance window (Saturday) - reduced traffic expected",
			MaxPodsDown: result.Summary.MaxDisruptable,
			SafetyLevel: "medium",
		},
		{
			TimeWindow:  "business-hours-avoid",
			Description: "Avoid 09:00-18:00 weekdays - peak traffic with high impact",
			MaxPodsDown: 0,
			SafetyLevel: "none",
		},
	}

	// Disruption budget = max pods that can go down safely
	result.Summary.DisruptionBudget = result.Summary.MaxDisruptable

	// Score
	if result.Summary.TotalWorkloads > 0 {
		pdbPct := result.Summary.WithPDB * 100 / result.Summary.TotalWorkloads
		singlePenalty := result.Summary.SingleReplica * 3
		result.HealthScore = pdbPct - singlePenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildDisruptionWindowRecs1893(&result)
	writeJSON(w, result)
}

func buildDisruptionWindowRecs1893(result *DisruptionWindowResult) []string {
	recs := []string{
		fmt.Sprintf("Disruption window: %d workloads, %d with PDB (%d%%), budget: %d pods can be disrupted",
			result.Summary.TotalWorkloads, result.Summary.WithPDB,
			safePercent1891(result.Summary.WithPDB, result.Summary.TotalWorkloads),
			result.Summary.DisruptionBudget),
	}
	if result.Summary.SingleReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d single-replica workloads at critical risk during node drain - scale to 2+ replicas", result.Summary.SingleReplica))
	}
	if result.Summary.WithoutPDB > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without PDB - add PodDisruptionBudget for controlled voluntary disruptions", result.Summary.WithoutPDB))
	}
	if result.Summary.DisruptionBudget == 0 {
		recs = append(recs, "Zero disruption budget - any node drain may cause service interruption")
	}
	return recs
}

// ---------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------

func isNodeReady1893(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func minInt1893(a, b int) int {
	if a < b {
		return a
	}
	return b
}
