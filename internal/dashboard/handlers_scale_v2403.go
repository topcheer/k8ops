package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.03 Scalability: Top Node by Memory Request, Namespace ServiceAccount Count, Cluster Image Unique
type TopNodeMemReqResult2403 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node     string  `json:"node"`
		MemReqGB float64 `json:"memReqGB"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeMemReq2403(w http.ResponseWriter, r *http.Request) {
	result := TopNodeMemReqResult2403{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nodeMem[pod.Spec.NodeName] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNodes = len(nodeMem)
	for node, mem := range nodeMem {
		result.TopNodes = append(result.TopNodes, struct {
			Node     string  `json:"node"`
			MemReqGB float64 `json:"memReqGB"`
		}{node, mem})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].MemReqGB > result.TopNodes[j].MemReqGB })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSSACountResult2403 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs int            `json:"totalServiceAccounts"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNSSACount2403(w http.ResponseWriter, r *http.Request) {
	result := NSSACountResult2403{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		result.Summary.ByNS[sa.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImgUniqueResult2403 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		UniqueImages    int `json:"uniqueImages"`
	} `json:"summary"`
}

func (s *Server) handleImgUnique2403(w http.ResponseWriter, r *http.Request) {
	result := ImgUniqueResult2403{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			seen[c.Image] = true
		}
	}
	result.Summary.UniqueImages = len(seen)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
