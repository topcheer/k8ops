package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.80 Documentation: Node MachineID Distribution, Pod Hostname Summary, Namespace Status Phase
type NodeMachineIDResult2480 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		UniqueMach int `json:"uniqueMachineIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeMachineID2480(w http.ResponseWriter, r *http.Request) {
	result := NodeMachineIDResult2480{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		mid := node.Status.NodeInfo.MachineID
		if mid != "" && !seen[mid] {
			seen[mid] = true
			result.Summary.UniqueMach++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodHostnameResult2480 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithHostname int `json:"withHostname"`
	} `json:"summary"`
}

func (s *Server) handlePodHostname2480(w http.ResponseWriter, r *http.Request) {
	result := PodHostnameResult2480{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Hostname != "" {
			result.Summary.WithHostname++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSPhaseResult2480 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByPhase map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handleNSPhase2480(w http.ResponseWriter, r *http.Request) {
	result := NSPhaseResult2480{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.ByPhase[string(ns.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
