package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.76 Documentation: Namespace ResourceQuota Catalog, Pod Topology Spread Constraints, Node Taints Inventory
type ResourceQuotaResult2276 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalQuotas int            `json:"totalQuotas"`
		ByNamespace map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleResourceQuota2276(w http.ResponseWriter, r *http.Request) {
	result := ResourceQuotaResult2276{ScannedAt: time.Now()}
	result.Summary.ByNamespace = make(map[string]int)
	quotaList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	for _, rq := range quotaList.Items {
		result.Summary.TotalQuotas++
		result.Summary.ByNamespace[rq.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopoSpreadResult2276 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		WithTopoSpread int `json:"withTopoSpread"`
	} `json:"summary"`
}

func (s *Server) handleTopoSpread2276(w http.ResponseWriter, r *http.Request) {
	result := TopoSpreadResult2276{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithTopoSpread++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeTaintsResult2276 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		WithTaints  int `json:"withTaints"`
		TotalTaints int `json:"totalTaints"`
	} `json:"summary"`
}

func (s *Server) handleNodeTaints2276(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintsResult2276{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if len(node.Spec.Taints) > 0 {
			result.Summary.WithTaints++
			result.Summary.TotalTaints += len(node.Spec.Taints)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
