package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.53 Scalability: Top Namespace by Event Count, Node Storage Allocatable, Cluster STS Total
type TopNSByEvtResult2553 struct {
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

func (s *Server) handleTopNSByEvt2553(w http.ResponseWriter, r *http.Request) {
	result := TopNSByEvtResult2553{ScannedAt: time.Now()}
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

type NodeStorAllocResult2553 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalStorAllocatableGB"`
	}
}

func (s *Server) handleNodeStorAlloc2553(w http.ResponseWriter, r *http.Request) {
	result := NodeStorAllocResult2553{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSTotal2553Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByNS     map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleSTSTotal2553(w http.ResponseWriter, r *http.Request) {
	result := STSTotal2553Result{ScannedAt: time.Now()}
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
