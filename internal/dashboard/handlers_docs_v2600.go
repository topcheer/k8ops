package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.00 Documentation: Node ContainerRuntimeVersion vs KubeletVersion, Pod Spec PodSecurityContext, Namespace Phase Summary
type RuntimeVsKubelet2600Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		Mismatched int `json:"versionMismatch"`
	}
}

func (s *Server) handleRuntimeVsKubelet2600(w http.ResponseWriter, r *http.Request) {
	result := RuntimeVsKubelet2600Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Status.NodeInfo.ContainerRuntimeVersion != node.Status.NodeInfo.KubeletVersion {
			result.Summary.Mismatched++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Mismatched*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PodSecurityCtx2600Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithSecCtx int `json:"withPodSecurityContext"`
	}
}

func (s *Server) handlePodSecurityCtx2600(w http.ResponseWriter, r *http.Request) {
	result := PodSecurityCtx2600Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil {
			result.Summary.WithSecCtx++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSPhase2600Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByPhase map[string]int `json:"byPhase"`
	}
}

func (s *Server) handleNSPhase2600(w http.ResponseWriter, r *http.Request) {
	result := NSPhase2600Result{ScannedAt: time.Now()}
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
