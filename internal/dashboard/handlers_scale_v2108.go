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
// v21.08 — Scalability & HA Dimension (Round 37)
// 1. Memory Request Efficiency
// 2. Pod IP Allocation Rate
// 3. Deployment Replica Concentration
// ============================================================

type MemEffResult2108 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         MemEffSummary2108 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type MemEffSummary2108 struct {
	AllocatableMem float64 `json:"allocatableMemGB"`
	RequestedMem   float64 `json:"requestedMemGB"`
	EfficiencyPct  int     `json:"efficiencyPct"`
}

func (s *Server) handleMemEff2108(w http.ResponseWriter, r *http.Request) {
	result := MemEffResult2108{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableMem += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.RequestedMem += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.AllocatableMem > 0 {
		result.Summary.EfficiencyPct = int(result.Summary.RequestedMem / result.Summary.AllocatableMem * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod IP Allocation Rate
type IPAllocResult2108 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         IPAllocSummary2108 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type IPAllocSummary2108 struct {
	TotalPods int `json:"totalPods"`
	WithIP    int `json:"podsWithIP"`
}

func (s *Server) handleIPAlloc2108(w http.ResponseWriter, r *http.Request) {
	result := IPAllocResult2108{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.PodIP != "" {
			result.Summary.WithIP++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Deployment Replica Concentration
type ReplicaConcResult2108 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ReplicaConcSummary2108 `json:"summary"`
	TopReplica      []ReplicaConcEntry2108 `json:"topReplicaDeployments"`
	Recommendations []string               `json:"recommendations"`
}

type ReplicaConcSummary2108 struct {
	TotalDeploys  int   `json:"totalDeployments"`
	TotalReplicas int32 `json:"totalReplicas"`
}

type ReplicaConcEntry2108 struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func (s *Server) handleReplicaConc2108(w http.ResponseWriter, r *http.Request) {
	result := ReplicaConcResult2108{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		result.Summary.TotalReplicas += replicas
		if replicas >= 10 {
			result.TopReplica = append(result.TopReplica, ReplicaConcEntry2108{Name: dep.Name, Replicas: replicas})
		}
	}
	sort.Slice(result.TopReplica, func(i, j int) bool { return result.TopReplica[i].Replicas > result.TopReplica[j].Replicas })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if len(result.TopReplica) > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments with >=10 replicas — review scaling", len(result.TopReplica)))
	}
	writeJSON(w, result)
}
