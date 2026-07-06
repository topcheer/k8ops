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

// SLResult is the pod scheduling latency analysis.
type SLResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         SLSummary      `json:"summary"`
	ByPhase         map[string]int `json:"byPhase"`
	SlowPods        []SLEntry      `json:"slowPods"`    // >60s to schedule
	PendingPods     []SLEntry      `json:"pendingPods"` // still pending
	ByNode          []SLNodeEntry  `json:"byNode"`      // scheduling per node
	ByNamespace     []SLNSEntry    `json:"byNamespace"`
	Issues          []SLIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// SLSummary aggregates scheduling latency statistics.
type SLSummary struct {
	TotalPods       int     `json:"totalPods"`
	RunningPods     int     `json:"runningPods"`
	PendingPods     int     `json:"pendingPods"`
	FailedPods      int     `json:"failedPods"`
	AvgScheduleSec  float64 `json:"avgScheduleSec"`  // avg time from creation to scheduled
	MaxScheduleSec  float64 `json:"maxScheduleSec"`  // slowest scheduling time
	SlowCount       int     `json:"slowCount"`       // pods that took >60s to schedule
	VerySlowCount   int     `json:"verySlowCount"`   // >300s
	Unschedulable   int     `json:"unschedulable"`   // Pending with Unschedulable condition
	NoNodeResources int     `json:"noNodeResources"` // pending due to insufficient resources
	EfficiencyScore int     `json:"efficiencyScore"` // 0-100
}

// SLEntry describes one pod's scheduling info.
type SLEntry struct {
	PodName       string  `json:"podName"`
	Namespace     string  `json:"namespace"`
	Phase         string  `json:"phase"`
	NodeName      string  `json:"nodeName,omitempty"`
	ScheduleSec   float64 `json:"scheduleSec"` // time from creation to scheduled (0 if still pending)
	PendingReason string  `json:"pendingReason,omitempty"`
	Age           string  `json:"age"`
	RiskLevel     string  `json:"riskLevel"`
}

// SLNodeEntry per-node scheduling stats.
type SLNodeEntry struct {
	NodeName    string  `json:"nodeName"`
	PodCount    int     `json:"podCount"`
	AvgSchedSec float64 `json:"avgSchedSec"`
	SlowCount   int     `json:"slowCount"`
}

// SLNSEntry per-namespace scheduling stats.
type SLNSEntry struct {
	Namespace    string  `json:"namespace"`
	PodCount     int     `json:"podCount"`
	PendingCount int     `json:"pendingCount"`
	AvgSchedSec  float64 `json:"avgSchedSec"`
}

// SLIssue is a detected scheduling problem.
type SLIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleSchedulingLatency analyzes pod scheduling latency across the cluster.
// GET /api/operations/scheduling-latency
func (s *Server) handleSchedulingLatency(w http.ResponseWriter, r *http.Request) {
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

	result := SLResult{ScannedAt: time.Now(), ByPhase: make(map[string]int)}
	now := time.Now()
	var totalSchedSec float64
	var schedCount int
	nodeMap := make(map[string]*SLNodeEntry)
	nsMap := make(map[string]*SLNSEntry)

	for _, pod := range pods.Items {
		result.Summary.TotalPods++
		result.ByPhase[string(pod.Status.Phase)]++

		entry := SLEntry{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Phase:     string(pod.Status.Phase),
			NodeName:  pod.Spec.NodeName,
			Age:       now.Sub(pod.CreationTimestamp.Time).Round(time.Second).String(),
		}

		createdTime := pod.CreationTimestamp.Time

		// Find when pod was scheduled (PodScheduled condition)
		var scheduledTime *time.Time
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
				scheduledTime = &cond.LastTransitionTime.Time
				break
			}
		}

		if scheduledTime != nil {
			entry.ScheduleSec = scheduledTime.Sub(createdTime).Seconds()
			if entry.ScheduleSec < 0 {
				entry.ScheduleSec = 0
			}
			totalSchedSec += entry.ScheduleSec
			schedCount++

			if entry.ScheduleSec > result.Summary.MaxScheduleSec {
				result.Summary.MaxScheduleSec = entry.ScheduleSec
			}

			// Risk based on scheduling time
			if entry.ScheduleSec > 300 {
				result.Summary.VerySlowCount++
				result.Summary.SlowCount++
				entry.RiskLevel = "critical"
				result.SlowPods = append(result.SlowPods, entry)
				result.Issues = append(result.Issues, SLIssue{
					Severity: "critical", Type: "very-slow-schedule",
					Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
					Message:  fmt.Sprintf("Pod %s/%s took %.0fs to schedule — check node capacity and affinity rules", pod.Namespace, pod.Name, entry.ScheduleSec),
				})
			} else if entry.ScheduleSec > 60 {
				result.Summary.SlowCount++
				entry.RiskLevel = "high"
				result.SlowPods = append(result.SlowPods, entry)
			} else {
				entry.RiskLevel = "low"
			}

			// Node tracking
			if pod.Spec.NodeName != "" {
				nodeStat := slGetOrCreateNode(nodeMap, pod.Spec.NodeName)
				nodeStat.PodCount++
				nodeStat.AvgSchedSec += entry.ScheduleSec
				if entry.ScheduleSec > 60 {
					nodeStat.SlowCount++
				}
			}
		}

		// Handle pending pods
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			entry.RiskLevel = "high"
			entry.PendingReason = slPendingReason(pod)
			entry.ScheduleSec = now.Sub(createdTime).Seconds()

			// Check for Unschedulable condition
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					if cond.Reason == "Unschedulable" {
						result.Summary.Unschedulable++
						msg := cond.Message
						if msg == "" {
							msg = "No nodes match scheduling constraints"
						}
						entry.PendingReason = msg
						if slIsResourceShortage(msg) {
							result.Summary.NoNodeResources++
							result.Issues = append(result.Issues, SLIssue{
								Severity: "critical", Type: "resource-shortage",
								Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
								Message:  fmt.Sprintf("Pod %s/%s unschedulable: %s", pod.Namespace, pod.Name, msg),
							})
						} else {
							result.Issues = append(result.Issues, SLIssue{
								Severity: "warning", Type: "unschedulable",
								Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
								Message:  fmt.Sprintf("Pod %s/%s unschedulable: %s", pod.Namespace, pod.Name, msg),
							})
						}
					}
				}
			}

			result.PendingPods = append(result.PendingPods, entry)
		}

		if pod.Status.Phase == corev1.PodFailed {
			result.Summary.FailedPods++
		}
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.RunningPods++
		}

		// Namespace tracking
		nsStat := slGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.PodCount++
		if pod.Status.Phase == corev1.PodPending {
			nsStat.PendingCount++
		}
		if scheduledTime != nil {
			nsStat.AvgSchedSec += entry.ScheduleSec
		}
	}

	// Finalize averages
	if schedCount > 0 {
		result.Summary.AvgScheduleSec = totalSchedSec / float64(schedCount)
	}
	for _, nodeStat := range nodeMap {
		if nodeStat.PodCount > 0 {
			nodeStat.AvgSchedSec /= float64(nodeStat.PodCount)
		}
		result.ByNode = append(result.ByNode, *nodeStat)
	}
	for _, nsStat := range nsMap {
		if nsStat.PodCount > 0 {
			nsStat.AvgSchedSec /= float64(nsStat.PodCount)
		}
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.SlowPods, func(i, j int) bool {
		return result.SlowPods[i].ScheduleSec > result.SlowPods[j].ScheduleSec
	})
	if len(result.SlowPods) > 20 {
		result.SlowPods = result.SlowPods[:20]
	}
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].AvgSchedSec > result.ByNode[j].AvgSchedSec
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PendingCount > result.ByNamespace[j].PendingCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return slIssueRank(result.Issues[i].Severity) < slIssueRank(result.Issues[j].Severity)
	})

	result.Summary.EfficiencyScore = slScore(result.Summary)
	result.Recommendations = slGenRecs(result.Summary, result.PendingPods, result.SlowPods)

	writeJSON(w, result)
}

// slPendingReason extracts a human-readable reason for pending.
func slPendingReason(pod corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			if cond.Message != "" {
				return cond.Message
			}
			return cond.Reason
		}
	}
	if pod.Status.Phase == corev1.PodPending {
		return "ContainerCreating or waiting for scheduling"
	}
	return ""
}

// slIsResourceShortage checks if the message indicates resource constraints.
func slIsResourceShortage(msg string) bool {
	keywords := []string{"Insufficient cpu", "Insufficient memory", "node(s) had untolerated",
		"Insufficient", "exceeded quota", "node(s) resource"}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// slScore computes scheduling efficiency 0-100.
func slScore(s SLSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.Unschedulable * 10
	score -= s.NoNodeResources * 12
	score -= s.VerySlowCount * 6
	score -= (s.SlowCount - s.VerySlowCount) * 3
	score -= s.PendingPods * 4
	if score < 0 {
		score = 0
	}
	return score
}

// slGenRecs produces actionable advice.
func slGenRecs(s SLSummary, pending []SLEntry, slow []SLEntry) []string {
	var recs []string

	if s.NoNodeResources > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) unschedulable due to insufficient CPU/memory — add nodes or reduce requests", s.NoNodeResources))
	}
	if s.Unschedulable > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) are unschedulable — check node affinity, taints, and resource availability", s.Unschedulable))
	}
	if s.VerySlowCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) took >5min to schedule — check for pod anti-affinity thrashing or priority preemption storms", s.VerySlowCount))
	}
	if s.SlowCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) took >60s to schedule — consider priority classes or reducing scheduling constraints", s.SlowCount))
	}
	if s.PendingPods > 0 {
		top := ""
		if len(pending) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %s)", pending[0].Namespace, pending[0].PodName, slTruncate(pending[0].PendingReason, 60))
		}
		recs = append(recs, fmt.Sprintf("%d pod(s) currently pending%s", s.PendingPods, top))
	}
	if s.MaxScheduleSec > 300 {
		recs = append(recs, fmt.Sprintf("Slowest scheduling took %.0fs — investigate scheduler throughput and node autoscaler", s.MaxScheduleSec))
	}
	if s.EfficiencyScore < 70 {
		recs = append(recs, fmt.Sprintf("Scheduling efficiency score is %d/100 — multiple pods have scheduling issues", s.EfficiencyScore))
	}
	if s.PendingPods == 0 && s.SlowCount == 0 {
		recs = append(recs, "All pods scheduled quickly — good cluster capacity and scheduling posture")
	}

	return recs
}

func slTruncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func slGetOrCreateNode(m map[string]*SLNodeEntry, name string) *SLNodeEntry {
	if e, ok := m[name]; ok {
		return e
	}
	e := &SLNodeEntry{NodeName: name}
	m[name] = e
	return e
}

func slGetOrCreateNS(m map[string]*SLNSEntry, ns string) *SLNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &SLNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func slIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
