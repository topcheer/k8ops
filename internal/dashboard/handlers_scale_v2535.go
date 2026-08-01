package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.35 Scalability: Top Namespace by DaemonSet, Node Memory Capacity vs Allocatable, Cluster Event Type Distribution
type TopNSByDSResult2535 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		DSCount   int    `json:"dsCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByDS2535(w http.ResponseWriter, r *http.Request) {
	result := TopNSByDSResult2535{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	nsDS := make(map[string]int)
	for _, ds := range dsList.Items {
		nsDS[ds.Namespace]++
	}
	result.Summary.TotalNS = len(nsDS)
	for ns, count := range nsDS {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			DSCount   int    `json:"dsCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].DSCount > result.TopNS[j].DSCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemCapVsAllocResult2535 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCapacityGB"`
		TotalAlloc float64 `json:"totalAllocatableGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemCapVsAlloc2535(w http.ResponseWriter, r *http.Request) {
	result := NodeMemCapVsAllocResult2535{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += node.Status.Capacity.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		result.Summary.TotalAlloc += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventTypeDistResult2535 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByType      map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleEventTypeDist2535(w http.ResponseWriter, r *http.Request) {
	result := EventTypeDistResult2535{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, ev := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByType[string(ev.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
