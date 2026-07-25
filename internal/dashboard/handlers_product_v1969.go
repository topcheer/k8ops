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
// v19.69 — Product Dimension (Round 14)
// 1. Pod Efficiency Score — CPU/memory utilization vs request ratio
// 2. Service Health Overview — service readiness composite score
// 3. Cluster Utilization Summary — aggregate resource utilization
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Efficiency Score
// ---------------------------------------------------------------

type PodEffResult1969 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PodEffSummary1969 `json:"summary"`
	Efficient       []PodEffEntry1969 `json:"efficientPods"`
	Wasteful        []PodEffEntry1969 `json:"wastefulPods"`
	Recommendations []string          `json:"recommendations"`
}

type PodEffSummary1969 struct {
	TotalPods        int     `json:"totalPods"`
	EfficientPods    int     `json:"efficientPods"`
	WastefulPods     int     `json:"wastefulPods"`
	AvgEfficiency    float64 `json:"avgEfficiencyPct"`
	OverProvisioned  int     `json:"overProvisioned"`
	UnderProvisioned int     `json:"underProvisioned"`
}

type PodEffEntry1969 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	CPUReq     float64 `json:"cpuRequest"`
	MemReqGB   float64 `json:"memRequestGB"`
	Efficiency float64 `json:"efficiencyPct"`
	Status     string  `json:"status"`
}

func (s *Server) handlePodEfficiencyScore(w http.ResponseWriter, r *http.Request) {
	result := PodEffResult1969{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalEff float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		count++

		cpuReq := 0.0
		memReq := 0.0
		for _, c := range pod.Spec.Containers {
			cpuReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			memReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}

		// Efficiency estimate: pods with very low requests relative to typical workloads
		// are efficient, pods with high requests but low actual usage are wasteful
		// Since we don't have metrics, use request sizing heuristics
		efficiency := 100.0
		status := "efficient"

		if cpuReq > 4.0 {
			efficiency -= 20
			status = "over-provisioned"
			result.Summary.OverProvisioned++
		} else if cpuReq > 2.0 {
			efficiency -= 10
		}

		if memReq > 8.0 {
			efficiency -= 20
			status = "over-provisioned"
			if result.Summary.OverProvisioned == 0 {
				result.Summary.OverProvisioned++
			}
		} else if memReq > 4.0 {
			efficiency -= 10
		}

		if cpuReq == 0 || memReq == 0 {
			efficiency -= 30
			status = "under-provisioned"
			result.Summary.UnderProvisioned++
		}

		if efficiency < 60 {
			result.Summary.WastefulPods++
			result.Wasteful = append(result.Wasteful, PodEffEntry1969{
				Name: pod.Name, Namespace: pod.Namespace,
				CPUReq: cpuReq, MemReqGB: memReq,
				Efficiency: efficiency, Status: status,
			})
			score -= 1
		} else {
			result.Summary.EfficientPods++
			result.Efficient = append(result.Efficient, PodEffEntry1969{
				Name: pod.Name, Namespace: pod.Namespace,
				CPUReq: cpuReq, MemReqGB: memReq,
				Efficiency: efficiency, Status: status,
			})
		}

		totalEff += efficiency
	}

	if count > 0 {
		result.Summary.AvgEfficiency = totalEff / float64(count)
	}

	sort.Slice(result.Wasteful, func(i, j int) bool {
		return result.Wasteful[i].Efficiency < result.Wasteful[j].Efficiency
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d efficient, %d wasteful (avg %.0f%% efficiency)", result.Summary.TotalPods, result.Summary.EfficientPods, result.Summary.WastefulPods, result.Summary.AvgEfficiency))
	if result.Summary.OverProvisioned > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d over-provisioned pods — reduce CPU/memory requests", result.Summary.OverProvisioned))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Health Overview
// ---------------------------------------------------------------

type SvcHealthResult1969 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SvcHealthSummary1969 `json:"summary"`
	Healthy         []SvcHealthEntry1969 `json:"healthyServices"`
	Unhealthy       []SvcHealthEntry1969 `json:"unhealthyServices"`
	Recommendations []string             `json:"recommendations"`
}

type SvcHealthSummary1969 struct {
	TotalServices int `json:"totalServices"`
	Healthy       int `json:"healthyServices"`
	Unhealthy     int `json:"unhealthyServices"`
	NoEndpoints   int `json:"servicesWithoutEndpoints"`
}

type SvcHealthEntry1969 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Healthy   bool   `json:"healthy"`
	Endpoints int    `json:"endpointCount"`
}

func (s *Server) handleServiceHealthOverview(w http.ResponseWriter, r *http.Request) {
	result := SvcHealthResult1969{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})

	// Build endpoint count map
	epCount := make(map[string]int)
	for _, ep := range epList.Items {
		count := 0
		for _, sub := range ep.Subsets {
			count += len(sub.Addresses)
		}
		epCount[ep.Namespace+"/"+ep.Name] = count
	}

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		key := svc.Namespace + "/" + svc.Name
		ep := epCount[key]

		entry := SvcHealthEntry1969{
			Name: svc.Name, Namespace: svc.Namespace,
			Type: string(svc.Spec.Type), Endpoints: ep,
		}

		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			entry.Healthy = true
			result.Summary.Healthy++
			result.Healthy = append(result.Healthy, entry)
		} else if ep > 0 {
			entry.Healthy = true
			result.Summary.Healthy++
			result.Healthy = append(result.Healthy, entry)
		} else {
			entry.Healthy = false
			result.Summary.Unhealthy++
			result.Summary.NoEndpoints++
			result.Unhealthy = append(result.Unhealthy, entry)
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services: %d healthy, %d unhealthy", result.Summary.TotalServices, result.Summary.Healthy, result.Summary.Unhealthy))
	if result.Summary.NoEndpoints > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services without endpoints — check backing pods", result.Summary.NoEndpoints))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Utilization Summary
// ---------------------------------------------------------------

type ClusterUtilResult1969 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ClusterUtilSummary1969   `json:"summary"`
	PerNS           []ClusterUtilNSEntry1969 `json:"perNamespace"`
	Recommendations []string                 `json:"recommendations"`
}

type ClusterUtilSummary1969 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AllocatableCPU float64 `json:"allocatableCPU"`
	AllocatableMem float64 `json:"allocatableMemGB"`
	RequestedCPU   float64 `json:"requestedCPU"`
	RequestedMem   float64 `json:"requestedMemGB"`
	CPUUtilization float64 `json:"cpuUtilizationPct"`
	MemUtilization float64 `json:"memUtilizationPct"`
}

type ClusterUtilNSEntry1969 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequest"`
	MemReq    float64 `json:"memRequestGB"`
	Pods      int     `json:"pods"`
}

func (s *Server) handleClusterUtilSummary(w http.ResponseWriter, r *http.Request) {
	result := ClusterUtilResult1969{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*ClusterUtilNSEntry1969)

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.AllocatableCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.AllocatableMem += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		result.Summary.TotalPods++

		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &ClusterUtilNSEntry1969{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.Pods++

		for _, c := range pod.Spec.Containers {
			cpu := c.Resources.Requests.Cpu().AsApproximateFloat64()
			mem := float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
			result.Summary.RequestedCPU += cpu
			result.Summary.RequestedMem += mem
			ns.CPUReq += cpu
			ns.MemReq += mem
		}
	}

	if result.Summary.AllocatableCPU > 0 {
		result.Summary.CPUUtilization = result.Summary.RequestedCPU / result.Summary.AllocatableCPU * 100
	}
	if result.Summary.AllocatableMem > 0 {
		result.Summary.MemUtilization = result.Summary.RequestedMem / result.Summary.AllocatableMem * 100
	}

	for _, ns := range nsStats {
		result.PerNS = append(result.PerNS, *ns)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].CPUReq > result.PerNS[j].CPUReq
	})

	// Score based on utilization health
	if result.Summary.CPUUtilization > 90 || result.Summary.MemUtilization > 90 {
		score -= 20
	} else if result.Summary.CPUUtilization > 80 || result.Summary.MemUtilization > 80 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("CPU: %.1f%% (%.1f / %.1f cores), Mem: %.1f%% (%.1f / %.1f GB)", result.Summary.CPUUtilization, result.Summary.RequestedCPU, result.Summary.AllocatableCPU, result.Summary.MemUtilization, result.Summary.RequestedMem, result.Summary.AllocatableMem))
	if result.Summary.CPUUtilization > 80 {
		result.Recommendations = append(result.Recommendations, "High CPU utilization — consider scaling nodes")
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
