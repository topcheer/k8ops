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
// v20.78 — Scalability & HA Dimension (Round 32)
// 1. Pod Scheduling Score — scheduling latency estimate
// 2. Resource Fragmentation — node resource fragmentation analysis
// 3. Multi-Zone HA Validator — cross-zone workload distribution
// ============================================================

type SchedScoreResult2078 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SchedScoreSummary2078 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type SchedScoreSummary2078 struct {
	TotalNodes   int  `json:"totalNodes"`
	TotalPods    int  `json:"totalPods"`
	PendingPods  int  `json:"pendingPods"`
	SchedulingOK bool `json:"schedulingOK"`
}

func (s *Server) handleSchedScore2078(w http.ResponseWriter, r *http.Request) {
	result := SchedScoreResult2078{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNodes = len(nodeList.Items)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		} else if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			score -= 5
		}
	}
	result.Summary.SchedulingOK = result.Summary.PendingPods == 0
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.PendingPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pending pods — scheduling may be constrained", result.Summary.PendingPods))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Resource Fragmentation
// ---------------------------------------------------------------

type FragResult2078 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         FragSummary2078 `json:"summary"`
	Recommendations []string        `json:"recommendations"`
}

type FragSummary2078 struct {
	TotalNodes      int `json:"totalNodes"`
	FragmentedNodes int `json:"fragmentedNodes"`
}

func (s *Server) handleFragAnalysis2078(w http.ResponseWriter, r *http.Request) {
	result := FragResult2078{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nodeUsage := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			nodeUsage[pod.Spec.NodeName]++
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		maxPods := 110.0
		if pods != nil && !pods.IsZero() {
			maxPods = pods.AsApproximateFloat64()
		}
		usage := nodeUsage[node.Name] / maxPods
		// Fragmented: very low usage means wasted capacity
		if usage < 0.1 && result.Summary.TotalNodes > 1 {
			result.Summary.FragmentedNodes++
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.FragmentedNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes with <10%% utilization — consolidate workloads", result.Summary.FragmentedNodes))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Multi-Zone HA Validator
// ---------------------------------------------------------------

type MZHAResult2078 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         MZHASummary2078 `json:"summary"`
	Recommendations []string        `json:"recommendations"`
}

type MZHASummary2078 struct {
	TotalNodes int      `json:"totalNodes"`
	Zones      []string `json:"zones"`
	SingleZone bool     `json:"singleZoneOnly"`
}

func (s *Server) handleMZHAValidator(w http.ResponseWriter, r *http.Request) {
	result := MZHAResult2078{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	zoneSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = node.Labels["failure-domain.beta.kubernetes.io/zone"]
		}
		if zone != "" {
			zoneSet[zone] = true
		}
	}

	for z := range zoneSet {
		result.Summary.Zones = append(result.Summary.Zones, z)
	}
	sort.Strings(result.Summary.Zones)
	result.Summary.SingleZone = len(zoneSet) <= 1

	if result.Summary.SingleZone && result.Summary.TotalNodes > 1 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.SingleZone {
		result.Recommendations = append(result.Recommendations,
			"Single-zone cluster — no zone redundancy for HA")
	}
	writeJSON(w, result)
}
