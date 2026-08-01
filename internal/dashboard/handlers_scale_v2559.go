package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.59 Scalability: Top Namespace by STS Replicas, Node CPU Capacity Detail, Cluster Deploy Total
type TopNSBySTSRepResult2559 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		RepCount  int    `json:"replicaCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySTSRep2559(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySTSRepResult2559{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	nsReps := make(map[string]int)
	for _, sts := range stsList.Items {
		if sts.Spec.Replicas != nil {
			nsReps[sts.Namespace] += int(*sts.Spec.Replicas)
		}
	}
	result.Summary.TotalNS = len(nsReps)
	for ns, count := range nsReps {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			RepCount  int    `json:"replicaCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].RepCount > result.TopNS[j].RepCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUCapDetailResult2559 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCPUCapCores"`
		TotalAlloc float64 `json:"totalCPUAllocCores"`
	}
}

func (s *Server) handleNodeCPUCapDetail2559(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUCapDetailResult2559{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployTotal2559Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int            `json:"totalDeployments"`
		ByNS         map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleDeployTotal2559(w http.ResponseWriter, r *http.Request) {
	result := DeployTotal2559Result{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.ByNS[dep.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
