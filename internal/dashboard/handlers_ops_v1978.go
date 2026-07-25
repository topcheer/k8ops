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
// v19.78 — Operations Dimension (Round 16)
// 1. Exit Code Pattern — container termination reason classification
// 2. Pod QoS Class — Guaranteed/Burstable/BestEffort distribution
// 3. Namespace Resource Pressure — per-NS resource contention estimator
// ============================================================

// ---------------------------------------------------------------
// 1. Exit Code Pattern
// ---------------------------------------------------------------

type ExitCodeResult1978 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ExitCodeSummary1978 `json:"summary"`
	Patterns        []ExitCodeEntry1978 `json:"patterns"`
	Recommendations []string            `json:"recommendations"`
}

type ExitCodeSummary1978 struct {
	TotalContainers int `json:"totalContainers"`
	WithExitInfo    int `json:"withExitInfo"`
	OOMKilled       int `json:"oomKilled"`
	Exit0           int `json:"exit0"`
	ExitError       int `json:"exitError"`
	SignalKilled    int `json:"signalKilled"`
}

type ExitCodeEntry1978 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	ExitCode  int32  `json:"exitCode"`
	Reason    string `json:"reason"`
	Signal    int32  `json:"signal"`
}

func (s *Server) handleExitCodePattern(w http.ResponseWriter, r *http.Request) {
	result := ExitCodeResult1978{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++

			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				result.Summary.WithExitInfo++

				entry := ExitCodeEntry1978{
					Pod: pod.Name, Namespace: pod.Namespace, Container: cs.Name,
					ExitCode: term.ExitCode, Reason: term.Reason, Signal: term.Signal,
				}

				switch {
				case term.Reason == "OOMKilled":
					result.Summary.OOMKilled++
					score -= 5
				case term.ExitCode == 0:
					result.Summary.Exit0++
				case term.Signal > 0:
					result.Summary.SignalKilled++
					score -= 2
				default:
					result.Summary.ExitError++
					score -= 3
				}

				result.Patterns = append(result.Patterns, entry)
			}
		}
	}

	sort.Slice(result.Patterns, func(i, j int) bool {
		return result.Patterns[i].ExitCode > result.Patterns[j].ExitCode
	})
	if len(result.Patterns) > 30 {
		result.Patterns = result.Patterns[:30]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers, %d with exit info: %d OOM, %d error, %d signal", result.Summary.TotalContainers, result.Summary.WithExitInfo, result.Summary.OOMKilled, result.Summary.ExitError, result.Summary.SignalKilled))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod QoS Class Distribution
// ---------------------------------------------------------------

type PodQoSResult1978 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PodQoSSummary1978   `json:"summary"`
	PerNS           []PodQoSNSEntry1978 `json:"perNamespace"`
	Recommendations []string            `json:"recommendations"`
}

type PodQoSSummary1978 struct {
	TotalPods     int     `json:"totalPods"`
	Guaranteed    int     `json:"guaranteed"`
	Burstable     int     `json:"burstable"`
	BestEffort    int     `json:"bestEffort"`
	GuaranteedPct float64 `json:"guaranteedPct"`
	BestEffortPct float64 `json:"bestEffortPct"`
}

type PodQoSNSEntry1978 struct {
	Namespace  string `json:"namespace"`
	Guaranteed int    `json:"guaranteed"`
	Burstable  int    `json:"burstable"`
	BestEffort int    `json:"bestEffort"`
}

func (s *Server) handlePodQoSClass(w http.ResponseWriter, r *http.Request) {
	result := PodQoSResult1978{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*PodQoSNSEntry1978)

	for _, pod := range podList.Items {
		if pod.Status.QOSClass == "" {
			continue
		}
		result.Summary.TotalPods++

		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &PodQoSNSEntry1978{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}

		switch pod.Status.QOSClass {
		case corev1.PodQOSGuaranteed:
			result.Summary.Guaranteed++
			ns.Guaranteed++
		case corev1.PodQOSBurstable:
			result.Summary.Burstable++
			ns.Burstable++
		case corev1.PodQOSBestEffort:
			result.Summary.BestEffort++
			ns.BestEffort++
			score -= 1
		}
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.GuaranteedPct = float64(result.Summary.Guaranteed) / float64(result.Summary.TotalPods) * 100
		result.Summary.BestEffortPct = float64(result.Summary.BestEffort) / float64(result.Summary.TotalPods) * 100
	}

	for _, ns := range nsStats {
		result.PerNS = append(result.PerNS, *ns)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].BestEffort > result.PerNS[j].BestEffort
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d Guaranteed (%.0f%%), %d Burstable, %d BestEffort (%.0f%%)", result.Summary.TotalPods, result.Summary.Guaranteed, result.Summary.GuaranteedPct, result.Summary.Burstable, result.Summary.BestEffort, result.Summary.BestEffortPct))
	if result.Summary.BestEffortPct > 30 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%.0f%% BestEffort pods — add resource requests for scheduling reliability", result.Summary.BestEffortPct))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Resource Pressure
// ---------------------------------------------------------------

type NSPressureResult1978 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NSPressureSummary1978 `json:"summary"`
	PressureNS      []NSPressureEntry1978 `json:"pressureNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type NSPressureSummary1978 struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	HighPressure    int     `json:"highPressureNamespaces"`
	MediumPressure  int     `json:"mediumPressureNamespaces"`
	LowPressure     int     `json:"lowPressureNamespaces"`
	TotalCPUReq     float64 `json:"totalCPURequested"`
	TotalMemReq     float64 `json:"totalMemRequestedGB"`
}

type NSPressureEntry1978 struct {
	Namespace     string  `json:"namespace"`
	CPUReq        float64 `json:"cpuRequest"`
	MemReq        float64 `json:"memRequestGB"`
	PodCount      int     `json:"podCount"`
	PressureLevel string  `json:"pressureLevel"`
}

func (s *Server) handleNSResourcePressure(w http.ResponseWriter, r *http.Request) {
	result := NSPressureResult1978{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	// Total cluster capacity
	totalAllocCPU := 0.0
	totalAllocMem := 0.0
	for _, node := range nodeList.Items {
		totalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		totalAllocMem += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
	}

	nsStats := make(map[string]*NSPressureEntry1978)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &NSPressureEntry1978{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.PodCount++
		for _, c := range pod.Spec.Containers {
			ns.CPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			ns.MemReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
		result.Summary.TotalCPUReq += ns.CPUReq
		result.Summary.TotalMemReq += ns.MemReq
	}

	// Classify pressure: if a single NS uses >30% of cluster resources, it's high pressure
	for _, ns := range nsStats {
		cpuPct := 0.0
		memPct := 0.0
		if totalAllocCPU > 0 {
			cpuPct = ns.CPUReq / totalAllocCPU * 100
		}
		if totalAllocMem > 0 {
			memPct = ns.MemReq / totalAllocMem * 100
		}

		maxPct := cpuPct
		if memPct > maxPct {
			maxPct = memPct
		}

		if maxPct > 30 {
			ns.PressureLevel = "high"
			result.Summary.HighPressure++
		} else if maxPct > 10 {
			ns.PressureLevel = "medium"
			result.Summary.MediumPressure++
		} else {
			ns.PressureLevel = "low"
			result.Summary.LowPressure++
		}
		result.PressureNS = append(result.PressureNS, *ns)
	}
	result.Summary.TotalNamespaces = len(nsStats)

	sort.Slice(result.PressureNS, func(i, j int) bool {
		return result.PressureNS[i].CPUReq > result.PressureNS[j].CPUReq
	})

	if result.Summary.HighPressure > 0 {
		score -= result.Summary.HighPressure * 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces: %d high, %d medium, %d low pressure", result.Summary.TotalNamespaces, result.Summary.HighPressure, result.Summary.MediumPressure, result.Summary.LowPressure))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
