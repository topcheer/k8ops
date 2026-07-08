package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ESResult is the ephemeral storage compliance analysis.
type ESResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         ESSummary `json:"summary"`
	ByWorkload      []ESEntry `json:"byWorkload"`
	NoLimits        []ESEntry `json:"noLimits"`
	Issues          []ESIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// ESSummary aggregates ephemeral storage stats.
type ESSummary struct {
	TotalPods         int `json:"totalPods"`
	HasEphemeralLimit int `json:"hasEphemeralLimit"`
	NoEphemeralLimit  int `json:"noEphemeralLimit"`
	HasEmptyDirLimit  int `json:"hasEmptyDirLimit"`
	NoEmptyDirLimit   int `json:"noEmptyDirLimit"`
	UnboundedTmpfs    int `json:"unboundedTmpfs"`
	ComplianceScore   int `json:"complianceScore"`
}

// ESEntry describes one pod's ephemeral storage compliance.
type ESEntry struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	Workload      string   `json:"workload"` // owner kind/name
	Containers    int      `json:"containers"`
	HasLimit      bool     `json:"hasEphemeralLimit"`
	EmptyDirCount int      `json:"emptyDirCount"`
	EmptyDirSizes []string `json:"emptyDirSizes,omitempty"` // size limits
	RiskLevel     string   `json:"riskLevel"`
}

// ESIssue is a detected storage compliance problem.
type ESIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleEphemeralStorage checks container ephemeral storage limit compliance.
// GET /api/deployment/ephemeral-storage
func (s *Server) handleEphemeralStorage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := ESResult{ScannedAt: time.Now()}

	for _, pod := range pods.Items {
		// Skip completed pods
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		entry := ESEntry{
			PodName:    pod.Name,
			Namespace:  pod.Namespace,
			Containers: len(pod.Spec.Containers),
		}

		// Determine workload owner
		for _, ref := range pod.OwnerReferences {
			if ref.Kind != "" {
				entry.Workload = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
				break
			}
		}

		// Check container ephemeral storage limits
		hasEphemeralLimit := true
		for _, c := range pod.Spec.Containers {
			if c.Resources.Limits.StorageEphemeral().IsZero() {
				hasEphemeralLimit = false
				break
			}
		}
		entry.HasLimit = hasEphemeralLimit
		if hasEphemeralLimit {
			result.Summary.HasEphemeralLimit++
		} else {
			result.Summary.NoEphemeralLimit++
			result.NoLimits = append(result.NoLimits, entry)
		}

		// Check emptyDir volumes
		emptyDirCount := 0
		emptyDirUnbounded := false
		for _, vol := range pod.Spec.Volumes {
			if vol.EmptyDir != nil {
				emptyDirCount++
				result.Summary.HasEmptyDirLimit++
				if vol.EmptyDir.SizeLimit == nil || vol.EmptyDir.SizeLimit.IsZero() {
					emptyDirUnbounded = true
					entry.EmptyDirSizes = append(entry.EmptyDirSizes, fmt.Sprintf("%s:unbounded", vol.Name))
				} else {
					entry.EmptyDirSizes = append(entry.EmptyDirSizes, fmt.Sprintf("%s:%s", vol.Name, vol.EmptyDir.SizeLimit.String()))
				}
			}
		}
		entry.EmptyDirCount = emptyDirCount

		if emptyDirUnbounded {
			result.Summary.NoEmptyDirLimit++
			result.Summary.UnboundedTmpfs++
			result.Issues = append(result.Issues, ESIssue{
				Severity: "warning", Type: "unbounded-emptydir",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Pod %s/%s has unbounded emptyDir volume(s) — can fill node disk and cause eviction", pod.Namespace, pod.Name),
			})
		}

		// Check init containers too
		for _, c := range pod.Spec.InitContainers {
			if c.Resources.Limits.StorageEphemeral().IsZero() {
				result.Issues = append(result.Issues, ESIssue{
					Severity: "info", Type: "init-container-no-limit",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Init container %s in pod %s/%s has no ephemeral storage limit", c.Name, pod.Namespace, pod.Name),
				})
			}
		}

		// Risk level
		if !hasEphemeralLimit && emptyDirUnbounded {
			entry.RiskLevel = "high"
		} else if !hasEphemeralLimit || emptyDirUnbounded {
			entry.RiskLevel = "medium"
		} else {
			entry.RiskLevel = "low"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return esRiskRank(result.ByWorkload[i].RiskLevel) < esRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return esIssueRank(result.Issues[i].Severity) < esIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ComplianceScore = esScore(result.Summary)
	result.Recommendations = esGenRecs(result.Summary)

	writeJSON(w, result)
}

// esScore computes compliance score 0-100.
func esScore(s ESSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	// No ephemeral storage limits on containers
	score -= s.NoEphemeralLimit * 3
	// Unbounded emptyDir is worse
	score -= s.UnboundedTmpfs * 5
	if score < 0 {
		score = 0
	}
	return score
}

// esGenRecs produces actionable advice.
func esGenRecs(s ESSummary) []string {
	var recs []string

	if s.NoEphemeralLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have no ephemeral storage limit — add resources.limits.ephemeral-storage to prevent disk exhaustion", s.NoEphemeralLimit))
	}
	if s.UnboundedTmpfs > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have unbounded emptyDir volumes — set sizeLimit to prevent filling node disk and triggering evictions", s.UnboundedTmpfs))
	}
	if s.ComplianceScore < 70 {
		recs = append(recs, fmt.Sprintf("Ephemeral storage compliance score is %d/100 — add storage limits to prevent disk pressure", s.ComplianceScore))
	}
	if s.NoEphemeralLimit == 0 && s.UnboundedTmpfs == 0 {
		recs = append(recs, fmt.Sprintf("All %d pods have proper ephemeral storage limits — good disk pressure protection", s.TotalPods))
	}

	return recs
}

func esRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func esIssueRank(s string) int {
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
