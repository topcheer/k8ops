package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.93 Scalability: Top Namespace by Event, Node Allocatable Storage Total, Cluster PriorityClass Count
type TopNSByEventResult2493 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		EvtCount  int    `json:"eventCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByEvent2493(w http.ResponseWriter, r *http.Request) {
	result := TopNSByEventResult2493{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	nsEvents := make(map[string]int)
	for _, ev := range eventList.Items {
		nsEvents[ev.Namespace]++
	}
	result.Summary.TotalNS = len(nsEvents)
	for ns, count := range nsEvents {
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

type NodeAllocStorTotalResult2493 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalAllocatableStorageGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocStorTotal2493(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocStorTotalResult2493{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PriorityClassCountResult2493 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPCs  int `json:"totalPriorityClasses"`
		GlobalDef int `json:"globalDefaultCount"`
	} `json:"summary"`
}

func (s *Server) handlePriorityClassCount2493(w http.ResponseWriter, r *http.Request) {
	result := PriorityClassCountResult2493{ScannedAt: time.Now()}
	pcList, _ := s.clientset.SchedulingV1().PriorityClasses().List(r.Context(), metav1.ListOptions{})
	for _, pc := range pcList.Items {
		result.Summary.TotalPCs++
		if pc.GlobalDefault {
			result.Summary.GlobalDef++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
