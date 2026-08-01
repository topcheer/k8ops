package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.92 Operations: Node ContainerRuntime Dist, Pod Spec NodeSelector Count, Container Image Latest Check
type RuntimeDistResult2592 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byRuntime"`
	}
}

func (s *Server) handleRuntimeDist2592(w http.ResponseWriter, r *http.Request) {
	result := RuntimeDistResult2592{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		rt := node.Status.NodeInfo.ContainerRuntimeVersion
		if rt == "" {
			rt = "<unknown>"
		}
		result.Summary.ByRuntime[rt]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeSelectorCountResult2592 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithSelector int `json:"withNodeSelector"`
	}
}

func (s *Server) handleNodeSelectorCount2592(w http.ResponseWriter, r *http.Request) {
	result := NodeSelectorCountResult2592{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.NodeSelector) > 0 {
			result.Summary.WithSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageLatestResult2592 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int `json:"totalImages"`
		WithLatest  int `json:"withLatestTag"`
	}
}

func (s *Server) handleImageLatest2592(w http.ResponseWriter, r *http.Request) {
	result := ImageLatestResult2592{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			if c.Image == "latest" || hasSuffix(c.Image, ":latest") {
				result.Summary.WithLatest++
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 {
		score = 100 - (result.Summary.WithLatest*100)/result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
