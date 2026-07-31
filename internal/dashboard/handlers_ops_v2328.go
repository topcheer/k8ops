package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.28 Operations: Pod QoS Burstable Audit, Node Memory Frag, Container Image Age
type BurstableResult2328 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByQoS     map[string]int `json:"byQoSClass"`
	} `json:"summary"`
}

func (s *Server) handleBurstable2328(w http.ResponseWriter, r *http.Request) {
	result := BurstableResult2328{ScannedAt: time.Now()}
	result.Summary.ByQoS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByQoS[string(pod.Status.QOSClass)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemFrag2Result2328 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCapGB float64 `json:"totalCapacityGB"`
		TotalReqGB float64 `json:"totalRequestedGB"`
		FragPct    int     `json:"fragPct"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemFrag2328(w http.ResponseWriter, r *http.Request) {
	result := NodeMemFrag2Result2328{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.TotalCapGB > 0 {
		result.Summary.FragPct = int((1 - result.Summary.TotalReqGB/result.Summary.TotalCapGB) * 100)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageAgeResult2328 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByTag       map[string]int `json:"byTagType"`
	} `json:"summary"`
}

func (s *Server) handleImageAge2328(w http.ResponseWriter, r *http.Request) {
	result := ImageAgeResult2328{ScannedAt: time.Now()}
	result.Summary.ByTag = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seen[c.Image] {
				continue
			}
			seen[c.Image] = true
			result.Summary.TotalImages++
			if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":") {
				result.Summary.ByTag["latest/none"]++
			} else {
				result.Summary.ByTag["versioned"]++
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 {
		latest := result.Summary.ByTag["latest/none"]
		score = 100 - (latest*50)/result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
