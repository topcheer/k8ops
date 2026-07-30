package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.53 — Operations Dimension (Round 45)
// 1. Node System Info Summary
// 2. Pod QoS BestEffort Count Tracker
// 3. Container Volume Name Consistency
// ============================================================

type SysInfoResult2153 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SysInfoSummary2153 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type SysInfoSummary2153 struct {
	TotalNodes       int    `json:"totalNodes"`
	KubeletVersion   string `json:"kubeletVersion"`
	ContainerRuntime string `json:"containerRuntime"`
}

func (s *Server) handleSysInfo2153(w http.ResponseWriter, r *http.Request) {
	result := SysInfoResult2153{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if result.Summary.KubeletVersion == "" {
			result.Summary.KubeletVersion = node.Status.NodeInfo.KubeletVersion
			result.Summary.ContainerRuntime = node.Status.NodeInfo.ContainerRuntimeVersion
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. BestEffort Count
type BestEffortResult2153 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         BestEffortSummary2153 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type BestEffortSummary2153 struct {
	TotalPods  int `json:"totalPods"`
	BestEffort int `json:"bestEffortPods"`
}

func (s *Server) handleBestEffort2153(w http.ResponseWriter, r *http.Request) {
	result := BestEffortResult2153{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		allReq := false
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				allReq = true
			}
		}
		if !allReq {
			result.Summary.BestEffort++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Volume Name Consistency
type VolNameConsResult2153 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         VolNameConsSummary2153 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type VolNameConsSummary2153 struct {
	TotalPods      int `json:"totalPods"`
	TotalVolMounts int `json:"totalVolumeMounts"`
	UnnamedMounts  int `json:"unnamedMounts"`
}

func (s *Server) handleVolNameCons2153(w http.ResponseWriter, r *http.Request) {
	result := VolNameConsResult2153{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			for _, vm := range c.VolumeMounts {
				result.Summary.TotalVolMounts++
				if vm.Name == "" {
					result.Summary.UnnamedMounts++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
