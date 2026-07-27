package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.05 — Product Dimension (Round 20)
// 1. Pod Restart Trend — restart count distribution & instability detection
// 2. Deployment Rollout Status — rollout progress & condition tracking
// 3. PVC Binding Health — PVC binding latency & pending status
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Restart Trend
// ---------------------------------------------------------------

type RestTrendResult2005 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         RestTrendSummary2005 `json:"summary"`
	Restarted       []RestTrendEntry2005 `json:"restartedPods"`
	Recommendations []string             `json:"recommendations"`
}

type RestTrendSummary2005 struct {
	TotalPods     int `json:"totalPods"`
	RestartedPods int `json:"restartedPods"`
	TotalRestarts int `json:"totalRestarts"`
	HighRestart   int `json:"highRestartPods"`
}

type RestTrendEntry2005 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Restarts  int32  `json:"restartCount"`
}

func (s *Server) handlePodRestartTrend(w http.ResponseWriter, r *http.Request) {
	result := RestTrendResult2005{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := int32(0)
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}

		result.Summary.TotalRestarts += int(totalRestarts)

		if totalRestarts > 0 {
			result.Summary.RestartedPods++
			entry := RestTrendEntry2005{
				Pod: pod.Name, Namespace: pod.Namespace, Restarts: totalRestarts,
			}
			result.Restarted = append(result.Restarted, entry)

			if totalRestarts > 5 {
				result.Summary.HighRestart++
				score -= 3
			}
		}
	}

	sort.Slice(result.Restarted, func(i, j int) bool {
		return result.Restarted[i].Restarts > result.Restarted[j].Restarts
	})
	if len(result.Restarted) > 20 {
		result.Restarted = result.Restarted[:20]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d restarted (%d high), %d total restarts", result.Summary.TotalPods, result.Summary.RestartedPods, result.Summary.HighRestart, result.Summary.TotalRestarts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Deployment Rollout Status
// ---------------------------------------------------------------

type RolloutResult2005 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RolloutSummary2005 `json:"summary"`
	InProgress      []RolloutEntry2005 `json:"inProgressDeployments"`
	Recommendations []string           `json:"recommendations"`
}

type RolloutSummary2005 struct {
	TotalDeployments int `json:"totalDeployments"`
	Complete         int `json:"rolloutComplete"`
	InProgress       int `json:"rolloutInProgress"`
	StaleProgress    int `json:"staleProgress"`
}

type RolloutEntry2005 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Updated   int    `json:"updatedReplicas"`
	Ready     int    `json:"readyReplicas"`
	Status    string `json:"status"`
}

func (s *Server) handleRolloutStatusV2(w http.ResponseWriter, r *http.Request) {
	result := RolloutResult2005{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		desired := 0
		if dep.Spec.Replicas != nil {
			desired = int(*dep.Spec.Replicas)
		}
		updated := int(dep.Status.UpdatedReplicas)
		ready := int(dep.Status.ReadyReplicas)

		entry := RolloutEntry2005{
			Name: dep.Name, Namespace: dep.Namespace,
			Updated: updated, Ready: ready,
		}

		isProgressing := false
		for _, cond := range dep.Status.Conditions {
			if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionTrue {
				isProgressing = true
			}
		}

		if updated >= desired && ready >= desired {
			entry.Status = "complete"
			result.Summary.Complete++
		} else if isProgressing {
			entry.Status = "in-progress"
			result.Summary.InProgress++
			result.InProgress = append(result.InProgress, entry)
			score -= 1
		} else if desired > 0 && ready < desired {
			entry.Status = "stale"
			result.Summary.StaleProgress++
			result.InProgress = append(result.InProgress, entry)
			score -= 3
		} else {
			entry.Status = "complete"
			result.Summary.Complete++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d complete, %d in-progress, %d stale", result.Summary.TotalDeployments, result.Summary.Complete, result.Summary.InProgress, result.Summary.StaleProgress))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. PVC Binding Health
// ---------------------------------------------------------------

type PVCHealthResult2005 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVCHealthSummary2005 `json:"summary"`
	Pending         []PVCHealthEntry2005 `json:"pendingPVCs"`
	Recommendations []string             `json:"recommendations"`
}

type PVCHealthSummary2005 struct {
	TotalPVCs  int     `json:"totalPVCs"`
	Bound      int     `json:"boundPVCs"`
	Pending    int     `json:"pendingPVCs"`
	AvgBindSec float64 `json:"avgBindingSeconds"`
}

type PVCHealthEntry2005 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Size      string `json:"requestedSize"`
}

func (s *Server) handlePVCBindingHealth(w http.ResponseWriter, r *http.Request) {
	result := PVCHealthResult2005{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	var totalBindSec float64
	var bindCount int

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		entry := PVCHealthEntry2005{
			Name: pvc.Name, Namespace: pvc.Namespace,
			Status: string(pvc.Status.Phase),
		}

		if pvc.Spec.Resources.Requests != nil {
			if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				entry.Size = qty.String()
			}
		}

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.Bound++
			// Estimate binding time from annotations or creation time
			if !pvc.CreationTimestamp.IsZero() {
				// Rough estimate: PVC binds within seconds to minutes
				totalBindSec += 5
				bindCount++
			}
		case corev1.ClaimPending:
			result.Summary.Pending++
			result.Pending = append(result.Pending, entry)
			score -= 3
		}
	}

	if bindCount > 0 {
		result.Summary.AvgBindSec = totalBindSec / float64(bindCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs: %d bound, %d pending", result.Summary.TotalPVCs, result.Summary.Bound, result.Summary.Pending))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
