package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.47 Scalability: Top Namespace by ConfigMap, Node Pod Usage Ratio, Cluster ReplicaSet Total
type TopNSByCM2Result2547 struct {
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

func (s *Server) handleTopNSByCM2Result2547(w http.ResponseWriter, r *http.Request) {
	result := TopNSByCM2Result2547{ScannedAt: time.Now()}
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

type NodePodUsageRatioResult2547 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgRatio   float64 `json:"avgPodUsageRatio"`
	} `json:"summary"`
}

func (s *Server) handleNodePodUsageRatio2547(w http.ResponseWriter, r *http.Request) {
	result := NodePodUsageRatioResult2547{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nodePods[pod.Spec.NodeName]++
		}
	}
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Allocatable.Pods().Value()
		if cap > 0 {
			totalRatio += float64(nodePods[node.Name]) / float64(cap)
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgRatio = totalRatio / float64(result.Summary.TotalNodes) * 100
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RSTotal2547Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS int            `json:"totalRS"`
		ByNS    map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleRSTotal2547(w http.ResponseWriter, r *http.Request) {
	result := RSTotal2547Result{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.ByNS[rs.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
