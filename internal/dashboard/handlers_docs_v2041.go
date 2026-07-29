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
// v20.41 — Documentation Dimension (Round 26)
// 1. Namespace Catalog — namespace purpose & annotation inventory
// 2. LimitRange Policy Doc — resource limit range documentation
// 3. Event Frequency Heatmap — event reason frequency distribution
// ============================================================

// ---------------------------------------------------------------
// 1. Namespace Catalog
// ---------------------------------------------------------------

type NSCatalogResult2041 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NSCatalogSummary2041 `json:"summary"`
	Namespaces      []NSCatalogEntry2041 `json:"namespaces"`
	Recommendations []string             `json:"recommendations"`
}

type NSCatalogSummary2041 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	ActiveNS        int `json:"activeNS"`
	EmptyNS         int `json:"emptyNS"`
	SystemNS        int `json:"systemNS"`
}

type NSCatalogEntry2041 struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	PodCount int    `json:"podCount"`
	AgeDays  int    `json:"ageDays"`
}

func (s *Server) handleNSCatalog(w http.ResponseWriter, r *http.Request) {
	result := NSCatalogResult2041{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podCountPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCountPerNS[pod.Namespace]++
		}
	}

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
		"k8ops-system": true,
	}

	now := time.Now()
	for _, ns := range nsList.Items {
		result.Summary.TotalNamespaces++
		pods := podCountPerNS[ns.Name]

		entry := NSCatalogEntry2041{
			Name: ns.Name, Status: string(ns.Status.Phase),
			PodCount: pods,
			AgeDays:  int(now.Sub(ns.CreationTimestamp.Time).Hours() / 24),
		}

		if systemNS[ns.Name] {
			result.Summary.SystemNS++
		} else if pods > 0 {
			result.Summary.ActiveNS++
		} else {
			result.Summary.EmptyNS++
			if ns.Status.Phase == corev1.NamespaceActive {
				score -= 1
			}
		}

		result.Namespaces = append(result.Namespaces, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Namespaces, func(i, j int) bool {
		return result.Namespaces[i].PodCount > result.Namespaces[j].PodCount
	})

	if result.Summary.EmptyNS > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d empty namespaces — clean up unused namespaces", result.Summary.EmptyNS))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. LimitRange Policy Doc
// ---------------------------------------------------------------

type LimitRangeDocResult2041 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         LimitRangeDocSummary2041 `json:"summary"`
	LimitRanges     []LimitRangeDocEntry2041 `json:"limitRanges"`
	Recommendations []string                 `json:"recommendations"`
}

type LimitRangeDocSummary2041 struct {
	TotalNS       int `json:"totalNamespaces"`
	NSWithLimits  int `json:"nsWithLimitRanges"`
	WithoutLimits int `json:"nsWithoutLimitRanges"`
}

type LimitRangeDocEntry2041 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	MaxCPU    string `json:"maxCpu,omitempty"`
	MaxMem    string `json:"maxMem,omitempty"`
}

func (s *Server) handleLimitRangeDoc(w http.ResponseWriter, r *http.Request) {
	result := LimitRangeDocResult2041{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	lrList, _ := s.clientset.CoreV1().LimitRanges("").List(r.Context(), metav1.ListOptions{})

	nsWithLR := make(map[string]bool)
	for _, lr := range lrList.Items {
		nsWithLR[lr.Namespace] = true
		entry := LimitRangeDocEntry2041{
			Name: lr.Name, Namespace: lr.Namespace,
		}
		for _, item := range lr.Spec.Limits {
			if item.Type == corev1.LimitTypeContainer {
				if maxCPU := item.Max[corev1.ResourceCPU]; !maxCPU.IsZero() {
					entry.MaxCPU = maxCPU.String()
				}
				if maxMem := item.Max[corev1.ResourceMemory]; !maxMem.IsZero() {
					entry.MaxMem = maxMem.String()
				}
			}
		}
		result.LimitRanges = append(result.LimitRanges, entry)
	}

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsWithLR[ns.Name] {
			result.Summary.NSWithLimits++
		} else {
			result.Summary.WithoutLimits++
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.LimitRanges, func(i, j int) bool {
		return result.LimitRanges[i].Namespace < result.LimitRanges[j].Namespace
	})

	if result.Summary.WithoutLimits > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces without LimitRange — add default resource limits", result.Summary.WithoutLimits))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event Frequency Heatmap
// ---------------------------------------------------------------

type EventFreqResult2041 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EventFreqSummary2041 `json:"summary"`
	TopReasons      []EventFreqEntry2041 `json:"topReasons"`
	Recommendations []string             `json:"recommendations"`
}

type EventFreqSummary2041 struct {
	TotalEvents   int `json:"totalEvents"`
	UniqueReasons int `json:"uniqueReasons"`
	WarningEvents int `json:"warningEvents"`
}

type EventFreqEntry2041 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	Type   string `json:"type"`
}

func (s *Server) handleEventFreqHeatmap(w http.ResponseWriter, r *http.Request) {
	result := EventFreqResult2041{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	reasonCount := make(map[string]int)
	reasonType := make(map[string]string)

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		reason := evt.Reason
		if reason == "" {
			reason = "Unknown"
		}
		reasonCount[reason]++
		reasonType[reason] = string(evt.Type)

		if evt.Type == corev1.EventTypeWarning {
			result.Summary.WarningEvents++
		}
	}

	result.Summary.UniqueReasons = len(reasonCount)

	// Sort and take top 15
	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range reasonCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for i, s2 := range sorted {
		if i >= 15 {
			break
		}
		result.TopReasons = append(result.TopReasons, EventFreqEntry2041{
			Reason: s2.key, Count: s2.count, Type: reasonType[s2.key],
		})
	}

	if result.Summary.WarningEvents > result.Summary.TotalEvents/2 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WarningEvents > 100 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d warning events — investigate recurring issues", result.Summary.WarningEvents))
	}

	writeJSON(w, result)
}
