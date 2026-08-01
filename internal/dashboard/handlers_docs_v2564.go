package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.64 Documentation: Node Capacity vs Allocatable Storage, Pod Spec NodeSelector Detail, Namespace Label vs Annotation Count
type NodeCapVsAllocStorResult2564 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCapStorageGB"`
		TotalAlloc float64 `json:"totalAllocStorageGB"`
	}
}

func (s *Server) handleNodeCapVsAllocStor2564(w http.ResponseWriter, r *http.Request) {
	result := NodeCapVsAllocStorResult2564{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += node.Status.Capacity.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
		result.Summary.TotalAlloc += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeSelectorDetailResult2564 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByLabelKey map[string]int `json:"byNodeSelectorKey"`
	}
}

func (s *Server) handleNodeSelectorDetail2564(w http.ResponseWriter, r *http.Request) {
	result := NodeSelectorDetailResult2564{ScannedAt: time.Now()}
	result.Summary.ByLabelKey = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for k := range pod.Spec.NodeSelector {
			result.Summary.ByLabelKey[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelVsAnnotResult2564 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS     int `json:"totalNamespaces"`
		TotalLabels int `json:"totalLabels"`
		TotalAnnots int `json:"totalAnnotations"`
	}
}

func (s *Server) handleNSLabelVsAnnot2564(w http.ResponseWriter, r *http.Request) {
	result := NSLabelVsAnnotResult2564{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.TotalLabels += len(ns.Labels)
		result.Summary.TotalAnnots += len(ns.Annotations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
