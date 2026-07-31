package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.91 Scalability: Top Namespace by Event, Node Allocatable Mem, Cluster Pod by Controller
type TopNSEventResult2391 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace  string `json:"namespace"`
		EventCount int    `json:"eventCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSEvent2391(w http.ResponseWriter, r *http.Request) {
	result := TopNSEventResult2391{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	nsEvents := make(map[string]int)
	for _, evt := range eventList.Items {
		nsEvents[evt.Namespace]++
	}
	result.Summary.TotalNS = len(nsEvents)
	for ns, count := range nsEvents {
		result.TopNS = append(result.TopNS, struct {
			Namespace  string `json:"namespace"`
			EventCount int    `json:"eventCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].EventCount > result.TopNS[j].EventCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocMemResult2391 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalMemGB float64 `json:"totalAllocatableMemGB"`
		AvgPerNode float64 `json:"avgPerNodeGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocMem2391(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocMemResult2391{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalMemGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalMemGB / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodByCtrlResult2391 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int            `json:"totalPods"`
		ByController map[string]int `json:"byControllerKind"`
	} `json:"summary"`
}

func (s *Server) handlePodByCtrl2391(w http.ResponseWriter, r *http.Request) {
	result := PodByCtrlResult2391{ScannedAt: time.Now()}
	result.Summary.ByController = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		kind := "standalone"
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				kind = ref.Kind
				break
			}
		}
		result.Summary.ByController[kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
