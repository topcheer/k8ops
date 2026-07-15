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

// ReadinessGateResult is the pod readiness gate compliance & custom condition audit.
type ReadinessGateResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         GateSummary         `json:"summary"`
	ByNamespace     []GateNSStat        `json:"byNamespace"`
	WithGates       []GateWorkloadEntry `json:"withGates"`
	BlockedPods     []GateBlockedPod    `json:"blockedPods"`
	Risks           []GateRisk          `json:"risks"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// GateSummary aggregates readiness gate metrics.
type GateSummary struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	WithReadinessGates int `json:"withReadinessGates"` // workloads using readiness gates
	WithoutGates       int `json:"withoutGates"`
	BlockedByGates     int `json:"blockedByGates"` // pods blocked by unmet gate condition
	GateConditions     int `json:"gateConditions"` // unique gate condition types
	TotalPods          int `json:"totalPods"`
	ReadyPods          int `json:"readyPods"`
	NotReadyPods       int `json:"notReadyPods"`
	GateBlockedPods    int `json:"gateBlockedPods"` // pods not ready due to gate
}

// GateNSStat per-namespace gate stats.
type GateNSStat struct {
	Namespace      string `json:"namespace"`
	TotalWorkloads int    `json:"totalWorkloads"`
	WithGates      int    `json:"withGates"`
	BlockedPods    int    `json:"blockedPods"`
}

// GateWorkloadEntry describes a workload using readiness gates.
type GateWorkloadEntry struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Kind            string   `json:"kind"`
	GateConditions  []string `json:"gateConditions"`
	PodCount        int      `json:"podCount"`
	ReadyPods       int      `json:"readyPods"`
	GateBlockedPods int      `json:"gateBlockedPods"`
}

// GateBlockedPod describes a pod blocked by an unmet gate condition.
type GateBlockedPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Condition string `json:"condition"`
	Status    string `json:"status"` // False, Unknown
	Reason    string `json:"reason,omitempty"`
}

// GateRisk describes a readiness gate risk.
type GateRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleReadinessGate audits pod readiness gate compliance & custom conditions.
// GET /api/deployment/readiness-gate
func (s *Server) handleReadinessGate(w http.ResponseWriter, r *http.Request) {
	result := ReadinessGateResult{
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

	// Track workloads with readiness gates
	wlMap := map[string]*GateWorkloadEntry{}
	nsStats := map[string]*GateNSStat{}
	gateConditionTypes := map[string]bool{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		result.Summary.TotalPods++

		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &GateNSStat{Namespace: ns}
		}

		// Determine workload name
		wlName := pod.Name
		wlKind := "Pod"
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" {
				wlKind = "Deployment"
				wlName = ref.Name
				if idx := strings.LastIndex(wlName, "-"); idx > 0 {
					lastSeg := wlName[idx+1:]
					if len(lastSeg) >= 5 && isAllHex(lastSeg) {
						wlName = wlName[:idx]
					}
				}
			} else if ref.Kind != "" {
				wlKind = ref.Kind
				wlName = ref.Name
			}
		}

		wlKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, wlKind, wlName)
		if wlMap[wlKey] == nil {
			wlMap[wlKey] = &GateWorkloadEntry{
				Name: wlName, Namespace: pod.Namespace, Kind: wlKind,
			}
			result.Summary.TotalWorkloads++
			nsStats[ns].TotalWorkloads++
		}
		entry := wlMap[wlKey]
		entry.PodCount++

		// Check pod readiness
		isReady := pod.Status.Phase == corev1.PodRunning
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
			}
		}
		if isReady {
			result.Summary.ReadyPods++
		} else {
			result.Summary.NotReadyPods++
		}

		// Check readiness gates
		gates := pod.Spec.ReadinessGates
		if len(gates) > 0 {
			entry.GateConditions = make([]string, 0, len(gates))
			for _, g := range gates {
				entry.GateConditions = append(entry.GateConditions, string(g.ConditionType))
				gateConditionTypes[string(g.ConditionType)] = true
			}
			result.Summary.WithReadinessGates++
			nsStats[ns].WithGates++

			// Check if any gate condition is not True
			for _, g := range gates {
				condType := string(g.ConditionType)
				found := false
				for _, cond := range pod.Status.Conditions {
					if string(cond.Type) == condType {
						found = true
						if cond.Status != corev1.ConditionTrue {
							result.Summary.GateBlockedPods++
							entry.GateBlockedPods++
							nsStats[ns].BlockedPods++
							result.BlockedPods = append(result.BlockedPods, GateBlockedPod{
								Name: pod.Name, Namespace: pod.Namespace,
								Condition: condType,
								Status:    string(cond.Status),
								Reason:    cond.Reason,
							})
							if !isReady {
								result.Summary.BlockedByGates++
							}
							result.Risks = append(result.Risks, GateRisk{
								Namespace: pod.Namespace,
								Issue: fmt.Sprintf("Pod %s/%s blocked by readiness gate '%s' (status: %s)",
									pod.Namespace, pod.Name, condType, cond.Status),
								Severity: "high",
							})
						}
						break
					}
				}
				if !found {
					// Gate condition not present in pod status — Unknown
					result.Summary.GateBlockedPods++
					entry.GateBlockedPods++
					result.BlockedPods = append(result.BlockedPods, GateBlockedPod{
						Name: pod.Name, Namespace: pod.Namespace,
						Condition: condType, Status: "Unknown",
						Reason: "Gate condition not reported by any condition controller",
					})
					result.Risks = append(result.Risks, GateRisk{
						Namespace: pod.Namespace,
						Issue: fmt.Sprintf("Pod %s/%s has readiness gate '%s' with Unknown status — no controller reporting this condition",
							pod.Namespace, pod.Name, condType),
						Severity: "medium",
					})
				}
			}
		} else {
			result.Summary.WithoutGates++
		}
	}

	// Build workload entries
	for _, entry := range wlMap {
		if len(entry.GateConditions) > 0 {
			result.WithGates = append(result.WithGates, *entry)
		}
	}
	sort.Slice(result.WithGates, func(i, j int) bool {
		return result.WithGates[i].GateBlockedPods > result.WithGates[j].GateBlockedPods
	})

	result.Summary.GateConditions = len(gateConditionTypes)

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].BlockedPods > result.ByNamespace[j].BlockedPods
	})

	// Health score
	score := 100
	if result.Summary.BlockedByGates > 0 {
		score -= min(30, result.Summary.BlockedByGates*10)
	}
	if result.Summary.GateBlockedPods > 0 {
		score -= min(15, result.Summary.GateBlockedPods*3)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.BlockedByGates > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) are blocked by unmet readiness gates — check gate condition controllers", result.Summary.BlockedByGates))
	}
	if len(gateConditionTypes) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unique gate condition type(s) detected — ensure controllers are running for all gate types", len(gateConditionTypes)))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"No readiness gate issues detected — all gates are satisfied or no gates are configured")
	}

	writeJSON(w, result)
}
