package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.79 Scalability: Top Namespace by Replica, Node Memory Capacity, Cluster DaemonSet Spread
type TopNSReplicaResult2379 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		Replicas  int32  `json:"replicas"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSReplica2379(w http.ResponseWriter, r *http.Request) {
	result := TopNSReplicaResult2379{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsReps := make(map[string]int32)
	for _, dep := range depList.Items {
		nsReps[dep.Namespace] += dep.Status.Replicas
	}
	result.Summary.TotalNS = len(nsReps)
	for ns, reps := range nsReps {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			Replicas  int32  `json:"replicas"`
		}{ns, reps})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Replicas > result.TopNS[j].Replicas })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemCapResult2379 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCapGB float64 `json:"totalCapacityGB"`
		AvgPerNode float64 `json:"avgCapacityPerNodeGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemCap2379(w http.ResponseWriter, r *http.Request) {
	result := NodeMemCapResult2379{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalCapGB / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSSpreadResult2379 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		TotalDesired int `json:"totalDesired"`
		TotalReady   int `json:"totalReady"`
	} `json:"summary"`
}

func (s *Server) handleDSSpread2379(w http.ResponseWriter, r *http.Request) {
	result := DSSpreadResult2379{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalDesired += int(ds.Status.DesiredNumberScheduled)
		result.Summary.TotalReady += int(ds.Status.NumberReady)
	}
	score := 100
	if result.Summary.TotalDesired > 0 {
		score = result.Summary.TotalReady * 100 / result.Summary.TotalDesired
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
