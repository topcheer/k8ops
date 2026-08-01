package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.81 Scalability: Top Namespace by ConfigMap, Node CPU Allocatable vs Capacity, Cluster NetworkPolicy Total
type TopNSByCMResult2481 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		CMCount   int    `json:"cmCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByCM2481(w http.ResponseWriter, r *http.Request) {
	result := TopNSByCMResult2481{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	nsCMs := make(map[string]int)
	for _, cm := range cmList.Items {
		nsCMs[cm.Namespace]++
	}
	result.Summary.TotalNS = len(nsCMs)
	for ns, count := range nsCMs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			CMCount   int    `json:"cmCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CMCount > result.TopNS[j].CMCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUVsCapResult2481 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCPUCapacity"`
		TotalAlloc float64 `json:"totalCPUAllocatable"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUVsCap2481(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUVsCapResult2481{ScannedAt: time.Now()}
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

type NetPolicyTotalResult2481 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP int            `json:"totalNetworkPolicies"`
		ByNS    map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNetPolicyTotal2481(w http.ResponseWriter, r *http.Request) {
	result := NetPolicyTotalResult2481{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		result.Summary.ByNS[np.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
