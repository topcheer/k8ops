package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.17 Scalability: Top Namespace by STS, Node Pod Count vs Capacity, Cluster ControllerRevision Count
type TopNSBySTSResult2517 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		STSCount  int    `json:"stsCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySTS2517(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySTSResult2517{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	nsSTS := make(map[string]int)
	for _, sts := range stsList.Items {
		nsSTS[sts.Namespace]++
	}
	result.Summary.TotalNS = len(nsSTS)
	for ns, count := range nsSTS {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			STSCount  int    `json:"stsCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].STSCount > result.TopNS[j].STSCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodVsCapResult2517 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalPods  int `json:"totalRunningPods"`
		TotalCap   int `json:"totalPodCapacity"`
	} `json:"summary"`
}

func (s *Server) handleNodePodVsCap2517(w http.ResponseWriter, r *http.Request) {
	result := NodePodVsCapResult2517{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	podCount := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCount++
		}
	}
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += int(node.Status.Capacity.Pods().Value())
	}
	result.Summary.TotalPods = podCount
	score := 100
	if result.Summary.TotalCap > 0 {
		used := result.Summary.TotalPods * 100 / result.Summary.TotalCap
		score = 100 - used
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ControllerRevResult2517 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs int            `json:"totalControllerRevisions"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleControllerRev2517(w http.ResponseWriter, r *http.Request) {
	result := ControllerRevResult2517{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	crList, _ := s.clientset.AppsV1().ControllerRevisions("").List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		result.Summary.ByNS[cr.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
