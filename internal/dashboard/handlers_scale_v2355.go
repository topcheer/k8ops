package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.55 Scalability: Top Namespace by ConfigMap, Node CPU Allocatable Core, Cluster StatefulSet Density
type TopNSCMResult2355 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		CMCount   int    `json:"configMapCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSCM2355(w http.ResponseWriter, r *http.Request) {
	result := TopNSCMResult2355{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	nsCMs := make(map[string]int)
	for _, cm := range cmList.Items {
		nsCMs[cm.Namespace]++
	}
	result.Summary.TotalNS = len(nsCMs)
	for ns, count := range nsCMs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			CMCount   int    `json:"configMapCount"`
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

type NodeCPUAllocResult2355 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCPU   int `json:"totalAllocatableCPUCores"`
		AvgPerNode int `json:"avgCPUPerNode"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUAlloc2355(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocResult2355{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPU += int(node.Status.Allocatable.Cpu().Value())
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalCPU / result.Summary.TotalNodes
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSDensityResult2355 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleSTSDensity2355(w http.ResponseWriter, r *http.Request) {
	result := STSDensityResult2355{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByNS[sts.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
