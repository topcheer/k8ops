package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestartStormResult is the pod restart pattern & crashloop clustering audit.
type RestartStormResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         RestartStormSummary    `json:"summary"`
	ByNamespace     []RestartNSStat        `json:"byNamespace"`
	HotspotPods     []RestartHotspotPod    `json:"hotspotPods"`
	CascadePatterns []CascadePattern       `json:"cascadePatterns"`
	Timeline        []RestartTimelineEntry `json:"timeline"`
	Risks           []RestartStormRisk     `json:"risks"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// RestartStormSummary aggregates restart storm metrics.
type RestartStormSummary struct {
	TotalPods          int  `json:"totalPods"`
	TotalRestarts      int  `json:"totalRestarts"`
	PodsWithRestarts   int  `json:"podsWithRestarts"`   // pods with >0 restarts
	HighRestartPods    int  `json:"highRestartPods"`    // >5 restarts
	CriticalRestarts   int  `json:"criticalRestarts"`   // >20 restarts
	ClusteringDetected bool `json:"clusteringDetected"` // multiple pods restarting in same namespace/time
	UniqueImages       int  `json:"uniqueImages"`       // unique images among restarting pods
	AffectedNS         int  `json:"affectedNS"`         // namespaces with restarting pods
}

// RestartNSStat per-namespace restart stats.
type RestartNSStat struct {
	Namespace      string `json:"namespace"`
	TotalPods      int    `json:"totalPods"`
	RestartingPods int    `json:"restartingPods"`
	TotalRestarts  int    `json:"totalRestarts"`
	MaxRestarts    int    `json:"maxRestarts"`
	IsCluster      bool   `json:"isCluster"` // multiple pods restarting = cluster
}

// RestartHotspotPod describes a pod with high restart count.
type RestartHotspotPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image,omitempty"`
	Restarts  int    `json:"restarts"`
	Reason    string `json:"reason,omitempty"`
	Age       string `json:"age,omitempty"`
}

// CascadePattern describes a cascade failure pattern.
type CascadePattern struct {
	Namespace string   `json:"namespace"`
	Pods      []string `json:"pods"`
	Pattern   string   `json:"pattern"` // same-image, same-namespace, time-correlated
	Severity  string   `json:"severity"`
}

// RestartTimelineEntry time-bucketed restart activity.
type RestartTimelineEntry struct {
	Hour     string `json:"hour"`
	Restarts int    `json:"restarts"`
	Pods     int    `json:"pods"`
}

// RestartStormRisk describes a restart-related risk.
type RestartStormRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleRestartStorm audits pod restart patterns & crashloop clustering.
// GET /api/operations/restart-storm
func (s *Server) handleRestartStorm(w http.ResponseWriter, r *http.Request) {
	result := RestartStormResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	nsStats := map[string]*RestartNSStat{}
	imageMap := map[string]int{} // image → count of restarting pods
	var hotspots []RestartHotspotPod

	for i := range pods.Items {
		pod := &pods.Items[i]
		result.Summary.TotalPods++

		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &RestartNSStat{Namespace: ns}
		}
		nsStats[ns].TotalPods++

		totalRestarts := 0
		var lastReason string
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
			if cs.LastTerminationState.Terminated != nil {
				lastReason = cs.LastTerminationState.Terminated.Reason
			}
		}

		if totalRestarts > 0 {
			result.Summary.PodsWithRestarts++
			result.Summary.TotalRestarts += totalRestarts
			nsStats[ns].RestartingPods++
			nsStats[ns].TotalRestarts += totalRestarts
			if totalRestarts > nsStats[ns].MaxRestarts {
				nsStats[ns].MaxRestarts = totalRestarts
			}

			// Track image
			if len(pod.Spec.Containers) > 0 {
				img := pod.Spec.Containers[0].Image
				imageMap[img]++
			}

			if totalRestarts > 5 {
				result.Summary.HighRestartPods++
			}
			if totalRestarts > 20 {
				result.Summary.CriticalRestarts++
			}

			if totalRestarts > 3 {
				hp := RestartHotspotPod{
					Name:      pod.Name,
					Namespace: ns,
					Restarts:  totalRestarts,
					Reason:    lastReason,
				}
				if len(pod.Spec.Containers) > 0 {
					hp.Image = pod.Spec.Containers[0].Image
				}
				if pod.CreationTimestamp.Time.IsZero() {
					hp.Age = "unknown"
				} else {
					hp.Age = time.Since(pod.CreationTimestamp.Time).Round(time.Hour).String()
				}
				hotspots = append(hotspots, hp)
			}
		}
	}

	// Sort hotspots by restart count
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].Restarts > hotspots[j].Restarts
	})
	result.HotspotPods = hotspots

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.RestartingPods > 1 {
			stat.IsCluster = true
			result.Summary.ClusteringDetected = true
			result.Risks = append(result.Risks, RestartStormRisk{
				Namespace: stat.Namespace,
				Issue:     fmt.Sprintf("Namespace %s has %d pods restarting (total %d restarts) — possible cascade failure", stat.Namespace, stat.RestartingPods, stat.TotalRestarts),
				Severity:  "high",
			})
		}
		if stat.RestartingPods > 0 {
			result.Summary.AffectedNS++
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalRestarts > result.ByNamespace[j].TotalRestarts
	})

	// Detect same-image cascade
	for img, count := range imageMap {
		if count > 2 {
			result.CascadePatterns = append(result.CascadePatterns, CascadePattern{
				Pattern:  "same-image",
				Pods:     []string{fmt.Sprintf("%d pods using %s", count, img)},
				Severity: "critical",
			})
			result.Risks = append(result.Risks, RestartStormRisk{
				Issue:    fmt.Sprintf("%d pods with same image %s are restarting — check for bad image release", count, img),
				Severity: "critical",
			})
		}
	}

	// Detect namespace-level cascade
	for _, stat := range nsStats {
		if stat.IsCluster && stat.TotalRestarts > 10 {
			result.CascadePatterns = append(result.CascadePatterns, CascadePattern{
				Namespace: stat.Namespace,
				Pattern:   "same-namespace",
				Pods:      []string{fmt.Sprintf("%d pods in %s", stat.RestartingPods, stat.Namespace)},
				Severity:  "high",
			})
		}
	}

	result.Summary.UniqueImages = len(imageMap)

	// Health score
	score := 100
	if result.Summary.CriticalRestarts > 0 {
		score -= 30
	}
	if result.Summary.HighRestartPods > 0 {
		score -= min(20, result.Summary.HighRestartPods*3)
	}
	if result.Summary.ClusteringDetected {
		score -= 15
	}
	if result.Summary.AffectedNS > 3 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.CriticalRestarts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) with critical restart count (>20) — investigate immediately", result.Summary.CriticalRestarts))
	}
	if result.Summary.ClusteringDetected {
		result.Recommendations = append(result.Recommendations,
			"Restart clustering detected — multiple pods in same namespace are restarting, possible cascade failure")
	}
	for _, cp := range result.CascadePatterns {
		if cp.Pattern == "same-image" {
			result.Recommendations = append(result.Recommendations,
				"Same image causing multiple pod restarts — consider rolling back to previous image version")
		}
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"No restart storm patterns detected — cluster is stable")
	}

	writeJSON(w, result)
}
