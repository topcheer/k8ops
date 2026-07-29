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
// v20.68 — Deployment Dimension (Round 31)
// 1. RollingUpdate Window Inspector — surge/unavailable config analysis
// 2. Pod Priority Preemption Tracker — high-priority pod eviction tracking
// 3. StatefulSet Partition Audit — rolling update partition compliance
// ============================================================

type RollingWindowResult2068 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         RollingWindowSummary2068 `json:"summary"`
	Issues          []RollingWindowEntry2068 `json:"issues"`
	Recommendations []string                 `json:"recommendations"`
}

type RollingWindowSummary2068 struct {
	TotalDeploys  int `json:"totalDeployments"`
	DefaultConfig int `json:"defaultConfig"`
	CustomConfig  int `json:"customConfig"`
}

type RollingWindowEntry2068 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	MaxSurge  string `json:"maxSurge"`
}

func (s *Server) handleRollingWindowIns2068(w http.ResponseWriter, r *http.Request) {
	result := RollingWindowResult2068{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		if dep.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
			result.Summary.DefaultConfig++
			continue
		}

		ru := dep.Spec.Strategy.RollingUpdate
		if ru == nil {
			result.Summary.DefaultConfig++
			continue
		}

		result.Summary.CustomConfig++
		surgeStr := "25%"
		if ru.MaxSurge != nil {
			surgeStr = ru.MaxSurge.String()
		}

		// High surge with many replicas can overwhelm nodes
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if ru.MaxSurge != nil && ru.MaxSurge.IntVal > int32(replicas) && replicas > 3 {
			result.Issues = append(result.Issues, RollingWindowEntry2068{
				Name: dep.Name, Namespace: dep.Namespace, MaxSurge: surgeStr,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Namespace < result.Issues[j].Namespace })

	if len(result.Issues) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments with high maxSurge — may overwhelm during rollout", len(result.Issues)))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Priority Preemption Tracker
// ---------------------------------------------------------------

type PreemptResult2068 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PreemptSummary2068 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type PreemptSummary2068 struct {
	TotalPods    int `json:"totalPods"`
	HighPriority int `json:"highPriorityPods"`
	LowPriority  int `json:"lowPriorityPods"`
	NoPriority   int `json:"noPriorityClass"`
}

func (s *Server) handlePreemptTracker(w http.ResponseWriter, r *http.Request) {
	result := PreemptResult2068{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if pod.Spec.PriorityClassName == "" {
			result.Summary.NoPriority++
		} else if pod.Spec.Priority != nil {
			if *pod.Spec.Priority > 1000000 {
				result.Summary.HighPriority++
			} else if *pod.Spec.Priority < 1000 {
				result.Summary.LowPriority++
			}
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NoPriority > 50 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods without priority class — consider setting for scheduling", result.Summary.NoPriority))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. StatefulSet Partition Audit
// ---------------------------------------------------------------

type SSPartitionResult2068 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SSPartitionSummary2068 `json:"summary"`
	PartitionedSTS  []SSPartitionEntry2068 `json:"partitionedStatefulSets"`
	Recommendations []string               `json:"recommendations"`
}

type SSPartitionSummary2068 struct {
	TotalSTS     int `json:"totalStatefulSets"`
	Partitioned  int `json:"partitionedSets"`
	StuckRollout int `json:"stuckRollouts"`
}

type SSPartitionEntry2068 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Partition int32  `json:"partition"`
}

func (s *Server) handleSSPartitionAudit(w http.ResponseWriter, r *http.Request) {
	result := SSPartitionResult2068{ScannedAt: time.Now()}
	score := 100

	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++

		ru := sts.Spec.UpdateStrategy.RollingUpdate
		if ru != nil && ru.Partition != nil && *ru.Partition > 0 {
			partition := *ru.Partition
			result.Summary.Partitioned++
			result.PartitionedSTS = append(result.PartitionedSTS, SSPartitionEntry2068{
				Name: sts.Name, Namespace: sts.Namespace, Partition: partition,
			})

			replicas := int32(1)
			if sts.Spec.Replicas != nil {
				replicas = *sts.Spec.Replicas
			}
			if partition >= replicas {
				result.Summary.StuckRollout++
				score -= 5
			} else {
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Partitioned > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d StatefulSets with active partition — rollout may be incomplete", result.Summary.Partitioned))
	}
	writeJSON(w, result)
}
