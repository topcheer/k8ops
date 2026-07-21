package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.10 — Scalability & HA Dimension (Round 4)
// 1. Burst Capacity Calculator
// 2. Resource Elasticity Index
// 3. Scale Bottleneck Detector
// ============================================================

// ---------------------------------------------------------------
// 1. Burst Capacity Calculator — how many pods can burst at once
// ---------------------------------------------------------------

type BurstCapacityResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         BurstCapacitySummary `json:"summary"`
	ByNode          []BurstNodeEntry     `json:"byNode"`
	Bottleneck      string               `json:"bottleneck"`
	ScalingCapacity map[string]int       `json:"scalingCapacity"`
	Recommendations []string             `json:"recommendations"`
}

type BurstCapacitySummary struct {
	TotalNodes       int `json:"totalNodes"`
	AvailCPUm        int `json:"availCPUm"`
	AvailMemMB       int `json:"availMemMB"`
	AvailPods        int `json:"availPods"`
	BurstPodsByCPU   int `json:"burstPodsByCPU"`
	BurstPodsByMem   int `json:"burstPodsByMem"`
	BurstPodsByLimit int `json:"burstPodsByLimit"`
	MaxBurstPods     int `json:"maxBurstPods"`
	AvgPodCPUm       int `json:"avgPodCPUm"`
	AvgPodMemMB      int `json:"avgPodMemMB"`
}

type BurstNodeEntry struct {
	Node       string `json:"node"`
	AvailCPUm  int    `json:"availCPUm"`
	AvailMemMB int    `json:"availMemMB"`
	AvailPods  int    `json:"availPods"`
	BurstPods  int    `json:"burstPods"`
}

func (s *Server) handleBurstCapacity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := BurstCapacityResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Calculate per-node used resources
	nodeUsedCPUm := map[string]int{}
	nodeUsedMemMB := map[string]int{}
	nodeUsedPods := map[string]int{}
	totalUsedCPUm, totalUsedMemMB, totalPodCount := 0, 0, 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nodeUsedPods[pod.Spec.NodeName]++
		totalPodCount++
		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue())
				nodeUsedCPUm[pod.Spec.NodeName] += m
				totalUsedCPUm += m
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mb := int(qty.Value() / (1024 * 1024))
				nodeUsedMemMB[pod.Spec.NodeName] += mb
				totalUsedMemMB += mb
			}
		}
	}

	// Calculate average pod resource footprint
	avgPodCPUm := 100  // default 100m
	avgPodMemMB := 128 // default 128MB
	if totalPodCount > 0 {
		avgPodCPUm = totalUsedCPUm / totalPodCount
		avgPodMemMB = totalUsedMemMB / totalPodCount
	}
	result.Summary.AvgPodCPUm = avgPodCPUm
	result.Summary.AvgPodMemMB = avgPodMemMB

	// Calculate burst capacity per node
	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++

		allocCPUm, allocMemMB, allocPods := 0, 0, 0
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			allocCPUm = int(qty.MilliValue())
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			allocMemMB = int(qty.Value() / (1024 * 1024))
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			allocPods = int(qty.Value())
		}

		availCPUm := allocCPUm - nodeUsedCPUm[node.Name]
		availMemMB := allocMemMB - nodeUsedMemMB[node.Name]
		availPods := allocPods - nodeUsedPods[node.Name]

		result.Summary.AvailCPUm += availCPUm
		result.Summary.AvailMemMB += availMemMB
		result.Summary.AvailPods += availPods

		burstByCPU := 0
		burstByMem := 0
		if avgPodCPUm > 0 {
			burstByCPU = availCPUm / avgPodCPUm
		}
		if avgPodMemMB > 0 {
			burstByMem = availMemMB / avgPodMemMB
		}
		nodeBurst := burstByCPU
		if burstByMem < nodeBurst {
			nodeBurst = burstByMem
		}
		if availPods < nodeBurst {
			nodeBurst = availPods
		}
		if nodeBurst < 0 {
			nodeBurst = 0
		}

		result.ByNode = append(result.ByNode, BurstNodeEntry{
			Node: node.Name, AvailCPUm: availCPUm,
			AvailMemMB: availMemMB, AvailPods: availPods,
			BurstPods: nodeBurst,
		})
	}

	// Global burst capacity
	result.Summary.BurstPodsByCPU = result.Summary.AvailCPUm / maxInt1910(avgPodCPUm, 1)
	result.Summary.BurstPodsByMem = result.Summary.AvailMemMB / maxInt1910(avgPodMemMB, 1)
	result.Summary.BurstPodsByLimit = result.Summary.AvailPods

	minBurst := result.Summary.BurstPodsByCPU
	if result.Summary.BurstPodsByMem < minBurst {
		minBurst = result.Summary.BurstPodsByMem
	}
	if result.Summary.BurstPodsByLimit < minBurst {
		minBurst = result.Summary.BurstPodsByLimit
	}
	result.Summary.MaxBurstPods = maxInt1910(minBurst, 0)

	// Identify bottleneck
	result.Bottleneck = "cpu"
	if result.Summary.BurstPodsByMem < result.Summary.BurstPodsByCPU {
		result.Bottleneck = "memory"
	}
	if result.Summary.BurstPodsByLimit < result.Summary.BurstPodsByMem && result.Summary.BurstPodsByLimit < result.Summary.BurstPodsByCPU {
		result.Bottleneck = "pod-limit"
	}

	result.ScalingCapacity = map[string]int{
		"byCPU":      result.Summary.BurstPodsByCPU,
		"byMemory":   result.Summary.BurstPodsByMem,
		"byPodLimit": result.Summary.BurstPodsByLimit,
	}

	// Score: higher burst capacity = better
	if result.Summary.MaxBurstPods >= 50 {
		result.HealthScore = 100
	} else {
		result.HealthScore = result.Summary.MaxBurstPods * 2
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildBurstCapacityRecs1910(&result)
	writeJSON(w, result)
}

func maxInt1910(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildBurstCapacityRecs1910(r *BurstCapacityResult) []string {
	recs := []string{fmt.Sprintf("Burst capacity: %d nodes, max %d burst pods (bottleneck: %s), avg pod %dm CPU / %dMB mem",
		r.Summary.TotalNodes, r.Summary.MaxBurstPods, r.Bottleneck,
		r.Summary.AvgPodCPUm, r.Summary.AvgPodMemMB)}
	if r.Summary.MaxBurstPods < 10 {
		recs = append(recs, fmt.Sprintf("Only %d burst pods possible - add nodes or reduce resource requests", r.Summary.MaxBurstPods))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Resource Elasticity Index — combined HPA+VPA+CA readiness
// ---------------------------------------------------------------

type ElasticityResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ElasticitySummary   `json:"summary"`
	ByWorkload      []ElasticityEntry   `json:"byWorkload"`
	Blockers        []ElasticityBlocker `json:"blockers"`
	Recommendations []string            `json:"recommendations"`
}

type ElasticitySummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	WithHPA         int `json:"withHPA"`
	WithVPA         int `json:"withVPA"`
	WithClusterAS   int `json:"withClusterAutoscaler"`
	FullyElastic    int `json:"fullyElastic"`
	NoElasticity    int `json:"noElasticity"`
	ElasticityIndex int `json:"elasticityIndex"`
}

type ElasticityEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HasHPA    bool   `json:"hasHPA"`
	HasVPA    bool   `json:"hasVPA"`
	Score     int    `json:"score"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

type ElasticityBlocker struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Blocker   string `json:"blocker"`
	Severity  string `json:"severity"`
}

func (s *Server) handleElasticityIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ElasticityResult{ScannedAt: time.Now()}

	// Check HPA coverage
	hpaMap := map[string]bool{}
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	for _, hpa := range hpas.Items {
		if !isSystemNamespace(hpa.Namespace) {
			hpaMap[hpa.Namespace+"/"+hpa.Name] = true
		}
	}

	// Check for VPA (vertical pod autoscaler) - look for vpa resources via dynamic client or skip
	// VPA is a CRD, we check if any workload references it via annotations
	vpaAnnotationMap := map[string]bool{}

	// Check cluster autoscaler presence (usually runs in kube-system)
	clusterASPresent := false
	caPods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	for _, pod := range caPods.Items {
		if strings.Contains(pod.Name, "cluster-autoscaler") {
			clusterASPresent = true
			break
		}
	}
	if clusterASPresent {
		result.Summary.WithClusterAS = 1
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		key := dep.Namespace + "/" + dep.Name
		entry := ElasticityEntry{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		// HPA check
		entry.HasHPA = hpaMap[key]
		if entry.HasHPA {
			result.Summary.WithHPA++
		}

		// VPA check (annotation-based detection)
		vpaAnnotationKey := strings.ToLower(dep.Annotations["vpa.k8s.io/enabled"])
		if vpaAnnotationKey == "true" || vpaAnnotationKey == "auto" {
			entry.HasVPA = true
			result.Summary.WithVPA++
			vpaAnnotationMap[key] = true
		}

		// Calculate elasticity score
		score := 0
		if entry.HasHPA {
			score += 40
		}
		if entry.HasVPA {
			score += 20
		}
		if clusterASPresent {
			score += 20
		}
		// Resource requests set = schedulable = +20
		hasRequests := false
		for _, c := range dep.Spec.Template.Spec.Containers {
			if _, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				hasRequests = true
				break
			}
		}
		if hasRequests {
			score += 20
		}
		entry.Score = score

		switch {
		case score >= 80:
			entry.RiskLevel = "low"
			result.Summary.FullyElastic++
		case score >= 40:
			entry.RiskLevel = "medium"
		case score == 0:
			entry.RiskLevel = "critical"
			entry.Issue = "no autoscaling configured - cannot respond to load changes"
			result.Summary.NoElasticity++
			result.Blockers = append(result.Blockers, ElasticityBlocker{
				Name: dep.Name, Namespace: dep.Namespace,
				Blocker:  "no HPA, VPA, or resource requests",
				Severity: "high",
			})
		default:
			entry.RiskLevel = "high"
			entry.Issue = "partial autoscaling - missing HPA"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].Score < result.ByWorkload[j].Score
	})

	// Elasticity index: percentage of fully elastic workloads
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.ElasticityIndex = result.Summary.FullyElastic * 100 / result.Summary.TotalWorkloads
		result.HealthScore = result.Summary.ElasticityIndex
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildElasticityRecs1910(&result)
	writeJSON(w, result)
}

func buildElasticityRecs1910(r *ElasticityResult) []string {
	recs := []string{fmt.Sprintf("Elasticity index: %d%% (%d/%d workloads fully elastic), %d with HPA, %d with VPA, CA: %v",
		r.Summary.ElasticityIndex, r.Summary.FullyElastic, r.Summary.TotalWorkloads,
		r.Summary.WithHPA, r.Summary.WithVPA, r.Summary.WithClusterAS > 0)}
	if r.Summary.NoElasticity > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with zero elasticity - add HPA and resource requests", r.Summary.NoElasticity))
	}
	if r.Summary.WithClusterAS == 0 {
		recs = append(recs, "Cluster Autoscaler not detected - install for automatic node scaling")
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Scale Bottleneck Detector — identify scaling constraints
// ---------------------------------------------------------------

type ScaleBottleneckResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         BottleneckSummary1910 `json:"summary"`
	Bottlenecks     []BottleneckEntry     `json:"bottlenecks"`
	ByWorkload      []BottleneckWLEntry   `json:"byWorkload"`
	Recommendations []string              `json:"recommendations"`
}

type BottleneckSummary1910 struct {
	TotalWorkloads       int `json:"totalWorkloads"`
	WithBottlenecks      int `json:"withBottlenecks"`
	CPUBottlenecks       int `json:"cpuBottlenecks"`
	MemBottlenecks       int `json:"memBottlenecks"`
	PodLimitBottlenecks  int `json:"podLimitBottlenecks"`
	AffinityBottlenecks  int `json:"affinityBottlenecks"`
	ImagePullBottlenecks int `json:"imagePullBottlenecks"`
}

type BottleneckEntry struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Count    int    `json:"count"`
}

type BottleneckWLEntry struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Replicas    int32    `json:"replicas"`
	Bottlenecks []string `json:"bottlenecks"`
	RiskLevel   string   `json:"riskLevel"`
}

func (s *Server) handleScaleBottleneck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ScaleBottleneckResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Node capacity
	totalAllocCPUm, totalAllocMemMB, totalAllocPods := 0, 0, 0
	nodeCount := 0
	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		nodeCount++
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			totalAllocCPUm += int(qty.MilliValue())
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			totalAllocMemMB += int(qty.Value() / (1024 * 1024))
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			totalAllocPods += int(qty.Value())
		}
	}

	// Check for pod density bottleneck
	usedPods := 0
	for _, pod := range pods.Items {
		if !isSystemNamespace(pod.Namespace) && pod.Spec.NodeName != "" {
			usedPods++
		}
	}
	podDensity := 0
	if totalAllocPods > 0 {
		podDensity = usedPods * 100 / totalAllocPods
	}
	if podDensity > 80 {
		result.Bottlenecks = append(result.Bottlenecks, BottleneckEntry{
			Type: "pod-density", Severity: "high",
			Detail: fmt.Sprintf("Pod density at %d%% (%d/%d) - approaching max pods per node", podDensity, usedPods, totalAllocPods),
			Count:  usedPods,
		})
		result.Summary.PodLimitBottlenecks++
	}

	// Check single-node bottleneck
	if nodeCount <= 1 {
		result.Bottlenecks = append(result.Bottlenecks, BottleneckEntry{
			Type: "single-node", Severity: "critical",
			Detail: "Single node cluster - no horizontal scaling possible",
			Count:  1,
		})
	}

	// Per-workload bottleneck analysis
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

		var bottlenecks []string
		riskLevel := "low"

		// CPU request bottleneck
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPUm := int(qty.MilliValue())
				if reqCPUm > totalAllocCPUm/int(maxInt1910(int(replicas), 1)) {
					bottlenecks = append(bottlenecks, "CPU request exceeds per-replica node capacity")
					riskLevel = "high"
					result.Summary.CPUBottlenecks++
				}
			}
			// Image size bottleneck (large images = slow pull)
			imageSize := c.Image
			if strings.Contains(imageSize, ":") && !strings.Contains(imageSize, ":latest") {
				// Has explicit tag - good
			} else {
				bottlenecks = append(bottlenecks, "using latest tag - image pull may be slow/unpredictable")
				if riskLevel == "low" {
					riskLevel = "medium"
				}
				result.Summary.ImagePullBottlenecks++
			}
		}

		// Affinity bottleneck
		if dep.Spec.Template.Spec.Affinity != nil {
			if dep.Spec.Template.Spec.Affinity.NodeAffinity != nil && dep.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				bottlenecks = append(bottlenecks, "hard affinity constraints limit scheduling flexibility")
				if riskLevel == "low" {
					riskLevel = "medium"
				}
				result.Summary.AffinityBottlenecks++
			}
		}

		// Readiness probe delay
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil && c.ReadinessProbe.InitialDelaySeconds > 30 {
				bottlenecks = append(bottlenecks, fmt.Sprintf("readiness probe delay %ds slows scaling", c.ReadinessProbe.InitialDelaySeconds))
				if riskLevel == "low" {
					riskLevel = "medium"
				}
			}
		}

		if len(bottlenecks) > 0 {
			result.Summary.WithBottlenecks++
			result.ByWorkload = append(result.ByWorkload, BottleneckWLEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas, Bottlenecks: bottlenecks, RiskLevel: riskLevel,
			})
		}
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		cleanPct := (result.Summary.TotalWorkloads - result.Summary.WithBottlenecks) * 100 / result.Summary.TotalWorkloads
		result.HealthScore = cleanPct
	} else {
		result.HealthScore = 100
	}
	if nodeCount <= 1 {
		result.HealthScore = result.HealthScore / 2
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildBottleneckRecs1910(&result)
	writeJSON(w, result)
}

func buildBottleneckRecs1910(r *ScaleBottleneckResult) []string {
	recs := []string{fmt.Sprintf("Scale bottlenecks: %d/%d workloads affected (%d CPU, %d affinity, %d image, %d pod-limit)",
		r.Summary.WithBottlenecks, r.Summary.TotalWorkloads,
		r.Summary.CPUBottlenecks, r.Summary.AffinityBottlenecks,
		r.Summary.ImagePullBottlenecks, r.Summary.PodLimitBottlenecks)}
	if r.Summary.AffinityBottlenecks > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with hard affinity constraints - use preferred affinity for flexibility", r.Summary.AffinityBottlenecks))
	}
	return recs
}
