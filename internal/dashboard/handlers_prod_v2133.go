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
// v21.33 — Product Dimension (Round 42)
// 1. Pod ServiceAccount Token Automount Override
// 2. Ingress Path Type Audit
// 3. PVC Finalizer Tracker
// ============================================================

type SAOverrideResult2133 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SAOverrideSummary2133 `json:"summary"`
	Disabled        []SAOverrideEntry2133 `json:"disabledPods"`
	Recommendations []string              `json:"recommendations"`
}

type SAOverrideSummary2133 struct {
	TotalPods int `json:"totalPods"`
	Disabled  int `json:"tokenDisabled"`
}

type SAOverrideEntry2133 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSAOverride2133(w http.ResponseWriter, r *http.Request) {
	result := SAOverrideResult2133{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.AutomountServiceAccountToken != nil && !*pod.Spec.AutomountServiceAccountToken {
			result.Summary.Disabled++
			result.Disabled = append(result.Disabled, SAOverrideEntry2133{Pod: pod.Name, Namespace: pod.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Disabled, func(i, j int) bool { return result.Disabled[i].Namespace < result.Disabled[j].Namespace })
	writeJSON(w, result)
}

// 2. Ingress Path Type
type IngPathTypeResult2133 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         IngPathTypeSummary2133 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type IngPathTypeSummary2133 struct {
	TotalIngresses int            `json:"totalIngresses"`
	ByPathType     map[string]int `json:"byPathType"`
}

func (s *Server) handleIngPathType2133(w http.ResponseWriter, r *http.Request) {
	result := IngPathTypeResult2133{ScannedAt: time.Now()}
	score := 100
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	byPT := make(map[string]int)
	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				pt := ""
				if path.PathType != nil {
					pt = string(*path.PathType)
				}
				if pt == "" {
					pt = "default"
				}
				byPT[pt]++
			}
		}
	}
	result.Summary.ByPathType = byPT
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Finalizer Tracker
type PVCFinalizerResult2133 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PVCFinalizerSummary2133 `json:"summary"`
	StuckPVC        []PVCFinalizerEntry2133 `json:"stuckPVCs"`
	Recommendations []string                `json:"recommendations"`
}

type PVCFinalizerSummary2133 struct {
	TotalPVCs int `json:"totalPVCs"`
	StuckPVCs int `json:"stuckPVCs"`
}

type PVCFinalizerEntry2133 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePVCFinalizer2133(w http.ResponseWriter, r *http.Request) {
	result := PVCFinalizerResult2133{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if len(pvc.Finalizers) > 0 {
			result.Summary.StuckPVCs++
			result.StuckPVC = append(result.StuckPVC, PVCFinalizerEntry2133{Name: pvc.Name, Namespace: pvc.Namespace})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.StuckPVC, func(i, j int) bool { return result.StuckPVC[i].Namespace < result.StuckPVC[j].Namespace })

	if result.Summary.StuckPVCs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVCs with finalizers may block deletion", result.Summary.StuckPVCs))
	}
	writeJSON(w, result)
}
