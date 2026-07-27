package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.08 — Operations Dimension (Round 21)
// 1. Pod Taint Toleration Match — pod-taint scheduling compliance
// 2. Node Condition Budget — node condition health tracking
// 3. Cluster Log Volume — estimated log output per namespace
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Taint Toleration Match
// ---------------------------------------------------------------

type TaintMatchResult2008 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         TaintMatchSummary2008     `json:"summary"`
	PerNode         []TaintMatchNodeEntry2008 `json:"perNode"`
	Recommendations []string                  `json:"recommendations"`
}

type TaintMatchSummary2008 struct {
	TotalNodes         int `json:"totalNodes"`
	NodesWithTaints    int `json:"nodesWithTaints"`
	TotalPods          int `json:"totalPods"`
	PodsWithToleration int `json:"podsWithToleration"`
}

type TaintMatchNodeEntry2008 struct {
	Node     string   `json:"node"`
	Taints   []string `json:"taints"`
	PodCount int      `json:"podCount"`
}

func (s *Server) handleTaintTolMatch(w http.ResponseWriter, r *http.Request) {
	result := TaintMatchResult2008{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count pods per node
	podsPerNode := make(map[string]int)
	tolCount := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
		if len(pod.Spec.Tolerations) > 0 {
			tolCount++
		}
	}
	result.Summary.PodsWithToleration = tolCount

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		var taints []string
		for _, taint := range node.Spec.Taints {
			taints = append(taints, taint.Key+"="+taint.Value+":"+string(taint.Effect))
		}

		if len(taints) > 0 {
			result.Summary.NodesWithTaints++
		}

		result.PerNode = append(result.PerNode, TaintMatchNodeEntry2008{
			Node: node.Name, Taints: taints, PodCount: podsPerNode[node.Name],
		})
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes (%d tainted), %d pods (%d with tolerations)", result.Summary.TotalNodes, result.Summary.NodesWithTaints, result.Summary.TotalPods, result.Summary.PodsWithToleration))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Condition Budget
// ---------------------------------------------------------------

type NodeCondResult2008 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeCondSummary2008 `json:"summary"`
	PerNode         []NodeCondEntry2008 `json:"perNode"`
	Recommendations []string            `json:"recommendations"`
}

type NodeCondSummary2008 struct {
	TotalNodes             int `json:"totalNodes"`
	HealthyNodes           int `json:"healthyNodes"`
	WithDiskPressure       int `json:"nodesWithDiskPressure"`
	WithMemPressure        int `json:"nodesWithMemPressure"`
	WithPIDPressure        int `json:"nodesWithPIDPressure"`
	WithNetworkUnavailable int `json:"nodesWithNetworkUnavailable"`
}

type NodeCondEntry2008 struct {
	Name   string   `json:"name"`
	Ready  bool     `json:"ready"`
	Issues []string `json:"issues"`
}

func (s *Server) handleNodeCondBudget(w http.ResponseWriter, r *http.Request) {
	result := NodeCondResult2008{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		entry := NodeCondEntry2008{Name: node.Name}
		isHealthy := true

		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeReady:
				entry.Ready = true
			case corev1.NodeDiskPressure:
				entry.Issues = append(entry.Issues, "DiskPressure")
				result.Summary.WithDiskPressure++
				isHealthy = false
				score -= 5
			case corev1.NodeMemoryPressure:
				entry.Issues = append(entry.Issues, "MemoryPressure")
				result.Summary.WithMemPressure++
				isHealthy = false
				score -= 5
			case corev1.NodePIDPressure:
				entry.Issues = append(entry.Issues, "PIDPressure")
				result.Summary.WithPIDPressure++
				isHealthy = false
				score -= 5
			case corev1.NodeNetworkUnavailable:
				entry.Issues = append(entry.Issues, "NetworkUnavailable")
				result.Summary.WithNetworkUnavailable++
				isHealthy = false
				score -= 5
			}
		}

		if isHealthy && entry.Ready {
			result.Summary.HealthyNodes++
		}

		result.PerNode = append(result.PerNode, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d healthy, %d disk-pressure, %d mem-pressure, %d pid-pressure", result.Summary.TotalNodes, result.Summary.HealthyNodes, result.Summary.WithDiskPressure, result.Summary.WithMemPressure, result.Summary.WithPIDPressure))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Log Volume
// ---------------------------------------------------------------

type LogVolResult2008 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         LogVolSummary2008 `json:"summary"`
	PerNS           []LogVolEntry2008 `json:"perNamespace"`
	Recommendations []string          `json:"recommendations"`
}

type LogVolSummary2008 struct {
	TotalPods      int     `json:"totalPods"`
	EstLogMBPerDay float64 `json:"estLogMBPerDay"`
	HighLogNS      int     `json:"highLogNamespaces"`
}

type LogVolEntry2008 struct {
	Namespace      string  `json:"namespace"`
	PodCount       int     `json:"podCount"`
	EstLogMBPerDay float64 `json:"estLogMBPerDay"`
}

func (s *Server) handleClusterLogVol(w http.ResponseWriter, r *http.Request) {
	result := LogVolResult2008{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Estimate: ~50MB log per pod per day for active workloads
	const estLogPerPod = 50.0
	nsStats := make(map[string]int)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		nsStats[pod.Namespace]++
	}

	var totalLog float64
	for ns, count := range nsStats {
		estLog := float64(count) * estLogPerPod
		totalLog += estLog

		entry := LogVolEntry2008{Namespace: ns, PodCount: count, EstLogMBPerDay: estLog}
		if estLog > 1000 {
			result.Summary.HighLogNS++
		}
		result.PerNS = append(result.PerNS, entry)
	}
	result.Summary.EstLogMBPerDay = totalLog

	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EstLogMBPerDay > result.PerNS[j].EstLogMBPerDay
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	if totalLog > 10000 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, est %.0f MB/day log volume, %d high-log NS", result.Summary.TotalPods, result.Summary.EstLogMBPerDay, result.Summary.HighLogNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
