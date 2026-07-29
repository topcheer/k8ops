package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.54 — Scalability & HA Dimension (Round 28)
// 1. HPA Metric Coverage — HPA metric source completeness
// 2. Pod Anti-Affinity Coverage — multi-replica topology spread
// 3. Cluster Capacity Headroom — total cluster capacity utilization
// ============================================================

// ---------------------------------------------------------------
// 1. HPA Metric Coverage
// ---------------------------------------------------------------

type HPAMetricResult2054 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HPAMetricSummary2054 `json:"summary"`
	PoorCoverage    []HPAMetricEntry2054 `json:"poorCoverage"`
	Recommendations []string             `json:"recommendations"`
}

type HPAMetricSummary2054 struct {
	TotalHPAs     int `json:"totalHPAs"`
	CPUMetrics    int `json:"cpuMetrics"`
	MemMetrics    int `json:"memMetrics"`
	CustomMetrics int `json:"customMetrics"`
}

type HPAMetricEntry2054 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
}

func (s *Server) handleHPAMetricCoverage(w http.ResponseWriter, r *http.Request) {
	result := HPAMetricResult2054{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++

		hasCPU := false
		hasMem := false
		hasCustom := false

		for _, metric := range hpa.Spec.Metrics {
			switch metric.Type {
			case autoscalingv2.ResourceMetricSourceType:
				if metric.Resource.Name == corev1.ResourceCPU {
					hasCPU = true
				}
				if metric.Resource.Name == corev1.ResourceMemory {
					hasMem = true
				}
			case autoscalingv2.PodsMetricSourceType, autoscalingv2.ExternalMetricSourceType:
				hasCustom = true
			}
		}

		if hasCPU {
			result.Summary.CPUMetrics++
		}
		if hasMem {
			result.Summary.MemMetrics++
		}
		if hasCustom {
			result.Summary.CustomMetrics++
		}

		if !hasCPU {
			result.PoorCoverage = append(result.PoorCoverage, HPAMetricEntry2054{
				Name: hpa.Name, Namespace: hpa.Namespace, Issue: "no CPU metric",
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.PoorCoverage, func(i, j int) bool {
		return result.PoorCoverage[i].Namespace < result.PoorCoverage[j].Namespace
	})

	if len(result.PoorCoverage) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d HPAs lack CPU metrics — add CPU resource metric for autoscaling", len(result.PoorCoverage)))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Anti-Affinity Coverage
// ---------------------------------------------------------------

type AntiAffinityResult2054 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         AntiAffinitySummary2054 `json:"summary"`
	MissingAA       []AntiAffinityEntry2054 `json:"missingAntiAffinity"`
	Recommendations []string                `json:"recommendations"`
}

type AntiAffinitySummary2054 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithAntiAffinity  int `json:"withAntiAffinity"`
	MissingAA         int `json:"missingAntiAffinity"`
}

type AntiAffinityEntry2054 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleAntiAffinityCoverage2054(w http.ResponseWriter, r *http.Request) {
	result := AntiAffinityResult2054{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			continue
		}
		result.Summary.TotalMultiReplica++

		hasAA := false
		if dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAA = true
		}
		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			hasAA = true
		}

		if hasAA {
			result.Summary.WithAntiAffinity++
		} else {
			result.Summary.MissingAA++
			result.MissingAA = append(result.MissingAA, AntiAffinityEntry2054{
				Name: dep.Name, Namespace: dep.Namespace,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.MissingAA, func(i, j int) bool {
		return result.MissingAA[i].Namespace < result.MissingAA[j].Namespace
	})

	if result.Summary.MissingAA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica deployments lack anti-affinity — add podAntiAffinity for HA", result.Summary.MissingAA))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Capacity Headroom
// ---------------------------------------------------------------

type ClusterCapResult2054 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ClusterCapSummary2054 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type ClusterCapSummary2054 struct {
	TotalCapacityCPU float64 `json:"totalCapacityCpu"`
	TotalCapacityMem float64 `json:"totalCapacityMemGB"`
	AllocatableCPU   float64 `json:"allocatableCpu"`
	AllocatableMem   float64 `json:"allocatableMemGB"`
	RequestedCPU     float64 `json:"requestedCpu"`
	RequestedMem     float64 `json:"requestedMemGB"`
	HeadroomCPUPct   int     `json:"headroomCpuPct"`
	HeadroomMemPct   int     `json:"headroomMemPct"`
}

func (s *Server) handleClusterCapHeadroom(w http.ResponseWriter, r *http.Request) {
	result := ClusterCapResult2054{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var capCPU, capMem, allocCPU, allocMem, reqCPU, reqMem float64

	for _, node := range nodeList.Items {
		capCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		capMem += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
		allocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocMem += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				reqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			}
			if !c.Resources.Requests.Memory().IsZero() {
				reqMem += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			}
		}
	}

	result.Summary.TotalCapacityCPU = capCPU
	result.Summary.TotalCapacityMem = capMem
	result.Summary.AllocatableCPU = allocCPU
	result.Summary.AllocatableMem = allocMem
	result.Summary.RequestedCPU = reqCPU
	result.Summary.RequestedMem = reqMem

	if allocCPU > 0 {
		result.Summary.HeadroomCPUPct = int((1 - reqCPU/allocCPU) * 100)
	}
	if allocMem > 0 {
		result.Summary.HeadroomMemPct = int((1 - reqMem/allocMem) * 100)
	}

	if result.Summary.HeadroomCPUPct < 20 || result.Summary.HeadroomMemPct < 20 {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.HeadroomCPUPct < 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("CPU headroom only %d%% — add nodes or optimize requests", result.Summary.HeadroomCPUPct))
	}

	writeJSON(w, result)
}
