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
// v21.21 — Product Dimension (Round 40) — MILESTONE: 1000+ endpoints
// 1. Pod Termination Grace Tracker
// 2. Service Internal Traffic Policy Audit
// 3. Namespace Lifecycle Age Distribution
// ============================================================

type TermGraceResult2121 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         TermGraceSummary2121 `json:"summary"`
	ShortGrace      []TermGraceEntry2121 `json:"shortGracePods"`
	Recommendations []string             `json:"recommendations"`
}

type TermGraceSummary2121 struct {
	TotalPods int `json:"totalPods"`
	DefaultGP int `json:"defaultGracePeriod"`
	CustomGP  int `json:"customGracePeriod"`
}

type TermGraceEntry2121 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleTermGrace2121(w http.ResponseWriter, r *http.Request) {
	result := TermGraceResult2121{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.TerminationGracePeriodSeconds == nil {
			result.Summary.DefaultGP++
		} else {
			result.Summary.CustomGP++
			if *pod.Spec.TerminationGracePeriodSeconds < 5 {
				result.ShortGrace = append(result.ShortGrace, TermGraceEntry2121{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.ShortGrace, func(i, j int) bool { return result.ShortGrace[i].Namespace < result.ShortGrace[j].Namespace })
	writeJSON(w, result)
}

// 2. Internal Traffic Policy
type IntTrafficResult2121 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         IntTrafficSummary2121 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type IntTrafficSummary2121 struct {
	TotalServices int `json:"totalServices"`
	ClusterPolicy int `json:"clusterPolicy"`
	LocalPolicy   int `json:"localPolicy"`
}

func (s *Server) handleIntTraffic2121(w http.ResponseWriter, r *http.Request) {
	result := IntTrafficResult2121{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.InternalTrafficPolicy != nil && *svc.Spec.InternalTrafficPolicy == "Local" {
			result.Summary.LocalPolicy++
		} else {
			result.Summary.ClusterPolicy++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NS Lifecycle Age
type NSAgeResult2121 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         NSAgeSummary2121 `json:"summary"`
	OldNS           []NSAgeEntry2121 `json:"oldNamespaces"`
	Recommendations []string         `json:"recommendations"`
}

type NSAgeSummary2121 struct {
	TotalNS int `json:"totalNamespaces"`
	OldNS   int `json:"oldNamespaces"`
}

type NSAgeEntry2121 struct {
	Name    string `json:"name"`
	AgeDays int    `json:"ageDays"`
}

func (s *Server) handleNSAge2121(w http.ResponseWriter, r *http.Request) {
	result := NSAgeResult2121{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		ageDays := int(now.Sub(ns.CreationTimestamp.Time).Hours() / 24)
		if ageDays > 365 {
			result.Summary.OldNS++
			result.OldNS = append(result.OldNS, NSAgeEntry2121{Name: ns.Name, AgeDays: ageDays})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OldNS, func(i, j int) bool { return result.OldNS[i].AgeDays > result.OldNS[j].AgeDays })

	if result.Summary.OldNS > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces older than 1 year", result.Summary.OldNS))
	}
	writeJSON(w, result)
}
