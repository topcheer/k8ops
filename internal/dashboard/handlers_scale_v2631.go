package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.31 Scalability: Top Namespace by NetworkPolicy, Node Memory Allocatable Total, Cluster PVC Bound Count
type TopNSByNetPolicy2631Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
		TotalNP int `json:"totalNetworkPolicies"`
	}
}

func (s *Server) handleTopNSByNetPolicy2631(w http.ResponseWriter, r *http.Request) {
	result := TopNSByNetPolicy2631Result{ScannedAt: time.Now()}
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsNP := make(map[string]int)
	for _, np := range npList.Items {
		nsNP[np.Namespace]++
	}
	result.Summary.TotalNS = len(nsNP)
	for _, count := range nsNP {
		result.Summary.TotalNP += count
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemAllocTotal2631Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocMemGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemAllocTotal2631(w http.ResponseWriter, r *http.Request) {
	result := NodeMemAllocTotal2631Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCBoundCount2631Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int `json:"totalPVCs"`
		Bound     int `json:"boundPVCs"`
	} `json:"summary"`
}

func (s *Server) handlePVCBoundCount2631(w http.ResponseWriter, r *http.Request) {
	result := PVCBoundCount2631Result{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if string(pvc.Status.Phase) == "Bound" {
			result.Summary.Bound++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
