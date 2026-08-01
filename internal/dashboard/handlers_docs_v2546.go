package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.46 Documentation: Node Feature Labels Count, Pod Spec Resource Claim Count, Namespace UID Dist
type NodeFeatureLabelsResult2546 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalLabels int `json:"totalFeatureLabels"`
	} `json:"summary"`
}

func (s *Server) handleNodeFeatureLabels2546(w http.ResponseWriter, r *http.Request) {
	result := NodeFeatureLabelsResult2546{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for k := range node.Labels {
			if len(k) > 16 && k[:17] == "node.kubernetes.io" {
				result.Summary.TotalLabels++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceClaimResult2546 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		TotalClaims int `json:"totalResourceClaims"`
	} `json:"summary"`
}

func (s *Server) handleResourceClaim2546(w http.ResponseWriter, r *http.Request) {
	result := ResourceClaimResult2546{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalClaims += len(pod.Spec.ResourceClaims)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSUIDDistResult2546 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int            `json:"totalNamespaces"`
		ByPrefix map[string]int `json:"byUIDPrefix"`
	} `json:"summary"`
}

func (s *Server) handleNSUIDDist2546(w http.ResponseWriter, r *http.Request) {
	result := NSUIDDistResult2546{ScannedAt: time.Now()}
	result.Summary.ByPrefix = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		uid := string(ns.UID)
		prefix := "<none>"
		if len(uid) >= 8 {
			prefix = uid[:8]
		}
		result.Summary.ByPrefix[prefix]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
