package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RLResult is the API server request latency analysis.
type RLResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         RLSummary      `json:"summary"`
	ByVerbs         map[string]int `json:"byVerbs"`
	RecentSlowPods  []RLEntry      `json:"recentSlowPods"`
	Issues          []RLIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// RLSummary aggregates responsiveness stats.
type RLSummary struct {
	TotalPods           int  `json:"totalPods"`
	NotReadyPods        int  `json:"notReadyPods"`
	LongStartingPods    int  `json:"longStartingPods"`    // pending >2m
	ContainerWait       int  `json:"containerWait"`       // high container start latency
	APIResponsive       bool `json:"apiResponsive"`       // API server responded
	ResponsivenessScore int  `json:"responsivenessScore"` // 0-100
}

// RLEntry describes a slow-to-start pod.
type RLEntry struct {
	PodName                string  `json:"podName"`
	Namespace              string  `json:"namespace"`
	NodeName               string  `json:"nodeName"`
	Phase                  string  `json:"phase"`
	PendingMin             float64 `json:"pendingMinutes"` // time in pending state
	ContainerStartDelayMin float64 `json:"containerStartDelayMin"`
	RiskLevel              string  `json:"riskLevel"`
}

// RLIssue is a detected responsiveness problem.
type RLIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleResponsiveness monitors API server request latency and pod start latency.
// GET /api/operations/api-latency
func (s *Server) handleResponsiveness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	now := time.Now()
	result := RLResult{ScannedAt: now, ByVerbs: make(map[string]int)}
	result.Summary.APIResponsive = true

	// List pods and check for slow starts
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Summary.APIResponsive = false
		result.Issues = append(result.Issues, RLIssue{
			Severity: "critical", Type: "api-unresponsive",
			Resource: "kube-apiserver",
			Message:  fmt.Sprintf("Failed to list pods: %v — API server may be slow or overloaded", err),
		})
		result.Summary.ResponsivenessScore = 0
		writeJSON(w, result)
		return
	}

	result.Summary.TotalPods = len(pods.Items)

	for _, pod := range pods.Items {
		// Skip completed pods
		if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
			continue
		}

		// Check not-ready pods
		isReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
				break
			}
		}

		if !isReady && pod.Status.Phase == "Running" {
			result.Summary.NotReadyPods++
		}

		// Check pending pods (slow scheduling)
		if pod.Status.Phase == "Pending" && !pod.CreationTimestamp.IsZero() {
			pendingMin := now.Sub(pod.CreationTimestamp.Time).Minutes()
			if pendingMin > 2 { // >2 minutes pending
				result.Summary.LongStartingPods++
				entry := RLEntry{
					PodName:    pod.Name,
					Namespace:  pod.Namespace,
					NodeName:   pod.Spec.NodeName,
					Phase:      string(pod.Status.Phase),
					PendingMin: pendingMin,
					RiskLevel:  rlAssessRisk(pendingMin),
				}
				result.RecentSlowPods = append(result.RecentSlowPods, entry)

				if pendingMin > 5 {
					result.Issues = append(result.Issues, RLIssue{
						Severity: "warning", Type: "slow-scheduling",
						Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
						Message:  fmt.Sprintf("Pod %s/%s has been pending for %.1f minutes — possible resource shortage or scheduling constraints", pod.Namespace, pod.Name, pendingMin),
					})
				}
			}
		}

		// Check container start delay
		if pod.Status.StartTime != nil {
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Terminated != nil && cs.State.Terminated.StartedAt.After(pod.Status.StartTime.Time) {
					delayMin := cs.State.Terminated.StartedAt.Sub(pod.Status.StartTime.Time).Minutes()
					if delayMin > 1 {
						result.Summary.ContainerWait++
						result.Issues = append(result.Issues, RLIssue{
							Severity: "info", Type: "slow-container-start",
							Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
							Message:  fmt.Sprintf("Container %s in pod %s/%s took %.1f minutes to start — check image pull and init containers", cs.Name, pod.Namespace, pod.Name, delayMin),
						})
					}
				}
				// Also check running containers
				if cs.State.Running != nil && cs.State.Running.StartedAt.After(pod.Status.StartTime.Time) {
					delayMin := cs.State.Running.StartedAt.Sub(pod.Status.StartTime.Time).Minutes()
					if delayMin > 1 {
						result.Summary.ContainerWait++
						if entry, ok := rlFindOrCreateEntry(&result, pod.Name, pod.Namespace, pod.Spec.NodeName, string(pod.Status.Phase)); ok {
							entry.ContainerStartDelayMin = delayMin
						}
					}
				}
			}
		}
	}

	// Sort
	sort.Slice(result.RecentSlowPods, func(i, j int) bool {
		return result.RecentSlowPods[i].PendingMin > result.RecentSlowPods[j].PendingMin
	})
	if len(result.RecentSlowPods) > 20 {
		result.RecentSlowPods = result.RecentSlowPods[:20]
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return rlIssueRank(result.Issues[i].Severity) < rlIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ResponsivenessScore = rlScore(result.Summary)
	result.Recommendations = rlGenRecs(result.Summary, result.RecentSlowPods)

	writeJSON(w, result)
}

// rlFindOrCreateEntry finds or creates an entry in RecentSlowPods.
func rlFindOrCreateEntry(result *RLResult, podName, ns, node, phase string) (*RLEntry, bool) {
	for i := range result.RecentSlowPods {
		if result.RecentSlowPods[i].PodName == podName && result.RecentSlowPods[i].Namespace == ns {
			return &result.RecentSlowPods[i], true
		}
	}
	entry := RLEntry{PodName: podName, Namespace: ns, NodeName: node, Phase: phase}
	result.RecentSlowPods = append(result.RecentSlowPods, entry)
	return &result.RecentSlowPods[len(result.RecentSlowPods)-1], true
}

// rlAssessRisk determines risk level.
func rlAssessRisk(pendingMin float64) string {
	if pendingMin > 10 {
		return "high"
	}
	if pendingMin > 5 {
		return "medium"
	}
	return "low"
}

// rlScore computes responsiveness score 0-100.
func rlScore(s RLSummary) int {
	if !s.APIResponsive {
		return 0
	}
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.LongStartingPods * 8
	score -= s.NotReadyPods * 5
	score -= s.ContainerWait * 3
	if score < 0 {
		score = 0
	}
	return score
}

// rlGenRecs produces actionable advice.
func rlGenRecs(s RLSummary, slowPods []RLEntry) []string {
	var recs []string

	if !s.APIResponsive {
		recs = append(recs, "API server is not responding — check control plane health immediately")
	}
	if s.LongStartingPods > 0 {
		top := ""
		if len(slowPods) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %.1fm pending)", slowPods[0].Namespace, slowPods[0].PodName, slowPods[0].PendingMin)
		}
		recs = append(recs, fmt.Sprintf("%d pod(s) have been pending >2 minutes%s — check resource availability and scheduling constraints", s.LongStartingPods, top))
	}
	if s.NotReadyPods > 0 {
		recs = append(recs, fmt.Sprintf("%d running pod(s) are not ready — check liveness/readiness probes and container health", s.NotReadyPods))
	}
	if s.ContainerWait > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) had slow start (>1m delay) — check image pull time and init container chains", s.ContainerWait))
	}
	if s.ResponsivenessScore < 70 {
		recs = append(recs, fmt.Sprintf("API responsiveness score is %d/100 — cluster may be under resource pressure", s.ResponsivenessScore))
	}
	if s.LongStartingPods == 0 && s.NotReadyPods == 0 && s.APIResponsive {
		recs = append(recs, fmt.Sprintf("All %d pods are responsive — good cluster health (score: %d/100)", s.TotalPods, s.ResponsivenessScore))
	}

	return recs
}

func rlIssueRank(s string) int {
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
