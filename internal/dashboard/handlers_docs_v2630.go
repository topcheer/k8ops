package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.30 Documentation: Node ContainerRuntime Verbose, Pod Spec HostNetwork Detail, Namespace Annotated Count
type NodeRuntimeVerbose2630Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byContainerRuntimePrefix"`
	} `json:"summary"`
}

func (s *Server) handleNodeRuntimeVerbose2630(w http.ResponseWriter, r *http.Request) {
	result := NodeRuntimeVerbose2630Result{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		rt := node.Status.NodeInfo.ContainerRuntimeVersion
		prefix := "<unknown>"
		for i, ch := range rt {
			if ch == ':' {
				prefix = rt[:i]
				break
			}
		}
		result.Summary.ByRuntime[prefix]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodHostNetwork2630Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostNet   int `json:"hostNetwork"`
	} `json:"summary"`
}

func (s *Server) handlePodHostNetwork2630(w http.ResponseWriter, r *http.Request) {
	result := PodHostNetwork2630Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostNetwork {
			result.Summary.HostNet++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSAnnotatedCount2630Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS   int `json:"totalNamespaces"`
		WithAnnot int `json:"withAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleNSAnnotatedCount2630(w http.ResponseWriter, r *http.Request) {
	result := NSAnnotatedCount2630Result{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Annotations) > 0 {
			result.Summary.WithAnnot++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
