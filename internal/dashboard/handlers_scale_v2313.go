package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.13 Scalability: Namespace Limit vs Request Balance, Node Ephemeral Storage Usage, Cluster Replica Total
type NSLimitBalanceResult2313 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int `json:"totalNS"`
		Balanced int `json:"balanced"`
		NoLimits int `json:"withoutLimits"`
	} `json:"summary"`
}

func (s *Server) handleNSLimitBalance2313(w http.ResponseWriter, r *http.Request) {
	result := NSLimitBalanceResult2313{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsReq := make(map[string]bool)
	nsLimit := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				nsReq[pod.Namespace] = true
			}
			if !c.Resources.Limits.Cpu().IsZero() {
				nsLimit[pod.Namespace] = true
			}
		}
	}
	allNS := make(map[string]bool)
	for ns := range nsReq {
		allNS[ns] = true
	}
	for ns := range nsLimit {
		allNS[ns] = true
	}
	for ns := range allNS {
		result.Summary.TotalNS++
		if nsReq[ns] && nsLimit[ns] {
			result.Summary.Balanced++
		}
		if nsReq[ns] && !nsLimit[ns] {
			result.Summary.NoLimits++
		}
	}
	score := 100
	if result.Summary.TotalNS > 0 {
		score = result.Summary.Balanced * 100 / result.Summary.TotalNS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeEphemeralResult2313 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeEphemeral2313(w http.ResponseWriter, r *http.Request) {
	result := NodeEphemeralResult2313{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.StorageEphemeral().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ReplicaTotalResult2313 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		DeployReplicas int32 `json:"deploymentReplicas"`
		STSReplicas    int32 `json:"stsReplicas"`
		DSReplicas     int32 `json:"dsReplicas"`
		TotalReplicas  int32 `json:"totalReplicas"`
	} `json:"summary"`
}

func (s *Server) handleReplicaTotal2313(w http.ResponseWriter, r *http.Request) {
	result := ReplicaTotalResult2313{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.DeployReplicas += dep.Status.Replicas
	}
	for _, sts := range stsList.Items {
		result.Summary.STSReplicas += sts.Status.Replicas
	}
	for _, ds := range dsList.Items {
		result.Summary.DSReplicas += ds.Status.DesiredNumberScheduled
	}
	result.Summary.TotalReplicas = result.Summary.DeployReplicas + result.Summary.STSReplicas + result.Summary.DSReplicas
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
