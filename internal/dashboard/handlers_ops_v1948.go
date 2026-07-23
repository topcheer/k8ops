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
// v19.48 — Operations Dimension (Round 11)
// 1. Container Log Volume Estimator — log throughput & storage pressure
// 2. Pod Eviction History — voluntary & forced eviction tracking
// 3. Kubelet Sync Latency — node status update freshness
// ============================================================

// ---------------------------------------------------------------
// 1. Container Log Volume Estimator
// ---------------------------------------------------------------

type LogVolumeResult1948 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         LogVolumeSummary1948 `json:"summary"`
	HighVolumePods  []LogVolumeEntry1948 `json:"highVolumePods"`
	ByNS            []LogVolumeNS1948    `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type LogVolumeSummary1948 struct {
	TotalContainers int     `json:"totalContainers"`
	WithLogLimits   int     `json:"withLogLimits"`
	WithoutLimits   int     `json:"withoutLogLimits"`
	HighLogVolPods  int     `json:"highLogVolumePods"`
	EstDailyGB      float64 `json:"estDailyLogGB"`
}

type LogVolumeEntry1948 struct {
	PodName    string  `json:"podName"`
	Namespace  string  `json:"namespace"`
	Container  string  `json:"container"`
	HasLimit   bool    `json:"hasLogLimit"`
	EstMBPerHr float64 `json:"estMBPerHour"`
}

type LogVolumeNS1948 struct {
	Namespace  string  `json:"namespace"`
	Containers int     `json:"containerCount"`
	EstDailyGB float64 `json:"estDailyGB"`
}

func (s *Server) handleLogVolumeEstimator(w http.ResponseWriter, r *http.Request) {
	result := LogVolumeResult1948{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]*LogVolumeNS1948)

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			// Estimate log volume based on restarts and age (heuristic)
			restartCount := 0
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == c.Name {
					restartCount = int(cs.RestartCount)
				}
			}

			// Rough estimate: 0.5MB/hr base + 5MB per restart
			estMBHr := 0.5 + float64(restartCount)*5.0
			hasLimit := false
			// Check for log limits via annotations (kubelet doesn't expose directly)
			for k := range pod.Annotations {
				if containsStr1948(k, "log") && containsStr1948(k, "limit") {
					hasLimit = true
					break
				}
			}

			if hasLimit {
				result.Summary.WithLogLimits++
			} else {
				result.Summary.WithoutLimits++
			}

			estDailyGB := estMBHr * 24 / 1024
			result.Summary.EstDailyGB += estDailyGB

			if estMBHr > 10 {
				result.Summary.HighLogVolPods++
				result.HighVolumePods = append(result.HighVolumePods, LogVolumeEntry1948{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					HasLimit: hasLimit, EstMBPerHr: estMBHr,
				})
				score -= 1
			}

			if nsStats[pod.Namespace] == nil {
				nsStats[pod.Namespace] = &LogVolumeNS1948{Namespace: pod.Namespace}
			}
			nsStats[pod.Namespace].Containers++
			nsStats[pod.Namespace].EstDailyGB += estDailyGB
		}
	}

	for _, ns := range nsStats {
		result.ByNS = append(result.ByNS, *ns)
	}

	if result.Summary.WithoutLimits > 0 {
		score -= 2
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutLimits > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without log limits — add log rotation config", result.Summary.WithoutLimits))
	}
	if result.Summary.HighLogVolPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d high-volume log pods — consider log forwarding to central store", result.Summary.HighLogVolPods))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Estimated %.1f GB/day log volume across %d containers", result.Summary.EstDailyGB, result.Summary.TotalContainers))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsStr1948(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 2. Pod Eviction History
// ---------------------------------------------------------------

type EvictionResult1948 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EvictionSummary1948  `json:"summary"`
	Evictions       []EvictionEntry1948  `json:"evictions"`
	ByReason        []EvictionReason1948 `json:"byReason"`
	Recommendations []string             `json:"recommendations"`
}

type EvictionSummary1948 struct {
	TotalEvictions int `json:"totalEvictions"`
	Voluntary      int `json:"voluntaryEvictions"`
	Forced         int `json:"forcedEvictions"`
	ByPressure     int `json:"pressureEvictions"`
	ByPreemption   int `json:"preemptionEvictions"`
	Recent24h      int `json:"evictionsLast24h"`
}

type EvictionEntry1948 struct {
	PodName   string  `json:"podName"`
	Namespace string  `json:"namespace"`
	Reason    string  `json:"reason"`
	AgeHours  float64 `json:"ageHours"`
	Node      string  `json:"nodeName"`
}

type EvictionReason1948 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (s *Server) handleEvictionHistory(w http.ResponseWriter, r *http.Request) {
	result := EvictionResult1948{ScannedAt: time.Now()}
	score := 100
	reasonMap := make(map[string]int)
	now := time.Now()

	evList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{
		FieldSelector: "reason=Evicted",
	})

	for _, ev := range evList.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvictions++

		ageHrs := now.Sub(ev.LastTimestamp.Time).Hours()
		if ageHrs < 24 {
			result.Summary.Recent24h++
		}

		reason := ""
		msg := ev.Message
		if containsStr1948(msg, "node") && containsStr1948(msg, "pressure") {
			reason = "node-pressure"
			result.Summary.ByPressure++
		} else if containsStr1948(msg, "preempt") || containsStr1948(msg, "PriorityClass") {
			reason = "preemption"
			result.Summary.ByPreemption++
		} else {
			reason = "voluntary"
			result.Summary.Voluntary++
		}
		reasonMap[reason]++

		result.Evictions = append(result.Evictions, EvictionEntry1948{
			PodName: ev.InvolvedObject.Name, Namespace: ev.Namespace,
			Reason: reason, AgeHours: ageHrs,
			Node: ev.Source.Host,
		})
	}

	result.Summary.Forced = result.Summary.ByPressure + result.Summary.ByPreemption

	for reason, count := range reasonMap {
		result.ByReason = append(result.ByReason, EvictionReason1948{Reason: reason, Count: count})
	}

	if result.Summary.Recent24h > 10 {
		score -= 10
	} else if result.Summary.Recent24h > 5 {
		score -= 5
	}
	if result.Summary.ByPressure > 0 {
		score -= 3
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.Recent24h > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d evictions in last 24h — investigate node pressure", result.Summary.Recent24h))
	}
	if result.Summary.ByPressure > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pressure-based evictions — add resources or rebalance", result.Summary.ByPressure))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total evictions (%d forced, %d voluntary)", result.Summary.TotalEvictions, result.Summary.Forced, result.Summary.Voluntary))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Kubelet Sync Latency
// ---------------------------------------------------------------

type KubeletSyncResult1948 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         KubeletSyncSummary1948 `json:"summary"`
	Nodes           []KubeletSyncEntry1948 `json:"nodes"`
	StaleNodes      []KubeletSyncStale1948 `json:"staleNodes"`
	Recommendations []string               `json:"recommendations"`
}

type KubeletSyncSummary1948 struct {
	TotalNodes      int     `json:"totalNodes"`
	AvgHeartbeatAge float64 `json:"avgHeartbeatAgeMin"`
	StaleNodes      int     `json:"staleNodes"`
	MaxHeartbeatAge float64 `json:"maxHeartbeatAgeMin"`
	FreshNodes      int     `json:"freshNodes"`
}

type KubeletSyncEntry1948 struct {
	Node         string  `json:"node"`
	HeartbeatAge float64 `json:"heartbeatAgeMin"`
	IsStale      bool    `json:"isStale"`
}

type KubeletSyncStale1948 struct {
	Node         string  `json:"node"`
	HeartbeatAge float64 `json:"heartbeatAgeMin"`
	Severity     string  `json:"severity"`
}

func (s *Server) handleKubeletSync(w http.ResponseWriter, r *http.Request) {
	result := KubeletSyncResult1948{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var totalAge float64

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		// Find most recent heartbeat condition
		var heartbeatTime time.Time
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				heartbeatTime = cond.LastHeartbeatTime.Time
				break
			}
		}
		if heartbeatTime.IsZero() {
			heartbeatTime = node.Status.Conditions[0].LastHeartbeatTime.Time
		}

		ageMin := 0.0
		if !heartbeatTime.IsZero() {
			ageMin = now.Sub(heartbeatTime).Minutes()
		}
		totalAge += ageMin

		isStale := ageMin > 5
		if ageMin > result.Summary.MaxHeartbeatAge {
			result.Summary.MaxHeartbeatAge = ageMin
		}

		entry := KubeletSyncEntry1948{
			Node: node.Name, HeartbeatAge: ageMin, IsStale: isStale,
		}
		result.Nodes = append(result.Nodes, entry)

		if isStale {
			result.Summary.StaleNodes++
			severity := "medium"
			if ageMin > 15 {
				severity = "high"
			}
			if ageMin > 30 {
				severity = "critical"
			}
			result.StaleNodes = append(result.StaleNodes, KubeletSyncStale1948{
				Node: node.Name, HeartbeatAge: ageMin, Severity: severity,
			})
			if severity == "critical" {
				score -= 15
			} else if severity == "high" {
				score -= 8
			} else {
				score -= 3
			}
		} else {
			result.Summary.FreshNodes++
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgHeartbeatAge = totalAge / float64(result.Summary.TotalNodes)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.StaleNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale nodes (>5min heartbeat) — check kubelet health", result.Summary.StaleNodes))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, avg heartbeat %.1fmin ago, %d fresh", result.Summary.TotalNodes, result.Summary.AvgHeartbeatAge, result.Summary.FreshNodes))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
