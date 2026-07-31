package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.67 Scalability: Top Namespace by Deployment, Node Capacity Storage, Cluster NetworkPolicy Density
type TopNSDeployResult2367 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace   string `json:"namespace"`
		DeployCount int    `json:"deployCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSDeploy2367(w http.ResponseWriter, r *http.Request) {
	result := TopNSDeployResult2367{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsDeps := make(map[string]int)
	for _, dep := range depList.Items {
		nsDeps[dep.Namespace]++
	}
	result.Summary.TotalNS = len(nsDeps)
	for ns, count := range nsDeps {
		result.TopNS = append(result.TopNS, struct {
			Namespace   string `json:"namespace"`
			DeployCount int    `json:"deployCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].DeployCount > result.TopNS[j].DeployCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCapStorageResult2367 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeCapStorage2367(w http.ResponseWriter, r *http.Request) {
	result := NodeCapStorageResult2367{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Storage().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocGB += node.Status.Allocatable.Storage().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NetPolDensityResult2367 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNetPols int            `json:"totalNetPols"`
		ByNS         map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNetPolDensity2367(w http.ResponseWriter, r *http.Request) {
	result := NetPolDensityResult2367{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNetPols++
		result.Summary.ByNS[np.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
