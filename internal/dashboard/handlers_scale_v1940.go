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
// v19.40 — Scalability & HA Dimension (Round 9 Final)
// 1. Resource Pressure Score — node resource contention analysis
// 2. Workload Anti-Affinity Coverage — pod co-location risk
// 3. Pod Startup Latency — scheduling-to-running time analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Resource Pressure Score
// ---------------------------------------------------------------

type ResPressureResult1940 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ResPressureSummary1940   `json:"summary"`
	Nodes           []ResPressureNode1940    `json:"nodes"`
	HighPressureNS  []ResPressureNSEntry1940 `json:"highPressureNamespaces"`
	Recommendations []string                 `json:"recommendations"`
}

type ResPressureSummary1940 struct {
	TotalNodes        int     `json:"totalNodes"`
	AvgCPUPressure    float64 `json:"avgCPUPressurePct"`
	AvgMemPressure    float64 `json:"avgMemPressurePct"`
	AvgPodPressure    float64 `json:"avgPodPressurePct"`
	HighPressureNodes int     `json:"highPressureNodes"`
	MaxCPUPressure    float64 `json:"maxCPUPressurePct"`
}

type ResPressureNode1940 struct {
	Name          string  `json:"name"`
	CPUUsage      float64 `json:"cpuUsagePct"`
	MemUsage      float64 `json:"memUsagePct"`
	PodUsage      float64 `json:"podUsagePct"`
	PressureScore int     `json:"pressureScore"`
	IsHigh        bool    `json:"isHighPressure"`
}

type ResPressureNSEntry1940 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequested"`
	MemReqMB  int     `json:"memRequestedMB"`
	PodCount  int     `json:"podCount"`
}

func (s *Server) handleResPressureScore(w http.ResponseWriter, r *http.Request) {
	result := ResPressureResult1940{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Per-node resource usage
	nodeRes := make(map[string]*struct {
		cpuAlloc, memAlloc, cpuReq, memReq float64
		podCount, podCap                   int
	})

	for _, node := range nodeList.Items {
		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memAlloc := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		podCap := 110
		if pcs := node.Status.Allocatable.Pods(); pcs != nil {
			podCap = int(pcs.Value())
		}
		nodeRes[node.Name] = &struct {
			cpuAlloc, memAlloc, cpuReq, memReq float64
			podCount, podCap                   int
		}{cpuAlloc: cpuAlloc, memAlloc: memAlloc, podCap: podCap}
		result.Summary.TotalNodes++
	}

	// Aggregate pod requests per node
	nsRes := make(map[string]*ResPressureNSEntry1940)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nr, ok := nodeRes[pod.Spec.NodeName]
		if ok {
			nr.podCount++
			for _, c := range pod.Spec.Containers {
				nr.cpuReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
				nr.memReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
			}
		}

		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if nsRes[pod.Namespace] == nil {
			nsRes[pod.Namespace] = &ResPressureNSEntry1940{Namespace: pod.Namespace}
		}
		nsRes[pod.Namespace].PodCount++
		for _, c := range pod.Spec.Containers {
			nsRes[pod.Namespace].CPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			nsRes[pod.Namespace].MemReqMB += int(c.Resources.Requests.Memory().Value() / (1024 * 1024))
		}
	}

	var totalCPU, totalMem, totalPod float64
	for _, node := range nodeList.Items {
		nr := nodeRes[node.Name]
		if nr == nil || nr.cpuAlloc == 0 {
			continue
		}
		cpuPct := nr.cpuReq / nr.cpuAlloc * 100
		memPct := nr.memReq / nr.memAlloc * 100
		podPct := float64(nr.podCount) / float64(nr.podCap) * 100

		// Pressure score: weighted average
		pressureScore := int(cpuPct*0.4 + memPct*0.3 + podPct*0.3)
		isHigh := pressureScore > 70

		entry := ResPressureNode1940{
			Name: node.Name, CPUUsage: cpuPct, MemUsage: memPct,
			PodUsage: podPct, PressureScore: pressureScore, IsHigh: isHigh,
		}
		result.Nodes = append(result.Nodes, entry)

		if isHigh {
			result.Summary.HighPressureNodes++
			score -= 10
		}

		totalCPU += cpuPct
		totalMem += memPct
		totalPod += podPct

		if cpuPct > result.Summary.MaxCPUPressure {
			result.Summary.MaxCPUPressure = cpuPct
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUPressure = totalCPU / float64(result.Summary.TotalNodes)
		result.Summary.AvgMemPressure = totalMem / float64(result.Summary.TotalNodes)
		result.Summary.AvgPodPressure = totalPod / float64(result.Summary.TotalNodes)
	}

	for _, ns := range nsRes {
		result.HighPressureNS = append(result.HighPressureNS, *ns)
	}
	sort.Slice(result.HighPressureNS, func(i, j int) bool {
		return result.HighPressureNS[i].CPUReq > result.HighPressureNS[j].CPUReq
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.HighPressureNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes under high pressure (>70%%) — redistribute or add capacity", result.Summary.HighPressureNodes))
	}
	if result.Summary.AvgPodPressure > 70 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Average pod density %.0f%% — approaching kubelet limits", result.Summary.AvgPodPressure))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Workload Anti-Affinity Coverage
// ---------------------------------------------------------------

type AntiAffinityResult1940 struct {
	ScannedAt        time.Time                   `json:"scannedAt"`
	HealthScore      int                         `json:"healthScore"`
	Grade            string                      `json:"grade"`
	Summary          AntiAffinitySummary1940     `json:"summary"`
	CoveredWorkloads []AntiAffinityEntry1940     `json:"coveredWorkloads"`
	UncoveredMulti   []AntiAffinityUncovered1940 `json:"uncoveredMultiReplica"`
	Recommendations  []string                    `json:"recommendations"`
}

type AntiAffinitySummary1940 struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	WithAntiAffinity   int `json:"withAntiAffinity"`
	WithPodAntiAff     int `json:"withPodAntiAffinity"`
	WithTopologySpread int `json:"withTopologySpread"`
	UncoveredMulti     int `json:"uncoveredMultiReplica"`
	CriticalGaps       int `json:"criticalGaps"`
}

type AntiAffinityEntry1940 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	Type      string `json:"type"`
}

type AntiAffinityUncovered1940 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	NodeCount int    `json:"nodeCount"`
	Severity  string `json:"severity"`
}

func (s *Server) handleAntiAffinityCoverage(w http.ResponseWriter, r *http.Request) {
	result := AntiAffinityResult1940{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Track node distribution per workload
	wlNodes := make(map[string]map[string]bool) // "ns/name" -> node set
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s", pod.Namespace, appName)
		if wlNodes[key] == nil {
			wlNodes[key] = make(map[string]bool)
		}
		wlNodes[key][pod.Spec.NodeName] = true
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}

		hasPodAntiAff := dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil
		hasTopoSpread := len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0

		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		nodeCount := len(wlNodes[key])

		if hasPodAntiAff {
			result.Summary.WithPodAntiAff++
			result.Summary.WithAntiAffinity++
			result.CoveredWorkloads = append(result.CoveredWorkloads, AntiAffinityEntry1940{
				Name: dep.Name, Namespace: dep.Namespace, Replicas: replicas, Type: "podAntiAffinity",
			})
		} else if hasTopoSpread {
			result.Summary.WithTopologySpread++
			result.Summary.WithAntiAffinity++
			result.CoveredWorkloads = append(result.CoveredWorkloads, AntiAffinityEntry1940{
				Name: dep.Name, Namespace: dep.Namespace, Replicas: replicas, Type: "topologySpread",
			})
		} else if replicas >= 2 {
			result.Summary.UncoveredMulti++
			severity := "medium"
			if replicas >= 3 && nodeCount == 1 {
				severity = "high"
			}
			if severity == "high" {
				result.Summary.CriticalGaps++
			}
			result.UncoveredMulti = append(result.UncoveredMulti, AntiAffinityUncovered1940{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas, NodeCount: nodeCount, Severity: severity,
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

	if result.Summary.UncoveredMulti > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d multi-replica workloads without anti-affinity — add for HA", result.Summary.UncoveredMulti))
	}
	if result.Summary.CriticalGaps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d critical gaps — 3+ replicas on single node", result.Summary.CriticalGaps))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Startup Latency
// ---------------------------------------------------------------

type StartupLatencyResult1940 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         StartupLatencySummary1940 `json:"summary"`
	Pods            []StartupLatencyEntry1940 `json:"pods"`
	SlowStarters    []StartupSlowEntry1940    `json:"slowStarters"`
	Recommendations []string                  `json:"recommendations"`
}

type StartupLatencySummary1940 struct {
	TotalPods     int     `json:"totalPods"`
	AvgLatencySec float64 `json:"avgLatencySec"`
	MaxLatencySec float64 `json:"maxLatencySec"`
	SlowCount     int     `json:"slowCount"`
	FailedStarts  int     `json:"failedStarts"`
	AvgImagePull  float64 `json:"avgImagePullSec"`
}

type StartupLatencyEntry1940 struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	LatencySec   float64 `json:"latencySec"`
	ImagePullSec float64 `json:"imagePullSec"`
}

type StartupSlowEntry1940 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	LatencySec float64 `json:"latencySec"`
	Reason     string  `json:"reason"`
}

func (s *Server) handleStartupLatencyV2(w http.ResponseWriter, r *http.Request) {
	result := StartupLatencyResult1940{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalLatency, totalPull float64
	var latencyCount int

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Calculate startup latency from conditions
		var scheduledTime, readyTime time.Time
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == "True" {
				scheduledTime = cond.LastTransitionTime.Time
			}
			if cond.Type == corev1.PodReady && cond.Status == "True" {
				readyTime = cond.LastTransitionTime.Time
			}
		}

		var latencySec float64
		if !scheduledTime.IsZero() && !readyTime.IsZero() && readyTime.After(scheduledTime) {
			latencySec = readyTime.Sub(scheduledTime).Seconds()
		} else if !pod.CreationTimestamp.IsZero() && !readyTime.IsZero() {
			latencySec = readyTime.Sub(pod.CreationTimestamp.Time).Seconds()
		}

		if latencySec <= 0 {
			continue
		}

		// Estimate image pull time from container statuses
		imagePullSec := 0.0
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Running != nil && !cs.State.Running.StartedAt.IsZero() {
				pullSec := cs.State.Running.StartedAt.Time.Sub(pod.CreationTimestamp.Time).Seconds()
				if pullSec > 0 && pullSec < latencySec {
					imagePullSec = pullSec
				}
			}
		}

		result.Summary.TotalPods++
		totalLatency += latencySec
		totalPull += imagePullSec
		latencyCount++

		if latencySec > result.Summary.MaxLatencySec {
			result.Summary.MaxLatencySec = latencySec
		}

		if len(result.Pods) < 100 {
			result.Pods = append(result.Pods, StartupLatencyEntry1940{
				Name: pod.Name, Namespace: pod.Namespace,
				LatencySec: latencySec, ImagePullSec: imagePullSec,
			})
		}

		if latencySec > 120 {
			result.Summary.SlowCount++
			reason := fmt.Sprintf("%.0fs startup — >2min", latencySec)
			if imagePullSec > 60 {
				reason += fmt.Sprintf(" (image pull %.0fs)", imagePullSec)
			}
			result.SlowStarters = append(result.SlowStarters, StartupSlowEntry1940{
				Name: pod.Name, Namespace: pod.Namespace,
				LatencySec: latencySec, Reason: reason,
			})
			score -= 2
		}
	}

	if latencyCount > 0 {
		result.Summary.AvgLatencySec = totalLatency / float64(latencyCount)
		result.Summary.AvgImagePull = totalPull / float64(latencyCount)
	}

	sort.Slice(result.Pods, func(i, j int) bool {
		return result.Pods[i].LatencySec > result.Pods[j].LatencySec
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SlowCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with >2min startup — optimize image size or init containers", result.Summary.SlowCount))
	}
	if result.Summary.AvgLatencySec > 60 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Average startup %.0fs — consider pre-pulling images", result.Summary.AvgLatencySec))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
