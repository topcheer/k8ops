package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.16 Documentation: Node OperatingSystem, Pod Spec Containers vs InitContainers, Namespace UID Summary
type NodeOSResult2516 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOS       map[string]int `json:"byOperatingSystem"`
	} `json:"summary"`
}

func (s *Server) handleNodeOS2516(w http.ResponseWriter, r *http.Request) {
	result := NodeOSResult2516{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		os := node.Status.NodeInfo.OperatingSystem
		if os == "" {
			os = "<unknown>"
		}
		result.Summary.ByOS[os]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrVsInitCtnrResult2516 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		TotalCtnrs     int `json:"totalContainers"`
		TotalInitCtnrs int `json:"totalInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleCtnrVsInitCtnr2516(w http.ResponseWriter, r *http.Request) {
	result := CtnrVsInitCtnrResult2516{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalCtnrs += len(pod.Spec.Containers)
		result.Summary.TotalInitCtnrs += len(pod.Spec.InitContainers)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSUIDResult2516 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int `json:"totalNamespaces"`
		UniqueUIDs int `json:"uniqueUIDs"`
	} `json:"summary"`
}

func (s *Server) handleNSUID2516(w http.ResponseWriter, r *http.Request) {
	result := NSUIDResult2516{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		uid := string(ns.UID)
		if uid != "" && !seen[uid] {
			seen[uid] = true
			result.Summary.UniqueUIDs++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
