package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.20 — Operations Dimension (Round 23)
// 1. Pod Phase Distribution — pod phase stats across cluster
// 2. Container Restart Reason — restart reason categorization
// 3. Node Kubelet Version — kubelet version distribution & drift
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Phase Distribution
// ---------------------------------------------------------------

type PhaseDistResult2020 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PhaseDistSummary2020 `json:"summary"`
	PerNS           []PhaseDistEntry2020 `json:"perNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type PhaseDistSummary2020 struct {
	TotalPods int `json:"totalPods"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
	Unknown   int `json:"unknown"`
}

type PhaseDistEntry2020 struct {
	Namespace string `json:"namespace"`
	Running   int    `json:"running"`
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
}

func (s *Server) handlePhaseDist(w http.ResponseWriter, r *http.Request) {
	result := PhaseDistResult2020{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsStats := make(map[string]*PhaseDistEntry2020)

	for _, pod := range podList.Items {
		result.Summary.TotalPods++

		phase := pod.Status.Phase
		switch phase {
		case corev1.PodRunning:
			result.Summary.Running++
		case corev1.PodPending:
			result.Summary.Pending++
		case corev1.PodFailed:
			result.Summary.Failed++
			score -= 3
		case corev1.PodSucceeded:
			result.Summary.Succeeded++
		default:
			result.Summary.Unknown++
		}

		entry, ok := nsStats[pod.Namespace]
		if !ok {
			entry = &PhaseDistEntry2020{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = entry
		}
		switch phase {
		case corev1.PodRunning:
			entry.Running++
		case corev1.PodPending:
			entry.Pending++
		case corev1.PodFailed:
			entry.Failed++
		}
	}

	for _, e := range nsStats {
		result.PerNS = append(result.PerNS, *e)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].Running+result.PerNS[i].Pending+result.PerNS[i].Failed >
			result.PerNS[j].Running+result.PerNS[j].Pending+result.PerNS[j].Failed
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d running, %d pending, %d failed, %d succeeded", result.Summary.TotalPods, result.Summary.Running, result.Summary.Pending, result.Summary.Failed, result.Summary.Succeeded))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Restart Reason
// ---------------------------------------------------------------

type RestReasonResult2020 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         RestReasonSummary2020 `json:"summary"`
	Reasons         []RestReasonEntry2020 `json:"reasons"`
	Recommendations []string              `json:"recommendations"`
}

type RestReasonSummary2020 struct {
	TotalRestarts int `json:"totalRestarts"`
	OOMKilled     int `json:"oomKilledCount"`
	Exited        int `json:"exitedCount"`
	Unknown       int `json:"unknownReasonCount"`
}

type RestReasonEntry2020 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (s *Server) handleRestReason(w http.ResponseWriter, r *http.Request) {
	result := RestReasonResult2020{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	reasonMap := make(map[string]int)

	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			totalR := int(cs.RestartCount)
			result.Summary.TotalRestarts += totalR

			if cs.LastTerminationState.Terminated != nil {
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "" {
					reason = "Unknown"
				}
				reasonMap[reason] += totalR

				switch reason {
				case "OOMKilled":
					result.Summary.OOMKilled += totalR
				case "Completed", "Error":
					result.Summary.Exited += totalR
				default:
					result.Summary.Unknown += totalR
				}
			} else if totalR > 0 {
				reasonMap["Unknown"] += totalR
				result.Summary.Unknown += totalR
			}
		}
	}

	for reason, count := range reasonMap {
		result.Reasons = append(result.Reasons, RestReasonEntry2020{Reason: reason, Count: count})
	}
	sort.Slice(result.Reasons, func(i, j int) bool {
		return result.Reasons[i].Count > result.Reasons[j].Count
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total restarts: %d OOMKilled, %d exited, %d unknown", result.Summary.TotalRestarts, result.Summary.OOMKilled, result.Summary.Exited, result.Summary.Unknown))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Kubelet Version
// ---------------------------------------------------------------

type KubeletVerResult2020 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         KubeletVerSummary2020 `json:"summary"`
	Versions        []KubeletVerEntry2020 `json:"versions"`
	Recommendations []string              `json:"recommendations"`
}

type KubeletVerSummary2020 struct {
	TotalNodes     int    `json:"totalNodes"`
	UniqueVersions int    `json:"uniqueVersions"`
	LatestVersion  string `json:"latestVersion"`
	DriftLevel     string `json:"driftLevel"`
}

type KubeletVerEntry2020 struct {
	Version string `json:"version"`
	Count   int    `json:"nodeCount"`
}

func (s *Server) handleKubeletVer(w http.ResponseWriter, r *http.Request) {
	result := KubeletVerResult2020{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	versionMap := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		ver := node.Status.NodeInfo.KubeletVersion
		if ver == "" {
			ver = "unknown"
		}
		versionMap[ver]++
	}

	result.Summary.UniqueVersions = len(versionMap)

	// Find latest version
	latestVer := ""
	maxCount := 0
	for ver, count := range versionMap {
		result.Versions = append(result.Versions, KubeletVerEntry2020{Version: ver, Count: count})
		if count > maxCount || latestVer == "" {
			maxCount = count
			latestVer = ver
		}
	}
	result.Summary.LatestVersion = latestVer

	if result.Summary.UniqueVersions > 2 {
		result.Summary.DriftLevel = "high"
		score -= 5
	} else if result.Summary.UniqueVersions > 1 {
		result.Summary.DriftLevel = "low"
	} else {
		result.Summary.DriftLevel = "none"
	}

	sort.Slice(result.Versions, func(i, j int) bool {
		return result.Versions[i].Count > result.Versions[j].Count
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d unique versions, latest: %s, drift: %s", result.Summary.TotalNodes, result.Summary.UniqueVersions, result.Summary.LatestVersion, result.Summary.DriftLevel))
	_ = strings.Builder{}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
