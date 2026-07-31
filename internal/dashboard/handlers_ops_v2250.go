package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.50 — Operations Dimension (Round 61)
// 1. Pod Container Restart Count Bucket
// 2. Node Memory Pages Committed Catalog
// 3. Service Endpoint Ready vs NotReady
// ============================================================

type RestartBucketResult2250 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByBucket        map[string]int `json:"byRestartBucket"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRestartBucket2250(w http.ResponseWriter, r *http.Request) {
	result := RestartBucketResult2250{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByBucket = make(map[string]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			bucket := "0"
			if cs.RestartCount > 0 {
				bucket = "1-5"
			}
			if cs.RestartCount > 5 {
				bucket = "6-20"
			}
			if cs.RestartCount > 20 {
				bucket = "20+"
			}
			result.Summary.ByBucket[bucket]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Memory Pages Catalog
type NodeMemPagesResult2250 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int   `json:"totalNodes"`
		TotalPages int64 `json:"totalHugePages"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeMemPages2250(w http.ResponseWriter, r *http.Request) {
	result := NodeMemPagesResult2250{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if hp := node.Status.Capacity[corev1.ResourceHugePagesPrefix+"2Mi"]; !hp.IsZero() {
			result.Summary.TotalPages += hp.Value()
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Endpoint Ready vs NotReady
type EPReadyRatioResult2250 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEndpoints int `json:"totalEndpoints"`
		Ready          int `json:"ready"`
		NotReady       int `json:"notReady"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEPReadyRatio2250(w http.ResponseWriter, r *http.Request) {
	result := EPReadyRatioResult2250{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	for _, ep := range epList.Items {
		for _, sub := range ep.Subsets {
			result.Summary.Ready += len(sub.Addresses)
			result.Summary.NotReady += len(sub.NotReadyAddresses)
		}
	}
	result.Summary.TotalEndpoints = result.Summary.Ready + result.Summary.NotReady
	if result.Summary.TotalEndpoints > 0 && result.Summary.NotReady > result.Summary.TotalEndpoints/5 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
