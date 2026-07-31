package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.25 Scalability: Top Namespace by Memory Request, Node Container Density, Cluster ConfigMap Total
type TopNSMemResult2325 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		MemReqGB  float64 `json:"memReqGB"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSMem2325(w http.ResponseWriter, r *http.Request) {
	result := TopNSMemResult2325{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsMem[pod.Namespace] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsMem)
	for ns, mem := range nsMem {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			MemReqGB  float64 `json:"memReqGB"`
		}{ns, mem})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReqGB > result.TopNS[j].MemReqGB })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCtnrDensityResult2325 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes      int `json:"totalNodes"`
		TotalContainers int `json:"totalContainers"`
		AvgPerNode      int `json:"avgPerNode"`
		MaxPerNode      int `json:"maxPerNode"`
	} `json:"summary"`
}

func (s *Server) handleNodeCtnrDensity2325(w http.ResponseWriter, r *http.Request) {
	result := NodeCtnrDensityResult2325{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeCtnrs := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for range pod.Spec.Containers {
			nodeCtnrs[pod.Spec.NodeName]++
			result.Summary.TotalContainers++
		}
	}
	result.Summary.TotalNodes = len(nodeCtnrs)
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalContainers / result.Summary.TotalNodes
		for _, count := range nodeCtnrs {
			if count > result.Summary.MaxPerNode {
				result.Summary.MaxPerNode = count
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterCMTotalResult2325 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int            `json:"totalConfigMaps"`
		ByNamespace map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleClusterCMTotal2325(w http.ResponseWriter, r *http.Request) {
	result := ClusterCMTotalResult2325{ScannedAt: time.Now()}
	result.Summary.ByNamespace = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.ByNamespace[cm.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
