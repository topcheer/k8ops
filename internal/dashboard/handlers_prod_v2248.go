package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.48 — Product Dimension (Round 61)
// 1. Pod Topology Spread Constraints Catalog
// 2. Container Stdin TTY Distribution
// 3. Service Allocation Allocated Ports Tracker
// ============================================================

type TopoSpreadCatalogResult2248 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int            `json:"totalPods"`
		WithTopoSpread int            `json:"withTopologySpread"`
		ByWhen         map[string]int `json:"byWhenUnsatisfiable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleTopoSpreadCatalog2248(w http.ResponseWriter, r *http.Request) {
	result := TopoSpreadCatalogResult2248{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByWhen = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithTopoSpread++
			for _, tsc := range pod.Spec.TopologySpreadConstraints {
				result.Summary.ByWhen[string(tsc.WhenUnsatisfiable)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Stdin TTY
type StdinTTYResult2248 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdin       int `json:"withStdin"`
		WithTTY         int `json:"withTTY"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleStdinTTY2248(w http.ResponseWriter, r *http.Request) {
	result := StdinTTYResult2248{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStdin++
			}
			if c.TTY {
				result.Summary.WithTTY++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Service Allocated Ports
type AllocPortsResult2248 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices  int `json:"totalServices"`
		WithAllocPorts int `json:"withAllocatedPorts"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAllocPorts2248(w http.ResponseWriter, r *http.Request) {
	result := AllocPortsResult2248{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.AllocateLoadBalancerNodePorts != nil && !*svc.Spec.AllocateLoadBalancerNodePorts {
			result.Summary.WithAllocPorts++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
