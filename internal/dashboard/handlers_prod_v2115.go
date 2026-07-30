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
// v21.15 — Product Dimension (Round 39)
// 1. Pod Priority Class Distribution
// 2. Service Account Token Volume Audit
// 3. Workload Revision History Limit
// ============================================================

type PriorityClassResult2115 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         PriorityClassSummary2115 `json:"summary"`
	Recommendations []string                 `json:"recommendations"`
}

type PriorityClassSummary2115 struct {
	TotalPods  int            `json:"totalPods"`
	ByPriority map[string]int `json:"byPriorityClass"`
}

func (s *Server) handlePriorityClass2115(w http.ResponseWriter, r *http.Request) {
	result := PriorityClassResult2115{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byPC := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "none"
		}
		byPC[pc]++
	}
	result.Summary.ByPriority = byPC
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. SA Token Volume Audit
type SATokenVolResult2115 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenVolSummary2115 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenVolSummary2115 struct {
	TotalSAs     int `json:"totalServiceAccounts"`
	WithTokenVol int `json:"withTokenVolume"`
}

func (s *Server) handleSATokenVol2115(w http.ResponseWriter, r *http.Request) {
	result := SATokenVolResult2115{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.Secrets) > 0 {
			result.Summary.WithTokenVol++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Revision History Limit
type RevHistLimitResult2115 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RevHistLimitSummary2115 `json:"summary"`
	Unlimited       []RevHistLimitEntry2115 `json:"unlimitedHistory"`
	Recommendations []string                `json:"recommendations"`
}

type RevHistLimitSummary2115 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithLimit    int `json:"withRevisionHistoryLimit"`
}

type RevHistLimitEntry2115 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleRevHistLimit2115(w http.ResponseWriter, r *http.Request) {
	result := RevHistLimitResult2115{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.RevisionHistoryLimit != nil {
			result.Summary.WithLimit++
		} else {
			result.Unlimited = append(result.Unlimited, RevHistLimitEntry2115{Name: dep.Name, Namespace: dep.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Unlimited, func(i, j int) bool { return result.Unlimited[i].Namespace < result.Unlimited[j].Namespace })

	if len(result.Unlimited) > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments without revisionHistoryLimit — set for cleanup", len(result.Unlimited)))
	}
	writeJSON(w, result)
}
