package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.06 Documentation: Node Taints Summary, Pod Spec NodeName Detail, Namespace Status Phase Summary
type NodeTaintsSummary2606Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByEffect   map[string]int `json:"byTaintEffect"`
	}
}

func (s *Server) handleNodeTaintsSummary2606(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintsSummary2606Result{ScannedAt: time.Now()}
	result.Summary.ByEffect = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, taint := range node.Spec.Taints {
			result.Summary.ByEffect[string(taint.Effect)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodNodeName2606Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByNode    map[string]int `json:"byNodeName"`
	}
}

func (s *Server) handlePodNodeName2606(w http.ResponseWriter, r *http.Request) {
	result := PodNodeName2606Result{ScannedAt: time.Now()}
	result.Summary.ByNode = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByNode[pod.Spec.NodeName]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSPhaseSummary2606Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		ActiveNS      int `json:"activeNamespaces"`
		TerminatingNS int `json:"terminatingNamespaces"`
	}
}

func (s *Server) handleNSPhaseSummary2606(w http.ResponseWriter, r *http.Request) {
	result := NSPhaseSummary2606Result{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		if ns.DeletionTimestamp != nil {
			result.Summary.TerminatingNS++
		} else {
			result.Summary.ActiveNS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
