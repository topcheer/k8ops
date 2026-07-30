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
// v21.22 — Deployment Dimension (Round 40)
// 1. Pod OS Selector Audit
// 2. Container Resource Limit Request Ratio
// 3. Deployment Max Surge Efficiency
// ============================================================

type OSSelectorResult2122 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         OSSelectorSummary2122 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type OSSelectorSummary2122 struct {
	TotalPods int `json:"totalPods"`
	WithOSSel int `json:"withOSSelector"`
}

func (s *Server) handleOSSelector2122(w http.ResponseWriter, r *http.Request) {
	result := OSSelectorResult2122{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil && pod.Spec.OS.Name != "" {
			result.Summary.WithOSSel++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Limit Request Ratio
type LimReqResult2122 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         LimReqSummary2122 `json:"summary"`
	Unbounded       []LimReqEntry2122 `json:"unboundedContainers"`
	Recommendations []string          `json:"recommendations"`
}

type LimReqSummary2122 struct {
	TotalContainers int `json:"totalContainers"`
	Balanced        int `json:"balanced"`
	Unbounded       int `json:"unbounded"`
}

type LimReqEntry2122 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleLimReq2122(w http.ResponseWriter, r *http.Request) {
	result := LimReqResult2122{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			hasReq := !c.Resources.Requests.Cpu().IsZero()
			hasLim := !c.Resources.Limits.Cpu().IsZero()
			if hasReq && hasLim {
				result.Summary.Balanced++
			} else if !hasReq && !hasLim {
				result.Summary.Unbounded++
				result.Unbounded = append(result.Unbounded, LimReqEntry2122{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Unbounded, func(i, j int) bool { return result.Unbounded[i].Namespace < result.Unbounded[j].Namespace })

	if result.Summary.Unbounded > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with no requests or limits", result.Summary.Unbounded))
	}
	writeJSON(w, result)
}

// 3. Max Surge Efficiency
type SurgeEffResult2122 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SurgeEffSummary2122 `json:"summary"`
	HighSurge       []SurgeEffEntry2122 `json:"highSurge"`
	Recommendations []string            `json:"recommendations"`
}

type SurgeEffSummary2122 struct {
	TotalDeploys int `json:"totalDeployments"`
	DefaultSurge int `json:"defaultSurge"`
	HighSurge    int `json:"highSurge"`
}

type SurgeEffEntry2122 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSurgeEff2122(w http.ResponseWriter, r *http.Request) {
	result := SurgeEffResult2122{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			ms := dep.Spec.Strategy.RollingUpdate.MaxSurge
			if ms.IntVal > 5 {
				result.Summary.HighSurge++
				result.HighSurge = append(result.HighSurge, SurgeEffEntry2122{Name: dep.Name, Namespace: dep.Namespace})
			} else {
				result.Summary.DefaultSurge++
			}
		} else {
			result.Summary.DefaultSurge++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.HighSurge, func(i, j int) bool { return result.HighSurge[i].Namespace < result.HighSurge[j].Namespace })
	writeJSON(w, result)
}
