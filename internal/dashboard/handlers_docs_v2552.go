package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.52 Documentation: Node Allocatable Pods vs Capacity, Pod Spec RestartPolicy, Namespace Annotation Key Distribution
type NodePodsVsCapResult2552 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCap   int `json:"totalPodCapacity"`
		TotalAlloc int `json:"totalPodAllocatable"`
	}
}

func (s *Server) handleNodePodsVsCap2552(w http.ResponseWriter, r *http.Request) {
	result := NodePodsVsCapResult2552{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += int(node.Status.Capacity.Pods().Value())
		result.Summary.TotalAlloc += int(node.Status.Allocatable.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RestartPolicyResult2552 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byRestartPolicy"`
	}
}

func (s *Server) handleRestartPolicy2552(w http.ResponseWriter, r *http.Request) {
	result := RestartPolicyResult2552{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.RestartPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSAnnotKeyResult2552 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByKey   map[string]int `json:"byAnnotationKey"`
	}
}

func (s *Server) handleNSAnnotKey2552(w http.ResponseWriter, r *http.Request) {
	result := NSAnnotKeyResult2552{ScannedAt: time.Now()}
	result.Summary.ByKey = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		for k := range ns.Annotations {
			result.Summary.ByKey[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
