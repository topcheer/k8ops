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
// v19.18 — Operations Dimension (Round 6)
// 1. Storage I/O Latency Risk
// 2. Network Packet Loss Risk
// 3. Cgroup Pressure Monitor
// ============================================================

// ---------------------------------------------------------------
// 1. Storage I/O Latency Risk — estimate storage performance risk
// ---------------------------------------------------------------

type StorageLatencyResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         StorageLatSummary `json:"summary"`
	ByNamespace     []StorageLatNS    `json:"byNamespace"`
	HighRiskPVCs    []StorageLatPVC   `json:"highRiskPVCs"`
	ByStorageClass  []StorageLatSC    `json:"byStorageClass"`
	Recommendations []string          `json:"recommendations"`
}

type StorageLatSummary struct {
	TotalPVCs        int `json:"totalPVCs"`
	TotalPVCSizeGB   int `json:"totalPVCSizeGB"`
	LocalStorageGB   int `json:"localStorageGB"`
	NetworkStorageGB int `json:"networkStorageGB"`
	HighRiskCount    int `json:"highRiskCount"`
	NoStorageClass   int `json:"noStorageClass"`
}

type StorageLatNS struct {
	Namespace string `json:"namespace"`
	PVCCount  int    `json:"pvcCount"`
	SizeGB    int    `json:"sizeGB"`
	RiskLevel string `json:"riskLevel"`
}

type StorageLatPVC struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	SizeGB       int    `json:"sizeGB"`
	StorageClass string `json:"storageClass"`
	IOPSEstimate int    `json:"iopsEstimate"`
	RiskLevel    string `json:"riskLevel"`
}

type StorageLatSC struct {
	Name        string `json:"name"`
	PVCCount    int    `json:"pvcCount"`
	TotalSizeGB int    `json:"totalSizeGB"`
	Type        string `json:"type"`
}

func (s *Server) handleStorageLatency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := StorageLatencyResult{ScannedAt: time.Now()}

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	scMap := map[string]string{}
	for _, sc := range scs.Items {
		scType := "network"
		prov := sc.Provisioner
		if prov == "kubernetes.io/no-provisioner" || prov == "local-path" ||
			prov == "rancher.io/local-path" || prov == "openebs.io/local" {
			scType = "local"
		}
		scMap[sc.Name] = scType
	}

	nsMap := map[string]*StorageLatNS{}
	scAgg := map[string]*StorageLatSC{}

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}
		result.Summary.TotalPVCSizeGB += sizeGB

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		scType := scMap[scName]
		if scType == "local" {
			result.Summary.LocalStorageGB += sizeGB
		} else {
			result.Summary.NetworkStorageGB += sizeGB
		}
		if scName == "" {
			result.Summary.NoStorageClass++
		}

		nsE, ok := nsMap[pvc.Namespace]
		if !ok {
			nsE = &StorageLatNS{Namespace: pvc.Namespace}
			nsMap[pvc.Namespace] = nsE
		}
		nsE.PVCCount++
		nsE.SizeGB += sizeGB

		// Risk: large PVC on network storage = higher latency
		riskLevel := "low"
		iopsEst := 0
		if scType == "network" && sizeGB > 100 {
			riskLevel = "high"
			iopsEst = 500
		} else if scType == "network" {
			riskLevel = "medium"
			iopsEst = 1000
		} else {
			iopsEst = 5000 // local storage = fast
		}
		if riskLevel == "high" {
			result.Summary.HighRiskCount++
			result.HighRiskPVCs = append(result.HighRiskPVCs, StorageLatPVC{
				Name: pvc.Name, Namespace: pvc.Namespace,
				SizeGB: sizeGB, StorageClass: scName,
				IOPSEstimate: iopsEst, RiskLevel: riskLevel,
			})
		}
		if nsE.RiskLevel == "" || riskLevel == "high" {
			nsE.RiskLevel = riskLevel
		}

		scE, ok := scAgg[scName]
		if !ok {
			scE = &StorageLatSC{Name: scName, Type: scType}
			scAgg[scName] = scE
		}
		scE.PVCCount++
		scE.TotalSizeGB += sizeGB
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	for _, sc := range scAgg {
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].PVCCount > result.ByStorageClass[j].PVCCount
	})

	// Score: fewer high-risk PVCs = better
	if result.Summary.TotalPVCs > 0 {
		safePct := (result.Summary.TotalPVCs - result.Summary.HighRiskCount) * 100 / result.Summary.TotalPVCs
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildStorageLatRecs1918(&result)
	writeJSON(w, result)
}

func buildStorageLatRecs1918(r *StorageLatencyResult) []string {
	recs := []string{fmt.Sprintf("Storage latency: %d PVCs (%dGB local, %dGB network), %d high-risk",
		r.Summary.TotalPVCs, r.Summary.LocalStorageGB, r.Summary.NetworkStorageGB, r.Summary.HighRiskCount)}
	if r.Summary.HighRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d PVCs on network storage >100GB - consider local SSDs for latency-sensitive workloads", r.Summary.HighRiskCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Network Packet Loss Risk
// ---------------------------------------------------------------

type PacketLossResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PacketLossSummary `json:"summary"`
	ByNode          []PacketLossNode  `json:"byNode"`
	HighRiskNS      []PacketLossNS    `json:"highRiskNS"`
	Recommendations []string          `json:"recommendations"`
}

type PacketLossSummary struct {
	TotalNodes    int `json:"totalNodes"`
	ReadyNodes    int `json:"readyNodes"`
	NetworkReady  int `json:"networkReady"`
	CNIHealth     int `json:"cniHealth"`
	PodsNotReady  int `json:"podsNotReady"`
	SvcWithoutEP  int `json:"servicesWithoutEndpoints"`
	HighRiskNodes int `json:"highRiskNodes"`
}

type PacketLossNode struct {
	Node      string `json:"node"`
	Ready     bool   `json:"ready"`
	PodCount  int    `json:"podCount"`
	Issues    int    `json:"issueCount"`
	RiskLevel string `json:"riskLevel"`
}

type PacketLossNS struct {
	Namespace string `json:"namespace"`
	Issues    int    `json:"issues"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handlePacketLossRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PacketLossResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Per-node pod count and readiness
	nodePods := map[string]int{}
	nodeIssues := map[string]int{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		nodePods[pod.Spec.NodeName]++
		isReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
			}
		}
		if !isReady && !isSystemNamespace(pod.Namespace) {
			result.Summary.PodsNotReady++
			nodeIssues[pod.Spec.NodeName]++
		}
	}

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		ready := isNodeReady1893(&node)
		if ready {
			result.Summary.ReadyNodes++
			result.Summary.NetworkReady++
		}

		issues := nodeIssues[node.Name]
		riskLevel := "low"
		if !ready {
			riskLevel = "critical"
			result.Summary.HighRiskNodes++
		} else if issues > 5 {
			riskLevel = "high"
			result.Summary.HighRiskNodes++
		} else if issues > 0 {
			riskLevel = "medium"
		}

		result.ByNode = append(result.ByNode, PacketLossNode{
			Node: node.Name, Ready: ready,
			PodCount: nodePods[node.Name],
			Issues:   issues, RiskLevel: riskLevel,
		})
	}

	// Services without endpoints
	nsIssues := map[string]int{}
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) || svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" {
			continue
		}
		// Check if endpoints exist via service age/status
		if svc.Spec.Selector == nil && svc.Spec.Type == corev1.ServiceTypeClusterIP {
			result.Summary.SvcWithoutEP++
			nsIssues[svc.Namespace]++
		}
	}

	for ns, issues := range nsIssues {
		if issues > 0 {
			riskLevel := "medium"
			if issues > 3 {
				riskLevel = "high"
			}
			result.HighRiskNS = append(result.HighRiskNS, PacketLossNS{
				Namespace: ns, Issues: issues, RiskLevel: riskLevel,
			})
		}
	}

	// Check CNI health (kube-proxy pods as proxy for network health)
	cniPods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	for _, pod := range cniPods.Items {
		isReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
			}
		}
		if isReady {
			result.Summary.CNIHealth++
		}
	}

	// Score
	if result.Summary.TotalNodes > 0 {
		healthyPct := (result.Summary.ReadyNodes - result.Summary.HighRiskNodes) * 100 / result.Summary.TotalNodes
		result.HealthScore = healthyPct
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildPacketLossRecs1918(&result)
	writeJSON(w, result)
}

func buildPacketLossRecs1918(r *PacketLossResult) []string {
	recs := []string{fmt.Sprintf("Network risk: %d nodes (%d ready), %d pods not ready, %d high-risk nodes, %d services without endpoints",
		r.Summary.TotalNodes, r.Summary.ReadyNodes, r.Summary.PodsNotReady,
		r.Summary.HighRiskNodes, r.Summary.SvcWithoutEP)}
	if r.Summary.PodsNotReady > 0 {
		recs = append(recs, fmt.Sprintf("%d pods not ready - check CNI plugin, kube-proxy, and node networking", r.Summary.PodsNotReady))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Cgroup Pressure Monitor
// ---------------------------------------------------------------

type CgroupPressureResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         CgroupPressureSummary `json:"summary"`
	ByNamespace     []CgroupPressureNS    `json:"byNamespace"`
	PressurePods    []CgroupPressurePod   `json:"pressurePods"`
	Recommendations []string              `json:"recommendations"`
}

type CgroupPressureSummary struct {
	TotalPods      int `json:"totalPods"`
	HighCPURequest int `json:"highCPURequestPods"`
	HighMemRequest int `json:"highMemRequestPods"`
	WithoutLimits  int `json:"withoutLimits"`
	OOMRiskCount   int `json:"oomRiskPods"`
	ThrottleRisk   int `json:"throttleRiskPods"`
}

type CgroupPressureNS struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	CPUm      int    `json:"cpuMilli"`
	MemMB     int    `json:"memMB"`
	RiskLevel string `json:"riskLevel"`
}

type CgroupPressurePod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CPUm      int    `json:"cpuMilli"`
	MemMB     int    `json:"memMB"`
	Issue     string `json:"issue"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleCgroupPressure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CgroupPressureResult{ScannedAt: time.Now()}

	nsMap := map[string]*CgroupPressureNS{}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		result.Summary.TotalPods++
		nsE, ok := nsMap[pod.Namespace]
		if !ok {
			nsE = &CgroupPressureNS{Namespace: pod.Namespace}
			nsMap[pod.Namespace] = nsE
		}
		nsE.PodCount++

		podCPUm := 0
		podMemMB := 0
		hasLimits := false

		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue())
				podCPUm += m
				nsE.CPUm += m
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mb := int(qty.Value() / (1024 * 1024))
				podMemMB += mb
				nsE.MemMB += mb
			}
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				hasLimits = true
			}
			if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				hasLimits = true
			}
		}

		if !hasLimits {
			result.Summary.WithoutLimits++
		}

		// High CPU request = throttle risk (>2 cores per pod)
		if podCPUm > 2000 {
			result.Summary.HighCPURequest++
			result.Summary.ThrottleRisk++
			result.PressurePods = append(result.PressurePods, CgroupPressurePod{
				Name: pod.Name, Namespace: pod.Namespace,
				CPUm: podCPUm, MemMB: podMemMB,
				Issue:     fmt.Sprintf("high CPU request %dm - CFS throttling risk", podCPUm),
				RiskLevel: "high",
			})
		}

		// High memory request = OOM risk (>4GB)
		if podMemMB > 4096 {
			result.Summary.HighMemRequest++
			result.Summary.OOMRiskCount++
			result.PressurePods = append(result.PressurePods, CgroupPressurePod{
				Name: pod.Name, Namespace: pod.Namespace,
				CPUm: podCPUm, MemMB: podMemMB,
				Issue:     fmt.Sprintf("high memory request %dMB - OOM kill risk under pressure", podMemMB),
				RiskLevel: "high",
			})
		}
	}

	for _, ns := range nsMap {
		if ns.CPUm > 4000 {
			ns.RiskLevel = "high"
		} else if ns.CPUm > 2000 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CPUm > result.ByNamespace[j].CPUm
	})

	// Score
	if result.Summary.TotalPods > 0 {
		safePct := (result.Summary.TotalPods - result.Summary.ThrottleRisk - result.Summary.OOMRiskCount) * 100 / result.Summary.TotalPods
		result.HealthScore = safePct
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildCgroupRecs1918(&result)
	writeJSON(w, result)
}

func buildCgroupRecs1918(r *CgroupPressureResult) []string {
	recs := []string{fmt.Sprintf("Cgroup pressure: %d pods, %d throttle risk, %d OOM risk, %d without limits",
		r.Summary.TotalPods, r.Summary.ThrottleRisk, r.Summary.OOMRiskCount, r.Summary.WithoutLimits)}
	if r.Summary.WithoutLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d pods without resource limits - risk of unbounded cgroup pressure", r.Summary.WithoutLimits))
	}
	if r.Summary.OOMRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pods with >4GB memory - monitor for OOM kills under memory pressure", r.Summary.OOMRiskCount))
	}
	return recs
}
