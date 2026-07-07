package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RCResult is the resource contention & throttling analysis.
type RCResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	Summary         RCSummary   `json:"summary"`
	ThrottledPods   []RCEntry   `json:"throttledPods"`
	MemoryPressure  []RCEntry   `json:"memoryPressure"`
	NoCPULimits     []RCEntry   `json:"noCpuLimits"`
	NoMemoryLimits  []RCEntry   `json:"noMemoryLimits"`
	CPULimitTooLow  []RCEntry   `json:"cpuLimitTooLow"`
	ByNamespace     []RCNSEntry `json:"byNamespace"`
	Issues          []RCIssue   `json:"issues"`
	Recommendations []string    `json:"recommendations"`
}

// RCSummary aggregates resource contention statistics.
type RCSummary struct {
	TotalPods         int `json:"totalPods"`
	RestartedPods     int `json:"restartedPods"`  // restartCount > 0
	ThrottledPods     int `json:"throttledPods"`  // likely CPU throttled
	MemoryPressure    int `json:"memoryPressure"` // high restart + memory hint
	NoCPULimits       int `json:"noCpuLimits"`
	NoMemoryLimits    int `json:"noMemoryLimits"`
	CPULimitTooLow    int `json:"cpuLimitTooLow"`    // <100m
	MemoryLimitTooLow int `json:"memoryLimitTooLow"` // <128Mi
	ContentionScore   int `json:"contentionScore"`   // 0-100
}

// RCEntry describes one pod's resource contention status.
type RCEntry struct {
	PodName      string   `json:"podName"`
	Namespace    string   `json:"namespace"`
	NodeName     string   `json:"nodeName"`
	Phase        string   `json:"phase"`
	RestartCount int32    `json:"restartCount"`
	CPURequest   string   `json:"cpuRequest,omitempty"`
	CPULimit     string   `json:"cpuLimit,omitempty"`
	MemRequest   string   `json:"memRequest,omitempty"`
	MemLimit     string   `json:"memLimit,omitempty"`
	Violations   []string `json:"violations,omitempty"`
	RiskLevel    string   `json:"riskLevel"`
}

// RCNSEntry per-namespace contention stats.
type RCNSEntry struct {
	Namespace      string `json:"namespace"`
	PodCount       int    `json:"podCount"`
	RestartedCount int    `json:"restartedCount"`
	NoLimitsCount  int    `json:"noLimitsCount"`
}

// RCIssue is a detected contention problem.
type RCIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleResourceContention detects CPU throttling, memory pressure, and resource contention.
// GET /api/operations/resource-contention
func (s *Server) handleResourceContention(w http.ResponseWriter, r *http.Request) {
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

	// Get nodes for pressure detection
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build node pressure map: nodeName → has memory/cpu pressure
	nodePressure := make(map[string]bool)
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if (cond.Type == corev1.NodeMemoryPressure || cond.Type == corev1.NodeDiskPressure) &&
				cond.Status == corev1.ConditionTrue {
				nodePressure[node.Name] = true
			}
		}
	}

	result := RCResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*RCNSEntry)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := int32(0)
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}

		entry := RCEntry{
			PodName:      pod.Name,
			Namespace:    pod.Namespace,
			NodeName:     pod.Spec.NodeName,
			Phase:        string(pod.Status.Phase),
			RestartCount: totalRestarts,
		}

		// Analyze container resource configs
		hasCPULimit := false
		hasMemLimit := false
		cpuLimitMilli := int64(0)
		memLimitMB := 0.0

		for _, c := range pod.Spec.Containers {
			// CPU request/limit
			if req := c.Resources.Requests.Cpu(); req != nil {
				entry.CPURequest = req.String()
			}
			if lim := c.Resources.Limits.Cpu(); lim != nil {
				entry.CPULimit = lim.String()
				hasCPULimit = true
				cpuLimitMilli = lim.MilliValue()
			}
			// Memory request/limit
			if req := c.Resources.Requests.Memory(); req != nil {
				entry.MemRequest = req.String()
			}
			if lim := c.Resources.Limits.Memory(); lim != nil {
				entry.MemLimit = lim.String()
				hasMemLimit = true
				memLimitMB = float64(lim.Value()) / (1024 * 1024)
			}
		}

		violations := []string{}

		// No CPU limit → may be throttled by cgroup or starve others
		if !hasCPULimit {
			result.Summary.NoCPULimits++
			violations = append(violations, "No CPU limit — can consume unlimited CPU, may starve neighbors")
			result.NoCPULimits = append(result.NoCPULimits, entry)
		}

		// No memory limit → OOM risk for neighbors
		if !hasMemLimit {
			result.Summary.NoMemoryLimits++
			violations = append(violations, "No memory limit — OOM can cascade to co-located pods")
			result.NoMemoryLimits = append(result.NoMemoryLimits, entry)
		}

		// CPU limit too low (<100m = 0.1 core) → likely throttled
		if hasCPULimit && cpuLimitMilli < 100 {
			result.Summary.CPULimitTooLow++
			violations = append(violations, fmt.Sprintf("CPU limit %dm < 100m — likely CPU throttled under load", cpuLimitMilli))
			result.CPULimitTooLow = append(result.CPULimitTooLow, entry)
			result.Issues = append(result.Issues, RCIssue{
				Severity: "warning", Type: "cpu-limit-low",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Pod %s/%s CPU limit is %dm — too low, likely throttled under load", pod.Namespace, pod.Name, cpuLimitMilli),
			})
		}

		// Memory limit too low (<128Mi)
		if hasMemLimit && memLimitMB < 128 {
			result.Summary.MemoryLimitTooLow++
			violations = append(violations, fmt.Sprintf("Memory limit %.0fMi < 128Mi — likely OOMKilled under load", memLimitMB))
		}

		// Restart-based throttling detection
		if totalRestarts >= 3 {
			result.Summary.ThrottledPods++
			violations = append(violations, fmt.Sprintf("%d restarts — may indicate CPU throttling causing liveness probe failures", totalRestarts))
			result.ThrottledPods = append(result.ThrottledPods, entry)
			result.Issues = append(result.Issues, RCIssue{
				Severity: "warning", Type: "restart-throttling",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Pod %s/%s has %d restarts — may be CPU throttled causing probe timeouts", pod.Namespace, pod.Name, totalRestarts),
			})
			result.Summary.RestartedPods++
		} else if totalRestarts > 0 {
			result.Summary.RestartedPods++
		}

		// Memory pressure on node
		if nodePressure[pod.Spec.NodeName] {
			result.Summary.MemoryPressure++
			violations = append(violations, "Node has MemoryPressure/DiskPressure — pod may be affected")
			result.MemoryPressure = append(result.MemoryPressure, entry)
			result.Issues = append(result.Issues, RCIssue{
				Severity: "critical", Type: "node-pressure",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Pod %s/%s on node %s under MemoryPressure/DiskPressure — risk of OOM/eviction", pod.Namespace, pod.Name, pod.Spec.NodeName),
			})
		}

		entry.Violations = violations
		entry.RiskLevel = rcAssessRisk(entry, nodePressure)

		// Namespace tracking
		nsStat := rcGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.PodCount++
		if totalRestarts > 0 {
			nsStat.RestartedCount++
		}
		if !hasCPULimit || !hasMemLimit {
			nsStat.NoLimitsCount++
		}
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.ThrottledPods, func(i, j int) bool {
		return result.ThrottledPods[i].RestartCount > result.ThrottledPods[j].RestartCount
	})
	if len(result.ThrottledPods) > 20 {
		result.ThrottledPods = result.ThrottledPods[:20]
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].NoLimitsCount > result.ByNamespace[j].NoLimitsCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return rcIssueRank(result.Issues[i].Severity) < rcIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ContentionScore = rcScore(result.Summary)
	result.Recommendations = rcGenRecs(result.Summary, result.ThrottledPods, result.NoCPULimits)

	writeJSON(w, result)
}

// rcAssessRisk determines risk level.
func rcAssessRisk(entry RCEntry, nodePressure map[string]bool) string {
	if nodePressure[entry.NodeName] {
		return "critical"
	}
	if entry.RestartCount >= 5 {
		return "high"
	}
	if entry.RestartCount >= 3 {
		return "medium"
	}
	if len(entry.Violations) > 0 {
		return "medium"
	}
	return "low"
}

// rcScore computes 0-100.
func rcScore(s RCSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.MemoryPressure * 12
	score -= s.ThrottledPods * 6
	score -= s.CPULimitTooLow * 4
	score -= s.NoCPULimits * 2
	score -= s.NoMemoryLimits * 3
	if score < 0 {
		score = 0
	}
	return score
}

// rcGenRecs produces actionable advice.
func rcGenRecs(s RCSummary, throttled []RCEntry, noCPU []RCEntry) []string {
	var recs []string

	if s.MemoryPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) on nodes with MemoryPressure/DiskPressure — risk of OOM/eviction, add node capacity", s.MemoryPressure))
	}
	if s.ThrottledPods > 0 {
		top := ""
		if len(throttled) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %d restarts)", throttled[0].Namespace, throttled[0].PodName, throttled[0].RestartCount)
		}
		recs = append(recs, fmt.Sprintf("%d pod(s) likely CPU throttled%s — increase CPU limits or fix liveness probes", s.ThrottledPods, top))
	}
	if s.CPULimitTooLow > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have CPU limit <100m — likely throttled under load, increase to 250m+", s.CPULimitTooLow))
	}
	if s.NoCPULimits > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have no CPU limit — can starve neighbors, add CPU limits", s.NoCPULimits))
	}
	if s.NoMemoryLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have no memory limit — OOM can cascade, add memory limits", s.NoMemoryLimits))
	}
	if s.MemoryLimitTooLow > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have memory limit <128Mi — likely OOMKilled, increase to 256Mi+", s.MemoryLimitTooLow))
	}
	if s.ContentionScore < 70 {
		recs = append(recs, fmt.Sprintf("Resource contention score is %d/100 — multiple pods have resource issues", s.ContentionScore))
	}
	if s.MemoryPressure == 0 && s.ThrottledPods == 0 {
		recs = append(recs, "No significant resource contention detected — good performance posture")
	}

	return recs
}

func rcGetOrCreateNS(m map[string]*RCNSEntry, ns string) *RCNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &RCNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func rcIssueRank(s string) int {
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
