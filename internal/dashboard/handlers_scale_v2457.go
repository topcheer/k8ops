package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.57 Scalability: Top Namespace by Memory Request, Node Pod Pressure Ratio, Cluster Event Total
type TopNSByMemResult2457 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		MemReqMB  float64 `json:"memReqMB"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByMem2457(w http.ResponseWriter, r *http.Request) {
	result := TopNSByMemResult2457{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		var mem float64
		for _, c := range pod.Spec.Containers {
			mem += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
		nsMem[pod.Namespace] += mem
	}
	result.Summary.TotalNS = len(nsMem)
	for ns, mem := range nsMem {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			MemReqMB  float64 `json:"memReqMB"`
		}{ns, mem})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReqMB > result.TopNS[j].MemReqMB })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodPressureResult2457 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int     `json:"totalNodes"`
		AvgUsagePct float64 `json:"avgPodUsagePercent"`
	} `json:"summary"`
}

func (s *Server) handleNodePodPressure2457(w http.ResponseWriter, r *http.Request) {
	result := NodePodPressureResult2457{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePodCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nodePodCount[pod.Spec.NodeName]++
		}
	}
	var totalUsage float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Allocatable.Pods().Value()
		if cap > 0 {
			totalUsage += float64(nodePodCount[node.Name]) / float64(cap) * 100
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgUsagePct = totalUsage / float64(result.Summary.TotalNodes)
	}
	score := 100 - int(result.Summary.AvgUsagePct)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type EventTotalResult2457 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByType      map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleEventTotal2457(w http.ResponseWriter, r *http.Request) {
	result := EventTotalResult2457{ScannedAt: time.Now()}
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
