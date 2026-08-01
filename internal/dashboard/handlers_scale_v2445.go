package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.45 Scalability: Top Namespace by Pod, Node Allocatable Memory, Cluster Secret Type Distribution
type TopNSByPodResult2445 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		PodCount  int    `json:"podCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByPod2445(w http.ResponseWriter, r *http.Request) {
	result := TopNSByPodResult2445{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPods[pod.Namespace]++
	}
	result.Summary.TotalNS = len(nsPods)
	for ns, count := range nsPods {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			PodCount  int    `json:"podCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PodCount > result.TopNS[j].PodCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocMemResult2445 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalAllocGB float64 `json:"totalAllocatableMemoryGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocMem2445(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocMemResult2445{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretTypeDistResult2445 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleSecretTypeDist2445(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeDistResult2445{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
