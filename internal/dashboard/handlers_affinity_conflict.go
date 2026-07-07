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

// AffResult is the affinity/anti-affinity conflict analysis.
type AffResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         AffSummary `json:"summary"`
	Conflicts       []AffEntry `json:"conflicts"`   // pods with unsatisfiable rules
	PendingPods     []AffEntry `json:"pendingPods"` // pending due to affinity
	ByWorkload      []AffEntry `json:"byWorkload"`
	HasAntiAffinity []AffEntry `json:"hasAntiAffinity"` // workloads using anti-affinity
	Issues          []AffIssue `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// AffSummary aggregates affinity conflict statistics.
type AffSummary struct {
	TotalPods                 int `json:"totalPods"`
	PendingPods               int `json:"pendingPods"`
	PendingDueToAffinity      int `json:"pendingDueToAffinity"`
	WorkloadsWithAntiAffinity int `json:"workloadsWithAntiAffinity"`
	Conflicts                 int `json:"conflicts"` // hard anti-affinity that can't be satisfied
	RequiredDuringScheduling  int `json:"requiredDuringScheduling"`
	PreferredOnly             int `json:"preferredOnly"`
	HealthScore               int `json:"healthScore"` // 0-100
}

// AffEntry describes one pod/workload's affinity status.
type AffEntry struct {
	PodName          string   `json:"podName,omitempty"`
	Workload         string   `json:"workload,omitempty"`
	Namespace        string   `json:"namespace"`
	NodeName         string   `json:"nodeName,omitempty"`
	Phase            string   `json:"phase"`
	HasAffinity      bool     `json:"hasAffinity"`
	HasAntiAffinity  bool     `json:"hasAntiAffinity"`
	AntiAffinityType string   `json:"antiAffinityType,omitempty"` // required / preferred
	TopologyKey      string   `json:"topologyKey,omitempty"`
	MatchLabels      []string `json:"matchLabels,omitempty"`
	PendingReason    string   `json:"pendingReason,omitempty"`
	RiskLevel        string   `json:"riskLevel"`
}

// AffIssue is a detected affinity problem.
type AffIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleAffinityConflict detects pods stuck due to unsatisfiable affinity rules.
// GET /api/product/affinity-conflict
func (s *Server) handleAffinityConflict(w http.ResponseWriter, r *http.Request) {
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

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build topology domain map: topologyKey → set of domains with at least one node
	topologyDomains := make(map[string]map[string]bool) // key → domain → true
	for _, node := range nodes.Items {
		for key, val := range node.Labels {
			if isTopologyLabel(key) {
				if topologyDomains[key] == nil {
					topologyDomains[key] = make(map[string]bool)
				}
				topologyDomains[key][val] = true
			}
		}
	}

	result := AffResult{ScannedAt: time.Now()}
	processedWorkloads := make(map[string]bool)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		entry := AffEntry{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			NodeName:  pod.Spec.NodeName,
			Phase:     string(pod.Status.Phase),
		}

		// Get workload name
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" || ref.Kind == "Deployment" ||
				ref.Kind == "StatefulSet" || ref.Kind == "DaemonSet" {
				wlName = ref.Name
				break
			}
		}
		entry.Workload = wlName

		affinity := pod.Spec.Affinity
		hasAntiAffinity := false
		antiAffType := ""

		if affinity != nil && affinity.PodAntiAffinity != nil {
			paa := affinity.PodAntiAffinity
			if len(paa.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				hasAntiAffinity = true
				antiAffType = "required"
				result.Summary.RequiredDuringScheduling++
				for _, term := range paa.RequiredDuringSchedulingIgnoredDuringExecution {
					entry.TopologyKey = term.TopologyKey
					for k, v := range term.LabelSelector.MatchLabels {
						entry.MatchLabels = append(entry.MatchLabels, fmt.Sprintf("%s=%s", k, v))
					}
				}
			} else if len(paa.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
				hasAntiAffinity = true
				antiAffType = "preferred"
				result.Summary.PreferredOnly++
				for _, term := range paa.PreferredDuringSchedulingIgnoredDuringExecution {
					entry.TopologyKey = term.PodAffinityTerm.TopologyKey
				}
			}
		}

		entry.HasAffinity = affinity != nil
		entry.HasAntiAffinity = hasAntiAffinity
		entry.AntiAffinityType = antiAffType

		if hasAntiAffinity && !processedWorkloads[wlName] {
			processedWorkloads[wlName] = true
			result.Summary.WorkloadsWithAntiAffinity++
			result.HasAntiAffinity = append(result.HasAntiAffinity, entry)
		}

		// Check pending pods for affinity issues
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			pendingReason := affGetPendingReason(pod)
			entry.PendingReason = pendingReason

			// Check if pending is due to affinity constraints
			if affIsAffinityRelated(pendingReason) {
				result.Summary.PendingDueToAffinity++
				entry.RiskLevel = "high"
				result.PendingPods = append(result.PendingPods, entry)

				// Check if topology domain is too small
				if entry.TopologyKey != "" {
					domains := topologyDomains[entry.TopologyKey]
					if len(domains) <= 1 && antiAffType == "required" {
						result.Summary.Conflicts++
						entry.RiskLevel = "critical"
						result.Conflicts = append(result.Conflicts, entry)
						result.Issues = append(result.Issues, AffIssue{
							Severity: "critical", Type: "unsatisfiable-anti-affinity",
							Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
							Message: fmt.Sprintf("Pod %s/%s requires podAntiAffinity on topologyKey '%s' but cluster only has %d domain(s) — rule cannot be satisfied",
								pod.Namespace, pod.Name, entry.TopologyKey, len(domains)),
						})
					}
				}

				if entry.RiskLevel != "critical" {
					result.Issues = append(result.Issues, AffIssue{
						Severity: "warning", Type: "affinity-pending",
						Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
						Message:  fmt.Sprintf("Pod %s/%s is Pending due to affinity constraints: %s", pod.Namespace, pod.Name, affTruncate(pendingReason, 80)),
					})
				}
			}
		}

		if entry.RiskLevel == "" {
			entry.RiskLevel = affAssessRisk(entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort
	sort.Slice(result.Conflicts, func(i, j int) bool {
		return result.Conflicts[i].Namespace < result.Conflicts[j].Namespace
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return affIssueRank(result.Issues[i].Severity) < affIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = affScore(result.Summary)
	result.Recommendations = affGenRecs(result.Summary, result.Conflicts, result.PendingPods)

	writeJSON(w, result)
}

// affGetPendingReason extracts the scheduling failure reason.
func affGetPendingReason(pod corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			if cond.Message != "" {
				return cond.Message
			}
			return cond.Reason
		}
	}
	return ""
}

// affIsAffinityRelated checks if the pending reason is affinity-related.
func affIsAffinityRelated(reason string) bool {
	keywords := []string{"anti-affinity", "affinity", "node(s) didn't match",
		"topology", "pod anti-affinity", "node selector"}
	lower := strings.ToLower(reason)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isTopologyLabel checks if a label key is a common topology domain.
func isTopologyLabel(key string) bool {
	topologyKeys := []string{
		"kubernetes.io/hostname",
		"topology.kubernetes.io/zone",
		"topology.kubernetes.io/region",
		"topology.csi.io/node",
		"topology.rook.io/region",
	}
	for _, tk := range topologyKeys {
		if key == tk {
			return true
		}
	}
	return false
}

// affAssessRisk determines risk level.
func affAssessRisk(entry AffEntry) string {
	if entry.Phase == "Pending" && entry.HasAntiAffinity {
		return "high"
	}
	if entry.HasAntiAffinity && entry.AntiAffinityType == "required" {
		return "medium"
	}
	return "low"
}

// affScore computes health score 0-100.
func affScore(s AffSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.Conflicts * 15
	score -= s.PendingDueToAffinity * 8
	score -= (s.WorkloadsWithAntiAffinity - s.PreferredOnly) * 2 // required anti-affinity is risky
	if score < 0 {
		score = 0
	}
	return score
}

// affGenRecs produces actionable advice.
func affGenRecs(s AffSummary, conflicts []AffEntry, pending []AffEntry) []string {
	var recs []string

	if s.Conflicts > 0 {
		top := ""
		if len(conflicts) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s on topologyKey '%s')", conflicts[0].Namespace, conflicts[0].Workload, conflicts[0].TopologyKey)
		}
		recs = append(recs, fmt.Sprintf("%d pod(s) have unsatisfiable anti-affinity rules%s — add nodes in more topology domains or change to preferred", s.Conflicts, top))
	}
	if s.PendingDueToAffinity > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) are Pending due to affinity constraints — review topologyKey domains and node labels", s.PendingDueToAffinity))
	}
	if s.RequiredDuringScheduling > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) use required (hard) anti-affinity — consider switching to preferred (soft) for more flexibility", s.RequiredDuringScheduling))
	}
	if s.WorkloadsWithAntiAffinity > 0 && s.PreferredOnly == s.WorkloadsWithAntiAffinity {
		recs = append(recs, "All anti-affinity rules are preferred (soft) — good practice for resilience")
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Affinity health score is %d/100 — review scheduling constraints", s.HealthScore))
	}
	if s.Conflicts == 0 && s.PendingDueToAffinity == 0 {
		recs = append(recs, "No affinity conflicts detected — good scheduling posture")
	}

	return recs
}

func affTruncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func affIssueRank(s string) int {
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
