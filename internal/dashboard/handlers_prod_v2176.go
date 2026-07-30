package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.76 — Product Dimension (Round 49)
// 1. Pod Container Working Set Estimator
// 2. Service Selector Target Match
// 3. Namespace Active Deadline Tracker
// ============================================================

type WorkingSetResult2176 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		AvgCtnrPerPod int `json:"avgContainersPerPod"`
		MaxCtnrPerPod int `json:"maxContainersPerPod"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleWorkingSet2176(w http.ResponseWriter, r *http.Request) {
	result := WorkingSetResult2176{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	maxC := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		c := len(pod.Spec.Containers)
		if c > maxC {
			maxC = c
		}
	}
	result.Summary.MaxCtnrPerPod = maxC
	if result.Summary.TotalPods > 0 {
		// Calculate average from all containers
		totalC := 0
		for _, pod := range podList.Items {
			if pod.Status.Phase == corev1.PodRunning {
				totalC += len(pod.Spec.Containers)
			}
		}
		result.Summary.AvgCtnrPerPod = totalC / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Service Selector Target Match
type SelTargetResult2176 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithSelector  int `json:"withSelector"`
		NoSelector    int `json:"noSelector"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSelTarget2176(w http.ResponseWriter, r *http.Request) {
	result := SelTargetResult2176{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if len(svc.Spec.Selector) > 0 {
			result.Summary.WithSelector++
		} else {
			result.Summary.NoSelector++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NS Active Deadline Tracker
type NSActiveDeadlineResult2176 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS       int `json:"totalNamespaces"`
		ActiveNS      int `json:"activeNamespaces"`
		TerminatingNS int `json:"terminatingNamespaces"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSActiveDeadline2176(w http.ResponseWriter, r *http.Request) {
	result := NSActiveDeadlineResult2176{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if ns.Status.Phase == corev1.NamespaceActive {
			result.Summary.ActiveNS++
		} else {
			result.Summary.TerminatingNS++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
