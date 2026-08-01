package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.92 Documentation: Node OS Image Architecture, Pod Spec Priority Value, Namespace Finalizer Count
type NodeOSArchResult2492 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSArch2492(w http.ResponseWriter, r *http.Request) {
	result := NodeOSArchResult2492{ScannedAt: time.Now()}
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

type PodPriorityValueResult2492 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithPriority int `json:"withPriorityValue"`
	} `json:"summary"`
}

func (s *Server) handlePodPriorityValue2492(w http.ResponseWriter, r *http.Request) {
	result := PodPriorityValueResult2492{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Priority != nil {
			result.Summary.WithPriority++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSFinalizerResult2492 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int `json:"totalNamespaces"`
		TotalFinal int `json:"totalFinalizers"`
	} `json:"summary"`
}

func (s *Server) handleNSFinalizer2492(w http.ResponseWriter, r *http.Request) {
	result := NSFinalizerResult2492{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.TotalFinal += len(ns.Spec.Finalizers)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
