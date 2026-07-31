package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v22.95 Scalability: Resource Waste Detection, Pod Spread Balance, Cluster Workload Concentration
type ResourceWasteResult2295 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers  int `json:"totalContainers"`
		OverProvisioned  int `json:"overProvisioned"`
		UnderProvisioned int `json:"underProvisioned"`
	} `json:"summary"`
}

func (s *Server) handleResourceWaste2295(w http.ResponseWriter, r *http.Request) {
	result := ResourceWasteResult2295{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			reqCPU := c.Resources.Requests.Cpu().AsApproximateFloat64()
			limCPU := c.Resources.Limits.Cpu().AsApproximateFloat64()
			if reqCPU > 0 && limCPU > 0 {
				if limCPU > reqCPU*10 {
					result.Summary.OverProvisioned++
				}
				if limCPU < reqCPU {
					result.Summary.UnderProvisioned++
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		waste := result.Summary.OverProvisioned + result.Summary.UnderProvisioned
		score = 100 - (waste*50)/result.Summary.TotalContainers
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PodSpreadBalanceResult2295 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		MaxPods    int `json:"maxPodsPerNode"`
		MinPods    int `json:"minPodsPerNode"`
		AvgPods    int `json:"avgPodsPerNode"`
	} `json:"summary"`
}

func (s *Server) handlePodSpreadBalance2295(w http.ResponseWriter, r *http.Request) {
	result := PodSpreadBalanceResult2295{ScannedAt: time.Now()}
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
		counts := make([]int, 0, len(nodePods))
		for _, c := range nodePods {
			counts = append(counts, c)
		}
		sort.Ints(counts)
		result.Summary.MinPods = counts[0]
		result.Summary.MaxPods = counts[len(counts)-1]
		total := 0
		for _, c := range counts {
			total += c
		}
		result.Summary.AvgPods = total / len(counts)
	}
	score := 100
	if result.Summary.AvgPods > 0 {
		imbalance := (result.Summary.MaxPods - result.Summary.MinPods) * 100 / result.Summary.AvgPods
		if imbalance > 50 {
			score = 70
		} else if imbalance > 30 {
			score = 85
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type WorkloadConcResult2295 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int            `json:"totalPods"`
		ByController map[string]int `json:"byControllerKind"`
	} `json:"summary"`
}

func (s *Server) handleWorkloadConc2295(w http.ResponseWriter, r *http.Request) {
	result := WorkloadConcResult2295{ScannedAt: time.Now()}
	result.Summary.ByController = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		kind := " standalone"
		for _, ref := range pod.ObjectMeta.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				kind = ref.Kind
				break
			}
		}
		result.Summary.ByController[kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
