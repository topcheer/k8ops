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
// v21.51 — Product Dimension (Round 45)
// 1. Pod ShareProcessNamespace Audit
// 2. Container Termination Message Policy
// 3. Service AppProtocol Coverage
// ============================================================

type ShareProcResult2151 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ShareProcSummary2151 `json:"summary"`
	Shared          []ShareProcEntry2151 `json:"sharedProcPods"`
	Recommendations []string             `json:"recommendations"`
}

type ShareProcSummary2151 struct {
	TotalPods  int `json:"totalPods"`
	SharedProc int `json:"shareProcessNamespace"`
}

type ShareProcEntry2151 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleShareProc2151(w http.ResponseWriter, r *http.Request) {
	result := ShareProcResult2151{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.SharedProc++
			result.Shared = append(result.Shared, ShareProcEntry2151{Pod: pod.Name, Namespace: pod.Namespace})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Shared, func(i, j int) bool { return result.Shared[i].Namespace < result.Shared[j].Namespace })

	if result.Summary.SharedProc > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods share process namespace", result.Summary.SharedProc))
	}
	writeJSON(w, result)
}

// 2. Termination Message Policy
type TermMsgResult2151 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         TermMsgSummary2151 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type TermMsgSummary2151 struct {
	TotalContainers int            `json:"totalContainers"`
	ByPolicy        map[string]int `json:"byPolicy"`
}

func (s *Server) handleTermMsg2151(w http.ResponseWriter, r *http.Request) {
	result := TermMsgResult2151{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byP := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			policy := "File"
			if c.TerminationMessagePolicy != "" {
				policy = string(c.TerminationMessagePolicy)
			}
			byP[policy]++
		}
	}
	result.Summary.ByPolicy = byP
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. AppProtocol Coverage
type AppProtoResult2151 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AppProtoSummary2151 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type AppProtoSummary2151 struct {
	TotalPorts   int `json:"totalPorts"`
	WithAppProto int `json:"withAppProtocol"`
}

func (s *Server) handleAppProto2151(w http.ResponseWriter, r *http.Request) {
	result := AppProtoResult2151{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			if p.AppProtocol != nil {
				result.Summary.WithAppProto++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
