package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.22 — Documentation Dimension (Round 56)
// 1. Node KubeProxy Version Catalog
// 2. Secret Mounted Volume Count
// 3. Pod Priority Value Distribution
// ============================================================

type KubeProxyVerResult2222 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKubeProxyVer2222(w http.ResponseWriter, r *http.Request) {
	result := KubeProxyVerResult2222{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByVersion = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVersion[node.Status.NodeInfo.KubeProxyVersion]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Mounted Volume Count
type SecretVolCountResult2222 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int `json:"totalPods"`
		WithSecretVol   int `json:"withSecretVolume"`
		TotalSecretRefs int `json:"totalSecretRefs"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecretVolCount2222(w http.ResponseWriter, r *http.Request) {
	result := SecretVolCountResult2222{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				result.Summary.WithSecretVol++
				result.Summary.TotalSecretRefs++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Priority Value Distribution
type PriorityValueResult2222 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByPriority map[string]int `json:"byPriorityValue"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePriorityValue2222(w http.ResponseWriter, r *http.Request) {
	result := PriorityValueResult2222{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPriority = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		bucket := "none"
		if pod.Spec.Priority != nil {
			if *pod.Spec.Priority > 1000000 {
				bucket = "system-critical"
			} else if *pod.Spec.Priority > 0 {
				bucket = "above-default"
			} else {
				bucket = "zero-or-below"
			}
		}
		result.Summary.ByPriority[bucket]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
