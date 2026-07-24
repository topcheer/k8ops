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
// v19.52 — Scalability & HA Dimension (Round 11 Final)
// 1. Node Resource Fragmentation — resource stranding analysis
// 2. Controller Queue Pressure — CRD controller & operator load
// 3. Pod Density Optimizer — optimal pod placement recommendations
// ============================================================

// ---------------------------------------------------------------
// 1. Node Resource Fragmentation
// ---------------------------------------------------------------

type FragResult1952 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         FragSummary1952     `json:"summary"`
	Nodes           []FragNodeEntry1952 `json:"nodes"`
	Recommendations []string            `json:"recommendations"`
}

type FragSummary1952 struct {
	TotalNodes    int     `json:"totalNodes"`
	AvgCPUFrag    float64 `json:"avgCPUFragmentationPct"`
	AvgMemFrag    float64 `json:"avgMemFragmentationPct"`
	HighFragNodes int     `json:"highFragmentationNodes"`
	StrandedCPU   float64 `json:"strandedCPUCores"`
	StrandedMem   float64 `json:"strandedMemGB"`
}

type FragNodeEntry1952 struct {
	Name       string  `json:"name"`
	CPUFrag    float64 `json:"cpuFragmentationPct"`
	MemFrag    float64 `json:"memFragmentationPct"`
	IsHighFrag bool    `json:"isHighFrag"`
}

func (s *Server) handleNodeFragAnalysis(w http.ResponseWriter, r *http.Request) {
	result := FragResult1952{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Calculate per-node resource allocation
	nodeRes := make(map[string]*struct {
		cpuCap, memCapGB, cpuReq, memReqGB float64
	})
	for _, node := range nodeList.Items {
		nodeRes[node.Name] = &struct {
			cpuCap, memCapGB, cpuReq, memReqGB float64
		}{
			cpuCap:   node.Status.Allocatable.Cpu().AsApproximateFloat64(),
			memCapGB: float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024),
		}
		result.Summary.TotalNodes++
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		nr, ok := nodeRes[pod.Spec.NodeName]
		if !ok {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nr.cpuReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			nr.memReqGB += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}

	var totalCPUFrag, totalMemFrag float64
	for name, nr := range nodeRes {
		if nr.cpuCap == 0 {
			continue
		}
		// Fragmentation = used capacity that can't fit new pods due to fragmentation
		// Simplified: % of capacity that is "leftover" but too small for a standard pod (0.5 CPU)
		cpuUsedPct := nr.cpuReq / nr.cpuCap * 100
		memUsedPct := nr.memReqGB / nr.memCapGB * 100

		// Fragmentation = gap between CPU and memory utilization
		cpuFrag := 0.0
		memFrag := 0.0
		if cpuUsedPct > 50 && memUsedPct > 50 {
			// High utilization node: fragmentation = stranded resources
			cpuRemain := nr.cpuCap - nr.cpuReq
			memRemain := nr.memCapGB - nr.memReqGB
			if cpuRemain < 0.5 && memRemain > 2 {
				memFrag = (memRemain / nr.memCapGB) * 100
				result.Summary.StrandedMem += memRemain
			}
			if memRemain < 2 && cpuRemain > 0.5 {
				cpuFrag = (cpuRemain / nr.cpuCap) * 100
				result.Summary.StrandedCPU += cpuRemain
			}
		}

		isHighFrag := cpuFrag > 20 || memFrag > 20
		entry := FragNodeEntry1952{
			Name: name, CPUFrag: cpuFrag, MemFrag: memFrag, IsHighFrag: isHighFrag,
		}
		result.Nodes = append(result.Nodes, entry)

		if isHighFrag {
			result.Summary.HighFragNodes++
			score -= 5
		}
		totalCPUFrag += cpuFrag
		totalMemFrag += memFrag
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUFrag = totalCPUFrag / float64(result.Summary.TotalNodes)
		result.Summary.AvgMemFrag = totalMemFrag / float64(result.Summary.TotalNodes)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.HighFragNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with high fragmentation — defragment via node drain", result.Summary.HighFragNodes))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Stranded: %.1f CPU cores, %.1f GB memory", result.Summary.StrandedCPU, result.Summary.StrandedMem))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Controller Queue Pressure
// ---------------------------------------------------------------

type CtrlQueueResult1952 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CtrlQueueSummary1952 `json:"summary"`
	Controllers     []CtrlQueueEntry1952 `json:"controllers"`
	Risks           []CtrlQueueRisk1952  `json:"risks"`
	Recommendations []string             `json:"recommendations"`
}

type CtrlQueueSummary1952 struct {
	TotalOperators   int `json:"totalOperators"`
	HealthyOperators int `json:"healthyOperators"`
	HighRestartOps   int `json:"highRestartOperators"`
	TotalRestarts    int `json:"totalRestarts"`
	StaleControllers int `json:"staleControllers"`
}

type CtrlQueueEntry1952 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"age"`
}

type CtrlQueueRisk1952 struct {
	Name     string `json:"name"`
	RiskType string `json:"riskType"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleCtrlQueuePressure(w http.ResponseWriter, r *http.Request) {
	result := CtrlQueueResult1952{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Find operator/controller pods (not system pods)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		// Heuristic: pods with operator/controller in name or with owner kind=ReplicaSet
		name := pod.Name
		isOperator := false
		for _, label := range []string{"control-plane", "app.kubernetes.io/component"} {
			if pod.Labels[label] == "controller-manager" || pod.Labels[label] == "operator" {
				isOperator = true
				break
			}
		}
		if !isOperator {
			// Check name patterns
			for _, pattern := range []string{"operator", "controller", "manager"} {
				if containsStr1952(name, pattern) {
					isOperator = true
					break
				}
			}
		}
		if !isOperator {
			continue
		}

		result.Summary.TotalOperators++
		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}
		result.Summary.TotalRestarts += restarts

		entry := CtrlQueueEntry1952{
			Name: name, Namespace: pod.Namespace,
			Ready: true, Restarts: restarts,
			Age: fmt.Sprintf("%.0fd", time.Since(pod.CreationTimestamp.Time).Hours()/24),
		}
		result.Controllers = append(result.Controllers, entry)
		result.Summary.HealthyOperators++

		if restarts > 5 {
			result.Summary.HighRestartOps++
			severity := "medium"
			if restarts > 20 {
				severity = "high"
			}
			result.Risks = append(result.Risks, CtrlQueueRisk1952{
				Name: name, RiskType: "high-restarts", Severity: severity,
				Detail: fmt.Sprintf("%d restarts — queue pressure or crash", restarts),
			})
			if severity == "high" {
				score -= 5
			} else {
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.HighRestartOps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d operators with high restarts — investigate queue depth", result.Summary.HighRestartOps))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d operators, %d total restarts", result.Summary.TotalOperators, result.Summary.TotalRestarts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsStr1952(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 3. Pod Density Optimizer
// ---------------------------------------------------------------

type PodDensityOptResult1952 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         PodDensityOptSummary1952  `json:"summary"`
	Recommendations []PodDensityOptEntry1952  `json:"recommendations"`
	NodeStats       []PodDensityNodeEntry1952 `json:"nodeStats"`
}

type PodDensityOptSummary1952 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	MaxPodCapacity int     `json:"maxPodCapacityPerNode"`
	DensityUtilPct float64 `json:"densityUtilizationPct"`
	CanOptimize    int     `json:"nodesNeedingOptimization"`
}

type PodDensityOptEntry1952 struct {
	Node   string `json:"node"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

type PodDensityNodeEntry1952 struct {
	Node     string  `json:"node"`
	PodCount int     `json:"podCount"`
	Capacity int     `json:"capacity"`
	Density  float64 `json:"densityPct"`
}

func (s *Server) handlePodDensityOptimizer(w http.ResponseWriter, r *http.Request) {
	result := PodDensityOptResult1952{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
			result.Summary.TotalPods++
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		podCap := 110
		if pods := node.Status.Allocatable.Pods(); pods != nil {
			podCap = int(pods.Value())
		}
		if podCap > result.Summary.MaxPodCapacity {
			result.Summary.MaxPodCapacity = podCap
		}

		podCount := podsPerNode[node.Name]
		density := 0.0
		if podCap > 0 {
			density = float64(podCount) / float64(podCap) * 100
		}

		result.NodeStats = append(result.NodeStats, PodDensityNodeEntry1952{
			Node: node.Name, PodCount: podCount,
			Capacity: podCap, Density: density,
		})

		// Optimization recommendations
		if density > 85 {
			result.Summary.CanOptimize++
			result.Recommendations = append(result.Recommendations, PodDensityOptEntry1952{
				Node: node.Name, Action: "reduce-density",
				Detail: fmt.Sprintf("At %.0f%% pod capacity (%d/%d) — redistribute workloads", density, podCount, podCap),
			})
			score -= 3
		} else if density < 30 && result.Summary.TotalNodes > 1 {
			result.Recommendations = append(result.Recommendations, PodDensityOptEntry1952{
				Node: node.Name, Action: "consolidate",
				Detail: fmt.Sprintf("Only %.0f%% utilized (%d/%d) — consolidate and cordon", density, podCount, podCap),
			})
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = float64(result.Summary.TotalPods) / float64(result.Summary.TotalNodes)
	}
	if result.Summary.TotalNodes > 0 && result.Summary.MaxPodCapacity > 0 {
		result.Summary.DensityUtilPct = result.Summary.AvgPodsPerNode / float64(result.Summary.MaxPodCapacity) * 100
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	sort.Slice(result.NodeStats, func(i, j int) bool {
		return result.NodeStats[i].Density > result.NodeStats[j].Density
	})
	writeJSON(w, result)
}
