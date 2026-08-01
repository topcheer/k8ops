package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.68 Documentation: Node Architecture Distribution, Pod Toleration Summary, Namespace Label Count
type NodeArchResult2468 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	} `json:"summary"`
}

func (s *Server) handleNodeArch2468(w http.ResponseWriter, r *http.Request) {
	result := NodeArchResult2468{ScannedAt: time.Now()}
	result.Summary.ByArch = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		arch := node.Labels[corev1.LabelArchStable]
		if arch == "" {
			arch = "<unknown>"
		}
		result.Summary.ByArch[arch]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TolerationSummaryResult2468 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		TotalTolerations int `json:"totalTolerations"`
	} `json:"summary"`
}

func (s *Server) handleTolerationSummary2468(w http.ResponseWriter, r *http.Request) {
	result := TolerationSummaryResult2468{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalTolerations += len(pod.Spec.Tolerations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelCountResult2468 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS     int `json:"totalNamespaces"`
		TotalLabels int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleNSLabelCount2468(w http.ResponseWriter, r *http.Request) {
	result := NSLabelCountResult2468{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.TotalLabels += len(ns.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
