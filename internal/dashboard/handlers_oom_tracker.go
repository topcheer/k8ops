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

// OOMTrackerResult is the full container OOM kill analysis.
type OOMTrackerResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         OOMSummary         `json:"summary"`
	AffectedPods    []OOMPodEntry      `json:"affectedPods"`
	TopKillers      []OOMKillEntry     `json:"topKillers"`
	ByNamespace     []OOMNamespaceStat `json:"byNamespace"`
	Recommendations []string           `json:"recommendations"`
}

// OOMSummary aggregates cluster-wide OOM statistics.
type OOMSummary struct {
	TotalRestarts      int `json:"totalRestarts"`
	OOMKilledCount     int `json:"oomKilledCount"`     // containers with OOMKilled termination
	PodsWithOOM        int `json:"podsWithOOM"`        // distinct pods affected
	HighRestartCount   int `json:"highRestartCount"`   // restarts >= 5
	NoMemLimit         int `json:"noMemLimit"`         // containers without memory limit
	LowMemLimit        int `json:"lowMemLimit"`        // limit < 256Mi
	MemLimitMuchHigher int `json:"memLimitMuchHigher"` // limit >> request (10x)
	OOMRiskScore       int `json:"oomRiskScore"`       // 0-100
}

// OOMPodEntry describes OOM-related issues for one pod.
type OOMPodEntry struct {
	Name         string             `json:"name"`
	Namespace    string             `json:"namespace"`
	Node         string             `json:"node"`
	RestartCount int32              `json:"restartCount"`
	Containers   []OOMContainerInfo `json:"containers"`
	RiskLevel    string             `json:"riskLevel"`
}

// OOMContainerInfo describes one container's OOM risk.
type OOMContainerInfo struct {
	Name            string   `json:"name"`
	RestartCount    int32    `json:"restartCount"`
	LastTermination string   `json:"lastTerminationReason,omitempty"`
	MemRequestMB    int64    `json:"memRequestMB"`
	MemLimitMB      int64    `json:"memLimitMB"`
	HasMemLimit     bool     `json:"hasMemLimit"`
	OOMKilled       bool     `json:"oomKilled"`
	Issues          []string `json:"issues"`
}

// OOMKillEntry is a summary of the worst OOM offenders.
type OOMKillEntry struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Restarts  int32  `json:"restarts"`
	Reason    string `json:"reason"`
}

// OOMNamespaceStat per-namespace OOM stats.
type OOMNamespaceStat struct {
	Namespace string `json:"namespace"`
	OOMCount  int    `json:"oomCount"`
	Restarts  int    `json:"restarts"`
}

// handleOOMTracker analyzes OOM kills and memory configuration across pods.
// GET /api/operations/oom-tracker
func (s *Server) handleOOMTracker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := OOMTrackerResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*OOMNamespaceStat)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		entry := OOMPodEntry{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Node:      pod.Spec.NodeName,
		}

		var podHasOOM bool
		var podRestartTotal int32

		for _, cs := range pod.Status.ContainerStatuses {
			ci := OOMContainerInfo{
				Name:         cs.Name,
				RestartCount: cs.RestartCount,
			}
			podRestartTotal += cs.RestartCount

			// Check last termination reason
			if cs.LastTerminationState.Terminated != nil {
				reason := cs.LastTerminationState.Terminated.Reason
				ci.LastTermination = reason
				if reason == "OOMKilled" {
					ci.OOMKilled = true
					podHasOOM = true
					result.Summary.OOMKilledCount++
					result.TopKillers = append(result.TopKillers, OOMKillEntry{
						Pod: pod.Name, Namespace: pod.Namespace,
						Container: cs.Name, Restarts: cs.RestartCount,
						Reason: "OOMKilled",
					})
				}
			}

			// Find matching container spec for memory config
			for _, c := range pod.Spec.Containers {
				if c.Name != cs.Name {
					continue
				}
				if r := c.Resources.Requests.Memory(); r != nil && !r.IsZero() {
					ci.MemRequestMB = r.Value() / (1024 * 1024)
				}
				if r := c.Resources.Limits.Memory(); r != nil && !r.IsZero() {
					ci.MemLimitMB = r.Value() / (1024 * 1024)
					ci.HasMemLimit = true
				} else {
					ci.HasMemLimit = false
					result.Summary.NoMemLimit++
					ci.Issues = append(ci.Issues, "no memory limit — can be OOM killed unpredictably")
				}

				if ci.HasMemLimit && ci.MemLimitMB < 256 {
					result.Summary.LowMemLimit++
					ci.Issues = append(ci.Issues, fmt.Sprintf("low memory limit (%dMB < 256MB) — may be too restrictive", ci.MemLimitMB))
				}

				if ci.HasMemLimit && ci.MemRequestMB > 0 && ci.MemLimitMB > ci.MemRequestMB*10 {
					result.Summary.MemLimitMuchHigher++
					ci.Issues = append(ci.Issues, fmt.Sprintf("memory limit (%dMB) is 10x+ higher than request (%dMB) — risk of node pressure", ci.MemLimitMB, ci.MemRequestMB))
				}

				if ci.OOMKilled && !ci.HasMemLimit {
					ci.Issues = append(ci.Issues, "OOM killed but has no memory limit — add a limit to control behavior")
				}

				if ci.OOMKilled && ci.HasMemLimit {
					ci.Issues = append(ci.Issues, fmt.Sprintf("OOM killed despite %dMB limit — increase limit or investigate memory leak", ci.MemLimitMB))
				}
				break
			}

			entry.Containers = append(entry.Containers, ci)
		}

		entry.RestartCount = podRestartTotal
		result.Summary.TotalRestarts += int(podRestartTotal)

		if podRestartCount := int(podRestartTotal); podRestartCount >= 5 {
			result.Summary.HighRestartCount++
		}

		if podHasOOM || podRestartTotal > 0 {
			entry.RiskLevel = assessOOMRisk(entry)
			if podHasOOM {
				result.Summary.PodsWithOOM++
			}

			// Namespace stats
			nsStat := getOrCreateOOMNs(nsMap, pod.Namespace)
			if podHasOOM {
				nsStat.OOMCount++
			}
			nsStat.Restarts += int(podRestartTotal)

			result.AffectedPods = append(result.AffectedPods, entry)
		}
	}

	// Sort affected pods by risk
	sort.Slice(result.AffectedPods, func(i, j int) bool {
		return oomRiskRank(result.AffectedPods[i].RiskLevel) < oomRiskRank(result.AffectedPods[j].RiskLevel)
	})

	// Sort top killers by restart count
	sort.Slice(result.TopKillers, func(i, j int) bool {
		return result.TopKillers[i].Restarts > result.TopKillers[j].Restarts
	})
	if len(result.TopKillers) > 20 {
		result.TopKillers = result.TopKillers[:20]
	}

	// Namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].OOMCount > result.ByNamespace[j].OOMCount
	})

	result.Summary.OOMRiskScore = calculateOOMScore(result.Summary)
	result.Recommendations = generateOOMRecs(result.Summary, result.TopKillers)

	writeJSON(w, result)
}

// assessOOMRisk determines risk level for a pod.
func assessOOMRisk(entry OOMPodEntry) string {
	risk := 0
	oomCount := 0
	for _, c := range entry.Containers {
		if c.OOMKilled {
			risk += 15
			oomCount++
		}
		if !c.HasMemLimit {
			risk += 5
		}
	}

	totalRestarts := int(entry.RestartCount)
	if totalRestarts >= 10 {
		risk += 20
	} else if totalRestarts >= 5 {
		risk += 10
	}

	switch {
	case risk >= 30:
		return "critical"
	case risk >= 15:
		return "high"
	case risk >= 5:
		return "medium"
	default:
		return "low"
	}
}

// calculateOOMScore computes 0-100.
func calculateOOMScore(s OOMSummary) int {
	score := 100
	score -= s.OOMKilledCount * 8
	score -= s.HighRestartCount * 5
	score -= s.NoMemLimit * 3
	score -= s.LowMemLimit * 2
	if score < 0 {
		score = 0
	}
	return score
}

// generateOOMRecs produces actionable advice.
func generateOOMRecs(s OOMSummary, killers []OOMKillEntry) []string {
	var recs []string

	if s.OOMKilledCount > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have been OOMKilled — increase memory limits or investigate memory leaks", s.OOMKilledCount))
	}
	if s.HighRestartCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have >=5 restarts — investigate crash loops and resource settings", s.HighRestartCount))
	}
	if s.NoMemLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have no memory limit — add limits for predictable OOM behavior", s.NoMemLimit))
	}
	if s.LowMemLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have low memory limit (<256MB) — may cause unnecessary OOM kills", s.LowMemLimit))
	}
	if s.MemLimitMuchHigher > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have memory limit 10x+ higher than request — risk of node memory pressure", s.MemLimitMuchHigher))
	}

	// Top OOM offender recommendation
	if len(killers) > 0 {
		top := killers[0]
		recs = append(recs, fmt.Sprintf("Top OOM offender: %s/%s (%s) with %d restarts — prioritize investigation", top.Namespace, top.Pod, top.Container, top.Restarts))
	}

	if s.OOMRiskScore < 60 {
		recs = append(recs, fmt.Sprintf("OOM risk score is %d/100 — review memory configuration across workloads", s.OOMRiskScore))
	}

	return recs
}

func getOrCreateOOMNs(m map[string]*OOMNamespaceStat, ns string) *OOMNamespaceStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &OOMNamespaceStat{Namespace: ns}
	m[ns] = e
	return e
}

func oomRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

// Ensure strings import is used
var _ = strings.Contains
