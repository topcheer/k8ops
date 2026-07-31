package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v22.89 Scalability: Top Image by Replica Count, Node Memory Oversubscription, Pod Age Distribution
type TopImageResult2289 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int `json:"totalUniqueImages"`
	} `json:"summary"`
	TopImages []struct {
		Image        string `json:"image"`
		ReplicaCount int    `json:"replicaCount"`
	} `json:"topImages"`
}

func (s *Server) handleTopImage2289(w http.ResponseWriter, r *http.Request) {
	result := TopImageResult2289{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	imgCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			imgCount[c.Image]++
		}
	}
	result.Summary.TotalImages = len(imgCount)
	for img, count := range imgCount {
		result.TopImages = append(result.TopImages, struct {
			Image        string `json:"image"`
			ReplicaCount int    `json:"replicaCount"`
		}{img, count})
	}
	sort.Slice(result.TopImages, func(i, j int) bool { return result.TopImages[i].ReplicaCount > result.TopImages[j].ReplicaCount })
	if len(result.TopImages) > 10 {
		result.TopImages = result.TopImages[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemOversubResult2289 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		Oversubscribed int `json:"oversubscribedNodes"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemOversub2289(w http.ResponseWriter, r *http.Request) {
	result := NodeMemOversubResult2289{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeReq := make(map[string]float64)
	nodeAlloc := make(map[string]float64)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		nodeAlloc[node.Name] = node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nodeReq[pod.Spec.NodeName] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	for _, node := range nodeList.Items {
		alloc := nodeAlloc[node.Name]
		req := nodeReq[node.Name]
		if alloc > 0 && req > alloc {
			result.Summary.Oversubscribed++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Oversubscribed*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PodAgeDistResult2289 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByAgeBucket map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handlePodAgeDist2289(w http.ResponseWriter, r *http.Request) {
	result := PodAgeDistResult2289{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		age := now.Sub(pod.Status.StartTime.Time)
		var bucket string
		switch {
		case age < time.Hour:
			bucket = "<1h"
		case age < 24*time.Hour:
			bucket = "1h-1d"
		case age < 7*24*time.Hour:
			bucket = "1-7d"
		case age < 30*24*time.Hour:
			bucket = "7-30d"
		default:
			bucket = "30d+"
		}
		result.Summary.ByAgeBucket[bucket]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
