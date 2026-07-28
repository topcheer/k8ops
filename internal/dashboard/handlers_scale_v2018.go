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
// v20.18 — Scalability & HA Dimension (Round 22 Final)
// 1. Cluster Pod Density Trend — pods/node density pressure tracking
// 2. Service Mesh Endpoint Budget — service endpoint count for mesh scaling
// 3. Node Allocatable Headroom — allocatable vs capacity ratio per node
// ============================================================

// ---------------------------------------------------------------
// 1. Cluster Pod Density Trend
// ---------------------------------------------------------------

type PodDensResult2018 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodDensSummary2018 `json:"summary"`
	PerNode         []PodDensEntry2018 `json:"perNode"`
	Recommendations []string           `json:"recommendations"`
}

type PodDensSummary2018 struct {
	TotalNodes   int     `json:"totalNodes"`
	TotalPods    int     `json:"totalPods"`
	AvgDensity   float64 `json:"avgPodsPerNode"`
	MaxDensity   int     `json:"maxPodsPerNode"`
	DensityLevel string  `json:"densityLevel"`
}

type PodDensEntry2018 struct {
	Node       string  `json:"node"`
	PodCount   int     `json:"podCount"`
	DensityPct float64 `json:"densityPct"`
}

func (s *Server) handlePodDensTrend(w http.ResponseWriter, r *http.Request) {
	result := PodDensResult2018{ScannedAt: time.Now()}
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

	result.Summary.TotalNodes = len(nodeList.Items)
	maxDens := 0
	for _, node := range nodeList.Items {
		count := podsPerNode[node.Name]
		if count > maxDens {
			maxDens = count
		}
		densPct := float64(count) / 110 * 100
		if densPct > 100 {
			densPct = 100
		}

		result.PerNode = append(result.PerNode, PodDensEntry2018{
			Node: node.Name, PodCount: count, DensityPct: densPct,
		})
	}
	result.Summary.MaxDensity = maxDens

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgDensity = float64(result.Summary.TotalPods) / float64(result.Summary.TotalNodes)
	}

	if maxDens > 100 {
		result.Summary.DensityLevel = "critical"
		score -= 10
	} else if maxDens > 80 {
		result.Summary.DensityLevel = "high"
		score -= 5
	} else if maxDens > 50 {
		result.Summary.DensityLevel = "medium"
	} else {
		result.Summary.DensityLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d pods, avg %.0f/node, max %d, level: %s", result.Summary.TotalNodes, result.Summary.TotalPods, result.Summary.AvgDensity, maxDens, result.Summary.DensityLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Mesh Endpoint Budget
// ---------------------------------------------------------------

type SMEPResult2018 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         SMEPSummary2018 `json:"summary"`
	PerNS           []SMEPEntry2018 `json:"perNamespace"`
	Recommendations []string        `json:"recommendations"`
}

type SMEPSummary2018 struct {
	TotalServices  int     `json:"totalServices"`
	TotalEndpoints int     `json:"totalEndpoints"`
	AvgEPPerSvc    float64 `json:"avgEndpointsPerService"`
	MaxEPPerSvc    int     `json:"maxEndpointsPerService"`
}

type SMEPEntry2018 struct {
	Namespace string `json:"namespace"`
	SvcCount  int    `json:"serviceCount"`
	EPCount   int    `json:"endpointCount"`
}

func (s *Server) handleSMEPBudget(w http.ResponseWriter, r *http.Request) {
	result := SMEPResult2018{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*SMEPEntry2018)
	totalEP := 0
	maxEP := 0

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		entry, ok := nsStats[svc.Namespace]
		if !ok {
			entry = &SMEPEntry2018{Namespace: svc.Namespace}
			nsStats[svc.Namespace] = entry
		}
		entry.SvcCount++
	}

	for _, ep := range epList.Items {
		count := 0
		for _, sub := range ep.Subsets {
			count += len(sub.Addresses)
		}
		totalEP += count
		if count > maxEP {
			maxEP = count
		}

		entry, ok := nsStats[ep.Namespace]
		if !ok {
			entry = &SMEPEntry2018{Namespace: ep.Namespace}
			nsStats[ep.Namespace] = entry
		}
		entry.EPCount += count
	}

	result.Summary.TotalEndpoints = totalEP
	result.Summary.MaxEPPerSvc = maxEP
	if result.Summary.TotalServices > 0 {
		result.Summary.AvgEPPerSvc = float64(totalEP) / float64(result.Summary.TotalServices)
	}

	for _, entry := range nsStats {
		result.PerNS = append(result.PerNS, *entry)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EPCount > result.PerNS[j].EPCount
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services, %d endpoints, avg %.1f/svc, max %d", result.Summary.TotalServices, totalEP, result.Summary.AvgEPPerSvc, maxEP))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Allocatable Headroom
// ---------------------------------------------------------------

type AllocHeadResult2018 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         AllocHeadSummary2018 `json:"summary"`
	PerNode         []AllocHeadEntry2018 `json:"perNode"`
	Recommendations []string             `json:"recommendations"`
}

type AllocHeadSummary2018 struct {
	TotalNodes    int     `json:"totalNodes"`
	AvgAllocRatio float64 `json:"avgAllocatableRatio"`
	WorstNode     string  `json:"worstNode"`
}

type AllocHeadEntry2018 struct {
	Node           string  `json:"node"`
	CapacityCPU    float64 `json:"capacityCPU"`
	AllocatableCPU float64 `json:"allocatableCPU"`
	Ratio          float64 `json:"ratio"`
}

func (s *Server) handleAllocHead(w http.ResponseWriter, r *http.Request) {
	result := AllocHeadResult2018{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	var totalRatio float64
	worstRatio := 1.0
	worstNode := ""

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		capCPU := node.Status.Capacity.Cpu().AsApproximateFloat64()
		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()

		ratio := 1.0
		if capCPU > 0 {
			ratio = allocCPU / capCPU
		}

		result.PerNode = append(result.PerNode, AllocHeadEntry2018{
			Node: node.Name, CapacityCPU: capCPU,
			AllocatableCPU: allocCPU, Ratio: ratio,
		})

		totalRatio += ratio
		if ratio < worstRatio {
			worstRatio = ratio
			worstNode = node.Name
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgAllocRatio = totalRatio / float64(result.Summary.TotalNodes)
	}
	result.Summary.WorstNode = worstNode

	if worstRatio < 0.5 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, avg alloc ratio %.2f, worst: %s (%.2f)", result.Summary.TotalNodes, result.Summary.AvgAllocRatio, worstNode, worstRatio))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
