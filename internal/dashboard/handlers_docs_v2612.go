package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.12 Documentation: Node Taint Key Dist, Pod Spec OSEnabled, Namespace Label vs Spec Finalizer
type NodeTaintKey2612Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKey      map[string]int `json:"byTaintKey"`
	} `json:"summary"`
}

func (s *Server) handleNodeTaintKey2612(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintKey2612Result{ScannedAt: time.Now()}
	result.Summary.ByKey = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, taint := range node.Spec.Taints {
			result.Summary.ByKey[taint.Key]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodOSEnabled2612Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithOS    int `json:"withOSSpecified"`
	} `json:"summary"`
}

func (s *Server) handlePodOSEnabled2612(w http.ResponseWriter, r *http.Request) {
	result := PodOSEnabled2612Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.WithOS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelVsSpec2612Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int `json:"totalNamespaces"`
		WithLabels int `json:"withLabels"`
		WithSpec   int `json:"withSpecFinalizers"`
	} `json:"summary"`
}

func (s *Server) handleNSLabelVsSpec2612(w http.ResponseWriter, r *http.Request) {
	result := NSLabelVsSpec2612Result{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Labels) > 0 {
			result.Summary.WithLabels++
		}
		if len(ns.Spec.Finalizers) > 0 {
			result.Summary.WithSpec++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
