package dashboard

import (
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.31 — Documentation Dimension (Round 41)
// 1. Event Reason Top 10 Catalog
// 2. Node Instance Type Distribution
// 3. PV Reclaim Policy Diversity
// ============================================================

type EventReasonResult2131 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         EventReasonSummary2131 `json:"summary"`
	TopReasons      []EventReasonEntry2131 `json:"topReasons"`
	Recommendations []string               `json:"recommendations"`
}

type EventReasonSummary2131 struct {
	TotalEvents int `json:"totalEvents"`
}

type EventReasonEntry2131 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (s *Server) handleEventReason2131(w http.ResponseWriter, r *http.Request) {
	result := EventReasonResult2131{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	reasonCount := make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		reason := evt.Reason
		if reason == "" {
			reason = "Unknown"
		}
		reasonCount[reason]++
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range reasonCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for i, s2 := range sorted {
		if i >= 10 {
			break
		}
		result.TopReasons = append(result.TopReasons, EventReasonEntry2131{Reason: s2.key, Count: s2.count})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Instance Type
type InstanceTypeResult2131 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         InstanceTypeSummary2131 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type InstanceTypeSummary2131 struct {
	TotalNodes     int            `json:"totalNodes"`
	ByInstanceType map[string]int `json:"byInstanceType"`
}

func (s *Server) handleInstanceType2131(w http.ResponseWriter, r *http.Request) {
	result := InstanceTypeResult2131{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	byIT := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		it := node.Labels["node.kubernetes.io/instance-type"]
		if it == "" {
			it = "unknown"
		}
		byIT[it]++
	}
	result.Summary.ByInstanceType = byIT
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PV Reclaim Diversity
type PVReclaimDivResult2131 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PVReclaimDivSummary2131 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type PVReclaimDivSummary2131 struct {
	TotalPVs int            `json:"totalPVs"`
	ByPolicy map[string]int `json:"byReclaimPolicy"`
}

func (s *Server) handlePVReclaimDiv2131(w http.ResponseWriter, r *http.Request) {
	result := PVReclaimDivResult2131{ScannedAt: time.Now()}
	score := 100
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})

	byPol := make(map[string]int)
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		byPol[string(pv.Spec.PersistentVolumeReclaimPolicy)]++
	}
	result.Summary.ByPolicy = byPol
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
