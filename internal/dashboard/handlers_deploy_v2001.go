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
// v20.01 — Deployment Dimension (Round 20)
// 1. Share Process Namespace — PID namespace sharing audit
// 2. Pod Priority Audit — priority class assignment gap
// 3. Container SubPath — volume mount subPath compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Share Process Namespace
// ---------------------------------------------------------------

type ShareProcResult2001 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ShareProcSummary2001 `json:"summary"`
	Issues          []ShareProcEntry2001 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type ShareProcSummary2001 struct {
	TotalPods    int `json:"totalPods"`
	WithSharePID int `json:"withShareProcessNamespace"`
	Without      int `json:"withoutShare"`
}

type ShareProcEntry2001 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleShareProcNS(w http.ResponseWriter, r *http.Request) {
	result := ShareProcResult2001{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.WithSharePID++
			result.Issues = append(result.Issues, ShareProcEntry2001{
				Pod: pod.Name, Namespace: pod.Namespace,
			})
			score -= 3
		} else {
			result.Summary.Without++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d sharing PID namespace, %d isolated", result.Summary.TotalPods, result.Summary.WithSharePID, result.Summary.Without))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Priority Audit
// ---------------------------------------------------------------

type PodPrioResult2001 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodPrioSummary2001 `json:"summary"`
	Without         []PodPrioEntry2001 `json:"withoutPriorityClass"`
	Recommendations []string           `json:"recommendations"`
}

type PodPrioSummary2001 struct {
	TotalPods      int `json:"totalPods"`
	WithPC         int `json:"withPriorityClass"`
	WithoutPC      int `json:"withoutPriorityClass"`
	SystemCritical int `json:"systemCriticalPods"`
}

type PodPrioEntry2001 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePodPriorityAudit(w http.ResponseWriter, r *http.Request) {
	result := PodPrioResult2001{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		pcName := pod.Spec.PriorityClassName
		if pcName != "" {
			result.Summary.WithPC++
			if pod.Spec.Priority != nil && *pod.Spec.Priority >= 1000000 {
				result.Summary.SystemCritical++
			}
		} else {
			result.Summary.WithoutPC++
			// Only flag non-system pods
			if pod.Namespace != "kube-system" {
				result.Without = append(result.Without, PodPrioEntry2001{
					Pod: pod.Name, Namespace: pod.Namespace,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with priority class, %d without (%d system-critical)", result.Summary.TotalPods, result.Summary.WithPC, result.Summary.WithoutPC, result.Summary.SystemCritical))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container SubPath Audit
// ---------------------------------------------------------------

type SubPathResult2001 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SubPathSummary2001 `json:"summary"`
	Pods            []SubPathEntry2001 `json:"pods"`
	Recommendations []string           `json:"recommendations"`
}

type SubPathSummary2001 struct {
	TotalContainers int `json:"totalContainers"`
	WithSubPath     int `json:"withSubPathMount"`
	WithoutSubPath  int `json:"withoutSubPath"`
}

type SubPathEntry2001 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Volume    string `json:"volumeName"`
	SubPath   string `json:"subPath"`
}

func (s *Server) handleContainerSubPath(w http.ResponseWriter, r *http.Request) {
	result := SubPathResult2001{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			for _, vm := range c.VolumeMounts {
				if vm.SubPath != "" {
					result.Summary.WithSubPath++
					result.Pods = append(result.Pods, SubPathEntry2001{
						Pod: pod.Name, Namespace: pod.Namespace,
						Volume: vm.Name, SubPath: vm.SubPath,
					})
				} else {
					result.Summary.WithoutSubPath++
				}
			}
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with subPath mounts, %d without", result.Summary.TotalContainers, result.Summary.WithSubPath, result.Summary.WithoutSubPath))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
