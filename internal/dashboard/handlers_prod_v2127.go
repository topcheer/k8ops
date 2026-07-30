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
// v21.27 — Product Dimension (Round 41)
// 1. Pod Active Deadline Audit
// 2. Service IP Family Distribution
// 3. Workload Label Diversity Score
// ============================================================

type ActiveDeadlineResult2127 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         ActiveDeadlineSummary2127 `json:"summary"`
	Recommendations []string                  `json:"recommendations"`
}

type ActiveDeadlineSummary2127 struct {
	TotalPods    int `json:"totalPods"`
	WithDeadline int `json:"withActiveDeadline"`
}

func (s *Server) handleActiveDeadline2127(w http.ResponseWriter, r *http.Request) {
	result := ActiveDeadlineResult2127{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ActiveDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Service IP Family
type IPFamilyResult2127 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         IPFamilySummary2127 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type IPFamilySummary2127 struct {
	TotalServices int            `json:"totalServices"`
	ByIPFamily    map[string]int `json:"byIPFamily"`
}

func (s *Server) handleIPFamily2127(w http.ResponseWriter, r *http.Request) {
	result := IPFamilyResult2127{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	byIF := make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if len(svc.Spec.IPFamilies) > 0 {
			for _, f := range svc.Spec.IPFamilies {
				byIF[string(f)]++
			}
		} else {
			byIF["IPv4"]++
		}
	}
	result.Summary.ByIPFamily = byIF
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Label Diversity Score
type LabelDivScoreResult2127 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         LabelDivScoreSummary2127 `json:"summary"`
	TopLabels       []LabelDivScoreEntry2127 `json:"topLabels"`
	Recommendations []string                 `json:"recommendations"`
}

type LabelDivScoreSummary2127 struct {
	TotalDeploys int `json:"totalDeployments"`
	UniqueLabels int `json:"uniqueLabelKeys"`
}

type LabelDivScoreEntry2127 struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func (s *Server) handleLabelDivScore2127(w http.ResponseWriter, r *http.Request) {
	result := LabelDivScoreResult2127{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	labelCount := make(map[string]int)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		for k := range dep.Spec.Template.Labels {
			labelCount[k]++
		}
	}
	result.Summary.UniqueLabels = len(labelCount)

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range labelCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for i, s2 := range sorted {
		if i >= 10 {
			break
		}
		result.TopLabels = append(result.TopLabels, LabelDivScoreEntry2127{Label: s2.key, Count: s2.count})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.UniqueLabels < 3 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Only %d unique label keys — add more for better filtering", result.Summary.UniqueLabels))
	}
	writeJSON(w, result)
}
