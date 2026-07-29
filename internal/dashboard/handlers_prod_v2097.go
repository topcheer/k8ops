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
// v20.97 — Product Dimension (Round 36)
// 1. Pod Waiting Reason Catalog — ImagePullBackOff/ErrImagePull
// 2. Service Endpoint Slice Count — slices per service
// 3. Container Lifecycle Hook Audit — preStop/postStart coverage
// ============================================================

type WaitReasonResult2097 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         WaitReasonSummary2097 `json:"summary"`
	WaitingPods     []WaitReasonEntry2097 `json:"waitingPods"`
	Recommendations []string              `json:"recommendations"`
}

type WaitReasonSummary2097 struct {
	TotalPods   int `json:"totalPods"`
	WaitingPods int `json:"waitingPods"`
}

type WaitReasonEntry2097 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}

func (s *Server) handleWaitReason2097(w http.ResponseWriter, r *http.Request) {
	result := WaitReasonResult2097{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				result.Summary.WaitingPods++
				result.WaitingPods = append(result.WaitingPods, WaitReasonEntry2097{
					Pod: pod.Name, Namespace: pod.Namespace, Reason: cs.State.Waiting.Reason,
				})
				score -= 3
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WaitingPods, func(i, j int) bool { return result.WaitingPods[i].Reason < result.WaitingPods[j].Reason })

	if result.Summary.WaitingPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers in waiting state", result.Summary.WaitingPods))
	}
	writeJSON(w, result)
}

// 2. Service Endpoint Slice Count
type EPSliceCntResult2097 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EPSliceCntSummary2097 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type EPSliceCntSummary2097 struct {
	TotalSlices int `json:"totalSlices"`
	TotalAddrs  int `json:"totalAddresses"`
}

func (s *Server) handleEPSliceCnt2097(w http.ResponseWriter, r *http.Request) {
	result := EPSliceCntResult2097{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})

	for _, eps := range epList.Items {
		result.Summary.TotalSlices++
		result.Summary.TotalAddrs += len(eps.Endpoints)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Container Lifecycle Hook Audit
type HookResult2097 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         HookSummary2097 `json:"summary"`
	Recommendations []string        `json:"recommendations"`
}

type HookSummary2097 struct {
	TotalContainers int `json:"totalContainers"`
	WithPreStop     int `json:"withPreStop"`
	WithPostStart   int `json:"withPostStart"`
}

func (s *Server) handleHook2097(w http.ResponseWriter, r *http.Request) {
	result := HookResult2097{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Lifecycle != nil {
				if c.Lifecycle.PreStop != nil {
					result.Summary.WithPreStop++
				}
				if c.Lifecycle.PostStart != nil {
					result.Summary.WithPostStart++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
