package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.41 Scalability: Top Namespace by Service, Node Memory Usage Ratio, Cluster PodDisruptionBudget Count
type TopNSBySvc2541Result struct {
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

func (s *Server) handleTopNSBySvc2541(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySvc2541Result{ScannedAt: time.Now()}
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

type NodeMemUsageRatioResult2541 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgUsage   float64 `json:"avgMemUsagePercent"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemUsageRatio2541(w http.ResponseWriter, r *http.Request) {
	result := NodeMemUsageRatioResult2541{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Capacity.Memory().AsApproximateFloat64()
		alloc := node.Status.Allocatable.Memory().AsApproximateFloat64()
		if cap > 0 {
			totalRatio += (cap - alloc) / cap * 100
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgUsage = totalRatio / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PDBCountResult2541 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPDBs int            `json:"totalPDBs"`
		ByNS      map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handlePDBCount2541(w http.ResponseWriter, r *http.Request) {
	result := PDBCountResult2541{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	for _, pdb := range pdbList.Items {
		result.Summary.TotalPDBs++
		result.Summary.ByNS[pdb.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
