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
// v20.14 — Operations Dimension (Round 22)
// 1. Pod Crash Loop Detector — CrashLoopBackOff pattern tracking
// 2. Deployment Replica Health — desired vs ready vs available
// 3. Event Warning Hotspot — namespace warning event concentration
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Crash Loop Detector
// ---------------------------------------------------------------

type CrashLoopResult2014 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CrashLoopSummary2014 `json:"summary"`
	Crashing        []CrashLoopEntry2014 `json:"crashingPods"`
	Recommendations []string             `json:"recommendations"`
}

type CrashLoopSummary2014 struct {
	TotalPods     int `json:"totalPods"`
	CrashLooping  int `json:"crashLoopingPods"`
	HighRestart   int `json:"highRestartPods"`
	TotalRestarts int `json:"totalRestarts"`
}

type CrashLoopEntry2014 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restartCount"`
}

func (s *Server) handleCrashLoopDetect(w http.ResponseWriter, r *http.Request) {
	result := CrashLoopResult2014{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := int32(0)
		isCrashing := false

		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				isCrashing = true
			}
		}

		result.Summary.TotalRestarts += int(totalRestarts)

		if isCrashing {
			result.Summary.CrashLooping++
			result.Crashing = append(result.Crashing, CrashLoopEntry2014{
				Pod: pod.Name, Namespace: pod.Namespace,
				Status: "CrashLoopBackOff", Restarts: totalRestarts,
			})
			score -= 5
		} else if totalRestarts > 5 {
			result.Summary.HighRestart++
			result.Crashing = append(result.Crashing, CrashLoopEntry2014{
				Pod: pod.Name, Namespace: pod.Namespace,
				Status: "HighRestart", Restarts: totalRestarts,
			})
			score -= 2
		}
	}

	sort.Slice(result.Crashing, func(i, j int) bool {
		return result.Crashing[i].Restarts > result.Crashing[j].Restarts
	})
	if len(result.Crashing) > 20 {
		result.Crashing = result.Crashing[:20]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d crash-looping, %d high-restart, %d total restarts", result.Summary.TotalPods, result.Summary.CrashLooping, result.Summary.HighRestart, result.Summary.TotalRestarts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Deployment Replica Health
// ---------------------------------------------------------------

type ReplicaHealthResult2014 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ReplicaHealthSummary2014 `json:"summary"`
	Issues          []ReplicaHealthEntry2014 `json:"issues"`
	Recommendations []string                 `json:"recommendations"`
}

type ReplicaHealthSummary2014 struct {
	TotalDeployments int `json:"totalDeployments"`
	FullyHealthy     int `json:"fullyHealthy"`
	UnderReplicated  int `json:"underReplicated"`
	ZeroReplicas     int `json:"zeroReplicas"`
}

type ReplicaHealthEntry2014 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Desired   int    `json:"desired"`
	Ready     int    `json:"ready"`
	Available int    `json:"available"`
}

func (s *Server) handleReplicaHealth(w http.ResponseWriter, r *http.Request) {
	result := ReplicaHealthResult2014{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		desired := 1
		if dep.Spec.Replicas != nil {
			desired = int(*dep.Spec.Replicas)
		}
		ready := int(dep.Status.ReadyReplicas)
		avail := int(dep.Status.AvailableReplicas)

		if desired == 0 {
			result.Summary.ZeroReplicas++
			continue
		}

		entry := ReplicaHealthEntry2014{
			Name: dep.Name, Namespace: dep.Namespace,
			Desired: desired, Ready: ready, Available: avail,
		}

		if ready >= desired && avail >= desired {
			result.Summary.FullyHealthy++
		} else {
			result.Summary.UnderReplicated++
			result.Issues = append(result.Issues, entry)
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d healthy, %d under-replicated, %d zero", result.Summary.TotalDeployments, result.Summary.FullyHealthy, result.Summary.UnderReplicated, result.Summary.ZeroReplicas))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event Warning Hotspot
// ---------------------------------------------------------------

type EvtHotspotResult2014 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EvtHotspotSummary2014 `json:"summary"`
	Hotspots        []EvtHotspotEntry2014 `json:"hotspots"`
	Recommendations []string              `json:"recommendations"`
}

type EvtHotspotSummary2014 struct {
	TotalEvents   int     `json:"totalEvents"`
	WarningEvents int     `json:"warningEvents"`
	WarningRatio  float64 `json:"warningRatio"`
	HotspotNS     int     `json:"hotspotNamespaces"`
}

type EvtHotspotEntry2014 struct {
	Namespace    string  `json:"namespace"`
	WarningCount int     `json:"warningCount"`
	TotalEvents  int     `json:"totalEvents"`
	Ratio        float64 `json:"warningRatio"`
}

func (s *Server) handleEvtHotspot(w http.ResponseWriter, r *http.Request) {
	result := EvtHotspotResult2014{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*EvtHotspotEntry2014)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		entry, ok := nsStats[evt.Namespace]
		if !ok {
			entry = &EvtHotspotEntry2014{Namespace: evt.Namespace}
			nsStats[evt.Namespace] = entry
		}
		entry.TotalEvents++

		if evt.Type == "Warning" {
			result.Summary.WarningEvents++
			entry.WarningCount++
		}
	}

	if result.Summary.TotalEvents > 0 {
		result.Summary.WarningRatio = float64(result.Summary.WarningEvents) / float64(result.Summary.TotalEvents)
	}

	for _, entry := range nsStats {
		if entry.TotalEvents > 0 {
			entry.Ratio = float64(entry.WarningCount) / float64(entry.TotalEvents)
		}
		if entry.WarningCount > 5 && entry.Ratio > 0.3 {
			result.Summary.HotspotNS++
			result.Hotspots = append(result.Hotspots, *entry)
			score -= 2
		}
	}

	sort.Slice(result.Hotspots, func(i, j int) bool {
		return result.Hotspots[i].WarningCount > result.Hotspots[j].WarningCount
	})
	if len(result.Hotspots) > 10 {
		result.Hotspots = result.Hotspots[:10]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events (%d warnings, %.0f%% ratio), %d hotspot NS", result.Summary.TotalEvents, result.Summary.WarningEvents, result.Summary.WarningRatio*100, result.Summary.HotspotNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
