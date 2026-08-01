package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.69 Scalability: Top Namespace by Service Count, Node Storage Capacity Total, Cluster PVC Bound
type TopNSBySvcResult2469 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		SvcCount  int    `json:"svcCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySvc2469(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySvcResult2469{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsSvcs := make(map[string]int)
	for _, svc := range svcList.Items {
		nsSvcs[svc.Namespace]++
	}
	result.Summary.TotalNS = len(nsSvcs)
	for ns, count := range nsSvcs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			SvcCount  int    `json:"svcCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SvcCount > result.TopNS[j].SvcCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeStorCapTotalResult2469 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCapGB float64 `json:"totalStorageCapacityGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeStorCapTotal2469(w http.ResponseWriter, r *http.Request) {
	result := NodeStorCapTotalResult2469{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCBoundResult2469 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int `json:"totalPVCs"`
		Bound     int `json:"boundPVCs"`
	} `json:"summary"`
}

func (s *Server) handlePVCBound2469(w http.ResponseWriter, r *http.Request) {
	result := PVCBoundResult2469{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.Bound++
		}
	}
	score := 100
	if result.Summary.TotalPVCs > 0 {
		score = result.Summary.Bound * 100 / result.Summary.TotalPVCs
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
