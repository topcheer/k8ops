package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.19 Scalability: Top Namespace by CPU Request, Node Pod Allocation Balance, Cluster Service Endpoint Density
type TopNSCPUResult2319 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuReq"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSCPU2319(w http.ResponseWriter, r *http.Request) {
	result := TopNSCPUResult2319{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns, cpu := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuReq"`
		}{ns, cpu})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodBalanceResult2319 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		AvgPods    int `json:"avgPodsPerNode"`
		StdDevPods int `json:"stdDevPods"`
	} `json:"summary"`
}

func (s *Server) handleNodePodBalance2319(w http.ResponseWriter, r *http.Request) {
	result := NodePodBalanceResult2319{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nodePods[pod.Spec.NodeName]++
	}
	result.Summary.TotalNodes = len(nodePods)
	if result.Summary.TotalNodes > 0 {
		total := 0
		counts := make([]int, 0, len(nodePods))
		for _, c := range nodePods {
			counts = append(counts, c)
			total += c
		}
		avg := total / len(counts)
		result.Summary.AvgPods = avg
		sumSq := 0
		for _, c := range counts {
			sumSq += (c - avg) * (c - avg)
		}
		result.Summary.StdDevPods = isqrt(sumSq / len(counts))
	}
	score := 100
	if result.Summary.AvgPods > 0 {
		cv := result.Summary.StdDevPods * 100 / result.Summary.AvgPods
		if cv > 40 {
			score = 70
		} else if cv > 20 {
			score = 85
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

func isqrt(n int) int {
	if n <= 0 {
		return 0
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}

type SvcEPDensityResult2319 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		TotalEPs      int `json:"totalEndpointAddresses"`
		AvgPerSvc     int `json:"avgPerService"`
	} `json:"summary"`
}

func (s *Server) handleSvcEPDensity2319(w http.ResponseWriter, r *http.Request) {
	result := SvcEPDensityResult2319{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	epCount := make(map[string]int)
	for _, ep := range epList.Items {
		total := 0
		for _, sub := range ep.Subsets {
			total += len(sub.Addresses)
		}
		epCount[ep.Namespace+"/"+ep.Name] = total
		result.Summary.TotalEPs += total
	}
	result.Summary.TotalServices = len(svcList.Items)
	if result.Summary.TotalServices > 0 {
		result.Summary.AvgPerSvc = result.Summary.TotalEPs / result.Summary.TotalServices
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
