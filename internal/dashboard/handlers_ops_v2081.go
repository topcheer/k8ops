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
// v20.81 — Operations Dimension (Round 33)
// 1. Pod Phase Distribution — running/pending/failed ratio
// 2. Kubelet Version Drift — node kubelet version consistency
// 3. Container Port Conflict Detector — duplicate hostPort bindings
// ============================================================

type PhaseDistResult2081 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PhaseDistSummary2081 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type PhaseDistSummary2081 struct {
	Total     int `json:"totalPods"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
}

func (s *Server) handlePhaseDist2081(w http.ResponseWriter, r *http.Request) {
	result := PhaseDistResult2081{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		result.Summary.Total++
		switch pod.Status.Phase {
		case corev1.PodRunning:
			result.Summary.Running++
		case corev1.PodPending:
			result.Summary.Pending++
			score -= 2
		case corev1.PodFailed:
			result.Summary.Failed++
			score -= 5
		case corev1.PodSucceeded:
			result.Summary.Succeeded++
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Failed > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d failed pods — investigate and clean up", result.Summary.Failed))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Kubelet Version Drift
// ---------------------------------------------------------------

type KVDriftResult2081 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         KVDriftSummary2081 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type KVDriftSummary2081 struct {
	TotalNodes int            `json:"totalNodes"`
	Versions   map[string]int `json:"kubeletVersions"`
}

func (s *Server) handleKVDrift2081(w http.ResponseWriter, r *http.Request) {
	result := KVDriftResult2081{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	versions := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		versions[node.Status.NodeInfo.KubeletVersion]++
	}
	result.Summary.Versions = versions

	if len(versions) > 1 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if len(versions) > 1 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d different kubelet versions — plan node upgrades", len(versions)))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Port Conflict Detector
// ---------------------------------------------------------------

type PortConflictResult2081 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PortConflictSummary2081 `json:"summary"`
	Conflicts       []PortConflictEntry2081 `json:"conflicts"`
	Recommendations []string                `json:"recommendations"`
}

type PortConflictSummary2081 struct {
	TotalHostPorts int `json:"totalHostPorts"`
	Conflicts      int `json:"conflicts"`
}

type PortConflictEntry2081 struct {
	Port      int32  `json:"port"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePortConflict2081(w http.ResponseWriter, r *http.Request) {
	result := PortConflictResult2081{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	portMap := make(map[int32][]string)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, p := range c.Ports {
				if p.HostPort != 0 {
					result.Summary.TotalHostPorts++
					portMap[p.HostPort] = append(portMap[p.HostPort], pod.Namespace+"/"+pod.Name)
				}
			}
		}
	}

	for port, users := range portMap {
		if len(users) > 1 {
			result.Summary.Conflicts++
			result.Conflicts = append(result.Conflicts, PortConflictEntry2081{Port: port, Namespace: users[0]})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Conflicts, func(i, j int) bool { return result.Conflicts[i].Port < result.Conflicts[j].Port })

	if result.Summary.Conflicts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d hostPort conflicts detected", result.Summary.Conflicts))
	}
	writeJSON(w, result)
}
