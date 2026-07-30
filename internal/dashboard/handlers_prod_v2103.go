package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.03 — Product Dimension (Round 37)
// 1. Pod Startup Probe Coverage
// 2. Service External Traffic Policy Audit
// 3. Namespace Finalizer Tracker
// ============================================================

type StartupProbeResult2103 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         StartupProbeSummary2103 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type StartupProbeSummary2103 struct {
	TotalContainers int `json:"totalContainers"`
	WithStartup     int `json:"withStartupProbe"`
}

func (s *Server) handleStartupProbe2103(w http.ResponseWriter, r *http.Request) {
	result := StartupProbeResult2103{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. External Traffic Policy
type ExtTrafficResult2103 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ExtTrafficSummary2103 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type ExtTrafficSummary2103 struct {
	TotalServices int `json:"totalServices"`
	ClusterPolicy int `json:"clusterPolicy"`
	LocalPolicy   int `json:"localPolicy"`
}

func (s *Server) handleExtTraffic2103(w http.ResponseWriter, r *http.Request) {
	result := ExtTrafficResult2103{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ExternalTrafficPolicy == "Local" {
			result.Summary.LocalPolicy++
		} else {
			result.Summary.ClusterPolicy++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Namespace Finalizer Tracker
type NSFinResult2103 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         NSFinSummary2103 `json:"summary"`
	StuckNS         []NSFinEntry2103 `json:"stuckNamespaces"`
	Recommendations []string         `json:"recommendations"`
}

type NSFinSummary2103 struct {
	TotalNS int `json:"totalNamespaces"`
	StuckNS int `json:"stuckNamespaces"`
}

type NSFinEntry2103 struct {
	Name       string   `json:"name"`
	Finalizers []string `json:"finalizers"`
}

func (s *Server) handleNSFin2103(w http.ResponseWriter, r *http.Request) {
	result := NSFinResult2103{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Spec.Finalizers) > 0 {
			result.Summary.StuckNS++
			finalizers := make([]string, len(ns.Spec.Finalizers))
			for i, f := range ns.Spec.Finalizers {
				finalizers[i] = string(f)
			}
			result.StuckNS = append(result.StuckNS, NSFinEntry2103{Name: ns.Name, Finalizers: finalizers})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.StuckNS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces with finalizers — may block deletion", result.Summary.StuckNS))
	}
	writeJSON(w, result)
}
