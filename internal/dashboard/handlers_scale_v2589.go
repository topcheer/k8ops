package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.89 Scalability: Top Namespace by Event v2, Node CPU Allocatable Summary, Cluster ConfigMap Total
type TopNSByEvt2Result2589 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		EvtCount  int    `json:"eventCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByEvt2Result2589(w http.ResponseWriter, r *http.Request) {
	result := TopNSByEvt2Result2589{ScannedAt: time.Now()}
	evtList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	nsEvts := make(map[string]int)
	for _, ev := range evtList.Items {
		nsEvts[ev.Namespace]++
	}
	result.Summary.TotalNS = len(nsEvts)
	for ns, count := range nsEvts {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			EvtCount  int    `json:"eventCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].EvtCount > result.TopNS[j].EvtCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUAllocSummaryResult2589 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		MinAlloc   float64 `json:"minCPUAlloc"`
		MaxAlloc   float64 `json:"maxCPUAlloc"`
	}
}

func (s *Server) handleNodeCPUAllocSummary2589(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocSummaryResult2589{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		if result.Summary.MinAlloc == 0 || alloc < result.Summary.MinAlloc {
			result.Summary.MinAlloc = alloc
		}
		if alloc > result.Summary.MaxAlloc {
			result.Summary.MaxAlloc = alloc
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMTotal2589Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs int            `json:"totalConfigMaps"`
		ByNS     map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleCMTotal2589(w http.ResponseWriter, r *http.Request) {
	result := CMTotal2589Result{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.ByNS[cm.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
