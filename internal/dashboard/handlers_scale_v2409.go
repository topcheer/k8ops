package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.09 Scalability: Top Namespace by Container, Node Allocatable Storage Ephemeral, Cluster Role Total
type TopNSCtnrResult2409 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		CtnrCount int    `json:"containerCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSCtnr2409(w http.ResponseWriter, r *http.Request) {
	result := TopNSCtnrResult2409{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCtnrs := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for range pod.Spec.Containers {
			nsCtnrs[pod.Namespace]++
		}
	}
	result.Summary.TotalNS = len(nsCtnrs)
	for ns, count := range nsCtnrs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			CtnrCount int    `json:"containerCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CtnrCount > result.TopNS[j].CtnrCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocStorEphResult2409 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalAllocatableGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocStorEph2409(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocStorEphResult2409{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleTotalResult2409 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles        int `json:"totalRoles"`
		TotalClusterRoles int `json:"totalClusterRoles"`
	} `json:"summary"`
}

func (s *Server) handleRoleTotal2409(w http.ResponseWriter, r *http.Request) {
	result := RoleTotalResult2409{ScannedAt: time.Now()}
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalRoles = len(roleList.Items)
	result.Summary.TotalClusterRoles = len(crList.Items)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
