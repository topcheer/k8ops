package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.06 Documentation: PV Phase Inventory, Pod Resource Claim Catalog, Node Capacity Pod CIDR
type PVPhaseResult2306 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs int            `json:"totalPVs"`
		ByPhase  map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePVPhase2306(w http.ResponseWriter, r *http.Request) {
	result := PVPhaseResult2306{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		result.Summary.ByPhase[string(pv.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResClaimResult2306 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithResClaims int `json:"withResourceClaims"`
		TotalClaims   int `json:"totalResourceClaims"`
	} `json:"summary"`
}

func (s *Server) handleResClaim2306(w http.ResponseWriter, r *http.Request) {
	result := ResClaimResult2306{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ResourceClaims) > 0 {
			result.Summary.WithResClaims++
			result.Summary.TotalClaims += len(pod.Spec.ResourceClaims)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodCIDRResult2306 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		WithPodCIDR int            `json:"withPodCIDR"`
		ByPrefix    map[string]int `json:"byCIDRPrefix"`
	} `json:"summary"`
}

func (s *Server) handleNodePodCIDR2306(w http.ResponseWriter, r *http.Request) {
	result := NodePodCIDRResult2306{ScannedAt: time.Now()}
	result.Summary.ByPrefix = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Spec.PodCIDR != "" {
			result.Summary.WithPodCIDR++
			// Extract /24 prefix
			parts := strings.SplitN(node.Spec.PodCIDR, ".", 3)
			if len(parts) >= 3 {
				result.Summary.ByPrefix[parts[0]+"."+parts[1]+"."+parts[2]+"."]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
