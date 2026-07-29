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
// v20.45 — Operations Dimension (Round 27)
// 1. Pod QPS Estimate — service-level request rate estimate
// 2. Log Volume Anomaly — high log-generating pods
// 3. Node Condition Budget — node condition count vs healthy
// ============================================================

// ---------------------------------------------------------------
// 1. Pod QPS Estimate
// ---------------------------------------------------------------

type PodQPSResult2045 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PodQPSSummary2045 `json:"summary"`
	HighQPSPods     []PodQPSEntry2045 `json:"highQPSPods"`
	Recommendations []string          `json:"recommendations"`
}

type PodQPSSummary2045 struct {
	TotalPods   int `json:"totalPods"`
	HighQPSPods int `json:"highQPSPods"`
	TotalEstQPS int `json:"totalEstQPS"`
}

type PodQPSEntry2045 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	EstQPS    int    `json:"estimatedQPS"`
}

func (s *Server) handlePodQPSEstimate(w http.ResponseWriter, r *http.Request) {
	result := PodQPSResult2045{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	// Estimate QPS from event generation rate per pod
	podEventCount := make(map[string]int)
	now := time.Now()
	for _, evt := range eventList.Items {
		ageHours := now.Sub(evt.CreationTimestamp.Time).Hours()
		if ageHours < 1 {
			ageHours = 1
		}
		key := evt.Namespace + "/" + evt.InvolvedObject.Name
		podEventCount[key]++
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		key := pod.Namespace + "/" + pod.Name
		events := podEventCount[key]
		// Rough QPS estimate: events per hour as proxy
		estQPS := events / 3600

		if estQPS > 10 {
			result.Summary.HighQPSPods++
			result.HighQPSPods = append(result.HighQPSPods, PodQPSEntry2045{
				Pod: pod.Name, Namespace: pod.Namespace, EstQPS: estQPS,
			})
			score -= 2
		}
		result.Summary.TotalEstQPS += estQPS
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HighQPSPods, func(i, j int) bool {
		return result.HighQPSPods[i].EstQPS > result.HighQPSPods[j].EstQPS
	})

	if result.Summary.HighQPSPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have high estimated activity — monitor for performance impact", result.Summary.HighQPSPods))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Log Volume Anomaly
// ---------------------------------------------------------------

type LogVolResult2045 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         LogVolSummary2045 `json:"summary"`
	NoisyPods       []LogVolEntry2045 `json:"noisyPods"`
	Recommendations []string          `json:"recommendations"`
}

type LogVolSummary2045 struct {
	TotalPods int `json:"totalPods"`
	NoisyPods int `json:"noisyPods"`
}

type LogVolEntry2045 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Restarts  int32  `json:"restarts"`
	Events    int    `json:"recentEvents"`
}

func (s *Server) handleLogVolAnomaly(w http.ResponseWriter, r *http.Request) {
	result := LogVolResult2045{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	podEventCount := make(map[string]int)
	for _, evt := range eventList.Items {
		key := evt.Namespace + "/" + evt.InvolvedObject.Name
		podEventCount[key]++
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}

		key := pod.Namespace + "/" + pod.Name
		events := podEventCount[key]

		// Noisy = high restarts OR high event count
		if restarts > 10 || events > 100 {
			result.Summary.NoisyPods++
			result.NoisyPods = append(result.NoisyPods, LogVolEntry2045{
				Pod: pod.Name, Namespace: pod.Namespace,
				Restarts: restarts, Events: events,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.NoisyPods, func(i, j int) bool {
		return result.NoisyPods[i].Events > result.NoisyPods[j].Events
	})

	if result.Summary.NoisyPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d noisy pods generating high log volume — investigate root cause", result.Summary.NoisyPods))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Condition Budget
// ---------------------------------------------------------------

type NodeCondBudgetResult2045 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         NodeCondBudgetSummary2045 `json:"summary"`
	NodesWithIssues []NodeCondBudgetEntry2045 `json:"nodesWithIssues"`
	Recommendations []string                  `json:"recommendations"`
}

type NodeCondBudgetSummary2045 struct {
	TotalNodes      int `json:"totalNodes"`
	HealthyNodes    int `json:"healthyNodes"`
	NodesWithIssues int `json:"nodesWithIssues"`
	TotalConditions int `json:"totalConditions"`
}

type NodeCondBudgetEntry2045 struct {
	Node       string   `json:"node"`
	Conditions []string `json:"conditions"`
}

func (s *Server) handleNodeCondBudget2045(w http.ResponseWriter, r *http.Request) {
	result := NodeCondBudgetResult2045{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		conditions := []string{}
		healthy := true

		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				continue
			}
			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				conditions = append(conditions, string(cond.Type))
				healthy = false
			}
			if cond.Type != corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				conditions = append(conditions, string(cond.Type))
				healthy = false
			}
		}

		result.Summary.TotalConditions += len(conditions)

		if healthy {
			result.Summary.HealthyNodes++
		} else {
			result.Summary.NodesWithIssues++
			result.NodesWithIssues = append(result.NodesWithIssues, NodeCondBudgetEntry2045{
				Node: node.Name, Conditions: conditions,
			})
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NodesWithIssues > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have health conditions — check node status", result.Summary.NodesWithIssues))
	}

	writeJSON(w, result)
}
