package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReplicaAvailResult is the deployment replica availability & ready pod ratio analysis.
type ReplicaAvailResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         ReplicaAvailSummary  `json:"summary"`
	ByWorkload      []ReplicaAvailEntry  `json:"byWorkload"`
	CriticalGaps    []ReplicaAvailEntry  `json:"criticalGaps"`
	ByNamespace     []ReplicaAvailNSStat `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

// ReplicaAvailSummary aggregates replica availability statistics.
type ReplicaAvailSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	HealthyWorkloads int `json:"healthyWorkloads"` // readyReplicas == replicas
	GapWorkloads     int `json:"gapWorkloads"`     // readyReplicas < replicas
	ZeroReady        int `json:"zeroReady"`        // readyReplicas == 0 but replicas > 0
	TotalDesired     int `json:"totalDesired"`
	TotalReady       int `json:"totalReady"`
	TotalAvailable   int `json:"totalAvailable"`
	HealthScore      int `json:"healthScore"`
}

// ReplicaAvailEntry describes one workload's replica availability.
type ReplicaAvailEntry struct {
	Name              string  `json:"name"`
	Namespace         string  `json:"namespace"`
	WorkloadType      string  `json:"workloadType"`
	DesiredReplicas   int32   `json:"desiredReplicas"`
	ReadyReplicas     int32   `json:"readyReplicas"`
	AvailableReplicas int32   `json:"availableReplicas"`
	ReadyRatio        float64 `json:"readyRatio"`
	UpdatedReplicas   int32   `json:"updatedReplicas"`
	StaleReplicas     int32   `json:"staleReplicas"` // replicas - updatedReplicas
	Age               string  `json:"age"`
	RiskLevel         string  `json:"riskLevel"`
	Issue             string  `json:"issue,omitempty"`
}

// ReplicaAvailNSStat shows replica availability per namespace.
type ReplicaAvailNSStat struct {
	Namespace      string `json:"namespace"`
	TotalWorkloads int    `json:"totalWorkloads"`
	GapWorkloads   int    `json:"gapWorkloads"`
	TotalDesired   int    `json:"totalDesired"`
	TotalReady     int    `json:"totalReady"`
}

// replicaAvailAuditCore performs the audit logic on workload lists (testable).
func replicaAvailAuditCore(deployments []appsv1.Deployment, statefulSets []appsv1.StatefulSet, daemonSets []appsv1.DaemonSet) ReplicaAvailResult {
	result := ReplicaAvailResult{
		ScannedAt: time.Now(),
	}

	nsStats := make(map[string]*ReplicaAvailNSStat)

	// Process Deployments
	for i := range deployments {
		d := &deployments[i]
		ns := d.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &ReplicaAvailNSStat{Namespace: ns}
		}
		nsStats[ns].TotalWorkloads++
		result.Summary.TotalWorkloads++

		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ReadyReplicas
		avail := d.Status.AvailableReplicas
		updated := d.Status.UpdatedReplicas

		entry := ReplicaAvailEntry{
			Name:              d.Name,
			Namespace:         ns,
			WorkloadType:      "Deployment",
			DesiredReplicas:   desired,
			ReadyReplicas:     ready,
			AvailableReplicas: avail,
			UpdatedReplicas:   updated,
			StaleReplicas:     desired - updated,
			Age:               formatDurationAge(d.CreationTimestamp.Time),
		}

		if desired > 0 {
			entry.ReadyRatio = float64(ready) / float64(desired)
		}

		entry.RiskLevel, entry.Issue = assessReplicaRisk(desired, ready, updated)

		result.Summary.TotalDesired += int(desired)
		result.Summary.TotalReady += int(ready)
		result.Summary.TotalAvailable += int(avail)

		if entry.Issue != "" {
			result.Summary.GapWorkloads++
			nsStats[ns].GapWorkloads++
			if ready == 0 && desired > 0 {
				result.Summary.ZeroReady++
				result.CriticalGaps = append(result.CriticalGaps, entry)
			}
		} else {
			result.Summary.HealthyWorkloads++
		}

		nsStats[ns].TotalDesired += int(desired)
		nsStats[ns].TotalReady += int(ready)

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Process StatefulSets
	for i := range statefulSets {
		ss := &statefulSets[i]
		ns := ss.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &ReplicaAvailNSStat{Namespace: ns}
		}
		nsStats[ns].TotalWorkloads++
		result.Summary.TotalWorkloads++

		desired := int32(0)
		if ss.Spec.Replicas != nil {
			desired = *ss.Spec.Replicas
		}
		ready := ss.Status.ReadyReplicas
		updated := ss.Status.UpdatedReplicas

		entry := ReplicaAvailEntry{
			Name:              ss.Name,
			Namespace:         ns,
			WorkloadType:      "StatefulSet",
			DesiredReplicas:   desired,
			ReadyReplicas:     ready,
			AvailableReplicas: ready, // StatefulSet doesn't have AvailableReplicas
			UpdatedReplicas:   updated,
			StaleReplicas:     desired - updated,
			Age:               formatDurationAge(ss.CreationTimestamp.Time),
		}

		if desired > 0 {
			entry.ReadyRatio = float64(ready) / float64(desired)
		}

		entry.RiskLevel, entry.Issue = assessReplicaRisk(desired, ready, updated)

		result.Summary.TotalDesired += int(desired)
		result.Summary.TotalReady += int(ready)
		result.Summary.TotalAvailable += int(ready)

		if entry.Issue != "" {
			result.Summary.GapWorkloads++
			nsStats[ns].GapWorkloads++
			if ready == 0 && desired > 0 {
				result.Summary.ZeroReady++
				result.CriticalGaps = append(result.CriticalGaps, entry)
			}
		} else {
			result.Summary.HealthyWorkloads++
		}

		nsStats[ns].TotalDesired += int(desired)
		nsStats[ns].TotalReady += int(ready)

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Process DaemonSets (desired = desiredNumberScheduled, ready = numberReady)
	for i := range daemonSets {
		ds := &daemonSets[i]
		ns := ds.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &ReplicaAvailNSStat{Namespace: ns}
		}
		nsStats[ns].TotalWorkloads++
		result.Summary.TotalWorkloads++

		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		updated := ds.Status.UpdatedNumberScheduled

		entry := ReplicaAvailEntry{
			Name:              ds.Name,
			Namespace:         ns,
			WorkloadType:      "DaemonSet",
			DesiredReplicas:   desired,
			ReadyReplicas:     ready,
			AvailableReplicas: ready,
			UpdatedReplicas:   updated,
			StaleReplicas:     desired - updated,
			Age:               formatDurationAge(ds.CreationTimestamp.Time),
		}

		if desired > 0 {
			entry.ReadyRatio = float64(ready) / float64(desired)
		}

		entry.RiskLevel, entry.Issue = assessReplicaRisk(desired, ready, updated)

		result.Summary.TotalDesired += int(desired)
		result.Summary.TotalReady += int(ready)
		result.Summary.TotalAvailable += int(ready)

		if entry.Issue != "" {
			result.Summary.GapWorkloads++
			nsStats[ns].GapWorkloads++
			if ready == 0 && desired > 0 {
				result.Summary.ZeroReady++
				result.CriticalGaps = append(result.CriticalGaps, entry)
			}
		} else {
			result.Summary.HealthyWorkloads++
		}

		nsStats[ns].TotalDesired += int(desired)
		nsStats[ns].TotalReady += int(ready)

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].GapWorkloads > result.ByNamespace[j].GapWorkloads
	})

	// Sort by ready ratio (worst first)
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].ReadyRatio < result.ByWorkload[j].ReadyRatio
	})

	result.Summary.HealthScore = replicaAvailScore(result.Summary)
	result.Recommendations = replicaAvailRecommendations(result.Summary)

	return result
}

// assessReplicaRisk evaluates replica availability and returns risk level + issue description.
func assessReplicaRisk(desired, ready, updated int32) (risk, issue string) {
	if desired == 0 {
		return "none", "" // scaled to 0 intentionally
	}
	if ready == 0 {
		return "critical", fmt.Sprintf("0/%d pods ready — workload is completely unavailable", desired)
	}
	if ready < desired {
		return "high", fmt.Sprintf("%d/%d pods ready (%.0f%%) — partial availability gap", ready, desired, float64(ready)/float64(desired)*100)
	}
	if updated < desired {
		return "medium", fmt.Sprintf("all pods ready but %d/%d still using old spec (rollout in progress)", updated, desired)
	}
	return "none", ""
}

// replicaAvailScore calculates health score from summary.
func replicaAvailScore(s ReplicaAvailSummary) int {
	if s.TotalDesired == 0 {
		return 100
	}
	availabilityRatio := float64(s.TotalReady) / float64(s.TotalDesired)
	score := int(availabilityRatio * 100)

	// Penalize zero-ready workloads heavily
	score -= s.ZeroReady * 10
	// Penalize gap workloads moderately
	score -= s.GapWorkloads * 3

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// replicaAvailRecommendations generates recommendations.
func replicaAvailRecommendations(s ReplicaAvailSummary) []string {
	var recs []string
	if s.ZeroReady > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads have 0 ready pods — check pod status, events, and logs immediately", s.ZeroReady))
	}
	if s.GapWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads have replica availability gaps — investigate pending pods, resource constraints, or scheduling failures", s.GapWorkloads))
	}
	if s.HealthyWorkloads == s.TotalWorkloads && s.TotalWorkloads > 0 {
		recs = append(recs, "all workloads have full replica availability — no gaps detected")
	}
	if s.TotalDesired > 0 {
		pct := float64(s.TotalReady) / float64(s.TotalDesired) * 100
		recs = append(recs, fmt.Sprintf("cluster-wide pod availability: %d/%d (%.1f%%)", s.TotalReady, s.TotalDesired, pct))
	}
	return recs
}

// formatDurationAge returns a human-readable age string.
func formatDurationAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// handleReplicaAvailability audits deployment replica availability and ready pod ratio.
// GET /api/deployment/replica-availability
func (s *Server) handleReplicaAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulSets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonSets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})

	result := replicaAvailAuditCore(deployments.Items, statefulSets.Items, daemonSets.Items)
	writeJSON(w, result)
}
