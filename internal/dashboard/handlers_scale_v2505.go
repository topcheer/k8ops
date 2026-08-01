package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.05 Scalability: Top Namespace by Deployment, Node Memory Allocatable Total, Cluster PV Phase Distribution
type TopNSByDeployResult2505 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		DepCount  int    `json:"deploymentCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByDeploy2505(w http.ResponseWriter, r *http.Request) {
	result := TopNSByDeployResult2505{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsDeps := make(map[string]int)
	for _, dep := range depList.Items {
		nsDeps[dep.Namespace]++
	}
	result.Summary.TotalNS = len(nsDeps)
	for ns, count := range nsDeps {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			DepCount  int    `json:"deploymentCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].DepCount > result.TopNS[j].DepCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemAllocTotalResult2505 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableMemGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemAllocTotal2505(w http.ResponseWriter, r *http.Request) {
	result := NodeMemAllocTotalResult2505{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVPhaseDistResult2505 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs int            `json:"totalPVs"`
		ByPhase  map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePVPhaseDist2505(w http.ResponseWriter, r *http.Request) {
	result := PVPhaseDistResult2505{ScannedAt: time.Now()}
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
