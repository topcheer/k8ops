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

// PEResult is the pod eviction & pressure history analysis.
type PEResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         PESummary      `json:"summary"`
	ByNode          []PENodeEntry  `json:"byNode"`
	ByNamespace     []PENSEntry    `json:"byNamespace"`
	ByCause         map[string]int `json:"byCause"`
	RecentEvictions []PEEntry      `json:"recentEvictions"`
	Issues          []PEIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// PESummary aggregates eviction statistics.
type PESummary struct {
	TotalPods        int `json:"totalPods"`
	EvictedPods      int `json:"evictedPods"`
	RecentEvictions  int `json:"recentEvictions"` // last 24h
	EvictionNodes    int `json:"evictionNodes"`   // nodes with evictions
	MemoryEvictions  int `json:"memoryEvictions"`
	DiskEvictions    int `json:"diskEvictions"`
	PIDEvictions     int `json:"pidEvictions"`
	UnknownEvictions int `json:"unknownEvictions"`
	HealthScore      int `json:"healthScore"` // 0-100
}

// PEEntry describes one evicted pod.
type PEEntry struct {
	PodName   string    `json:"podName"`
	Namespace string    `json:"namespace"`
	NodeName  string    `json:"nodeName"`
	EvictedAt time.Time `json:"evictedAt"`
	Cause     string    `json:"cause"`
	Message   string    `json:"message"`
	AgeHours  float64   `json:"ageHours"`
}

// PENodeEntry per-node eviction stats.
type PENodeEntry struct {
	NodeName      string `json:"nodeName"`
	EvictionCount int    `json:"evictionCount"`
	MemoryEvict   int    `json:"memoryEvict"`
	DiskEvict     int    `json:"diskEvict"`
	PIDEvict      int    `json:"pidEvict"`
	RiskLevel     string `json:"riskLevel"`
}

// PENSEntry per-namespace eviction stats.
type PENSEntry struct {
	Namespace     string `json:"namespace"`
	EvictionCount int    `json:"evictionCount"`
}

// PEIssue is a detected eviction problem.
type PEIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handlePodEviction tracks pod evictions and correlates with node pressure.
// GET /api/operations/pod-evictions
func (s *Server) handlePodEviction(w http.ResponseWriter, r *http.Request) {
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

	now := time.Now()
	result := PEResult{ScannedAt: now, ByCause: make(map[string]int)}
	nodeMap := make(map[string]*PENodeEntry)
	nsMap := make(map[string]*PENSEntry)

	result.Summary.TotalPods = len(pods.Items)

	for _, pod := range pods.Items {
		// Check for evicted pods (phase=Failed, reason=Evicted)
		if pod.Status.Phase != corev1.PodFailed {
			continue
		}

		isEvicted := false
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "Evicted" {
				isEvicted = true
				break
			}
		}

		// Also check pod-level reason
		if !isEvicted {
			for _, cond := range pod.Status.Conditions {
				if cond.Reason == "Evicted" || strings.Contains(cond.Message, "Evicted") {
					isEvicted = true
					break
				}
			}
		}

		// Check status message for eviction
		if !isEvicted && strings.Contains(pod.Status.Message, "The node was low on resource") {
			isEvicted = true
		}

		if !isEvicted {
			continue
		}

		result.Summary.EvictedPods++

		// Parse eviction cause from status message
		cause := "unknown"
		msg := pod.Status.Message
		if msg == "" {
			for _, cond := range pod.Status.Conditions {
				if cond.Message != "" {
					msg = cond.Message
					break
				}
			}
		}

		lowerMsg := strings.ToLower(msg)
		if strings.Contains(lowerMsg, "memory") {
			cause = "memory"
			result.Summary.MemoryEvictions++
		} else if strings.Contains(lowerMsg, "disk") || strings.Contains(lowerMsg, "ephemeral storage") {
			cause = "disk"
			result.Summary.DiskEvictions++
		} else if strings.Contains(lowerMsg, "pid") {
			cause = "pid"
			result.Summary.PIDEvictions++
		} else {
			result.Summary.UnknownEvictions++
		}
		result.ByCause[cause]++

		// Determine eviction time
		var evictedAt time.Time
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				evictedAt = cs.State.Terminated.FinishedAt.Time
				break
			}
		}
		if evictedAt.IsZero() {
			evictedAt = pod.DeletionTimestamp.Time
		}
		if evictedAt.IsZero() {
			evictedAt = pod.CreationTimestamp.Time
		}

		ageHours := now.Sub(evictedAt).Hours()
		if ageHours < 0 {
			ageHours = 0
		}

		entry := PEEntry{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			NodeName:  pod.Spec.NodeName,
			EvictedAt: evictedAt,
			Cause:     cause,
			Message:   peTruncate(msg, 120),
			AgeHours:  ageHours,
		}

		// Track recent (24h)
		if ageHours <= 24 {
			result.Summary.RecentEvictions++
			result.RecentEvictions = append(result.RecentEvictions, entry)
		}

		// Per-node tracking
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			nodeName = "unknown"
		}
		nodeStat := peGetOrCreateNode(nodeMap, nodeName)
		nodeStat.EvictionCount++
		switch cause {
		case "memory":
			nodeStat.MemoryEvict++
		case "disk":
			nodeStat.DiskEvict++
		case "pid":
			nodeStat.PIDEvict++
		}

		// Per-namespace tracking
		nsStat := peGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.EvictionCount++
	}

	// Finalize node stats
	for _, ns := range nodeMap {
		ns.RiskLevel = peAssessNodeRisk(ns)
		result.ByNode = append(result.ByNode, *ns)
		if ns.EvictionCount > 0 {
			result.Summary.EvictionNodes++
		}
	}

	// Generate issues for high-eviction nodes
	for _, node := range result.ByNode {
		if node.EvictionCount >= 5 {
			result.Issues = append(result.Issues, PEIssue{
				Severity: "warning", Type: "high-eviction-node",
				Resource: node.NodeName,
				Message:  fmt.Sprintf("Node %s has %d evicted pods (memory: %d, disk: %d, pid: %d) — investigate node resource capacity", node.NodeName, node.EvictionCount, node.MemoryEvict, node.DiskEvict, node.PIDEvict),
			})
		}
	}

	if result.Summary.RecentEvictions >= 3 {
		result.Issues = append(result.Issues, PEIssue{
			Severity: "warning", Type: "recent-eviction-spike",
			Resource: "cluster",
			Message:  fmt.Sprintf("%d pods evicted in the last 24 hours — possible resource pressure or node instability", result.Summary.RecentEvictions),
		})
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	// Sort
	sort.Slice(result.RecentEvictions, func(i, j int) bool {
		return result.RecentEvictions[i].AgeHours < result.RecentEvictions[j].AgeHours
	})
	if len(result.RecentEvictions) > 20 {
		result.RecentEvictions = result.RecentEvictions[:20]
	}
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].EvictionCount > result.ByNode[j].EvictionCount
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].EvictionCount > result.ByNamespace[j].EvictionCount
	})
	if len(result.ByNamespace) > 15 {
		result.ByNamespace = result.ByNamespace[:15]
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return peIssueRank(result.Issues[i].Severity) < peIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = peScore(result.Summary)
	result.Recommendations = peGenRecs(result.Summary, result.ByNode)

	writeJSON(w, result)
}

// peAssessNodeRisk determines risk level based on eviction count.
func peAssessNodeRisk(node *PENodeEntry) string {
	if node.EvictionCount >= 10 {
		return "critical"
	}
	if node.EvictionCount >= 5 {
		return "high"
	}
	if node.EvictionCount >= 2 {
		return "medium"
	}
	return "low"
}

// peScore computes health score 0-100.
func peScore(s PESummary) int {
	score := 100
	score -= s.RecentEvictions * 8
	score -= s.MemoryEvictions * 3
	score -= s.DiskEvictions * 3
	score -= s.PIDEvictions * 2
	if score < 0 {
		score = 0
	}
	return score
}

// peGenRecs produces actionable advice.
func peGenRecs(s PESummary, byNode []PENodeEntry) []string {
	var recs []string

	if s.RecentEvictions > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) evicted in last 24h — investigate node resource pressure immediately", s.RecentEvictions))
	}
	if s.MemoryEvictions > 0 {
		recs = append(recs, fmt.Sprintf("%d memory evictions — nodes are running out of memory, increase node RAM or add memory limits to pods", s.MemoryEvictions))
	}
	if s.DiskEvictions > 0 {
		recs = append(recs, fmt.Sprintf("%d disk/ephemeral-storage evictions — nodes are running out of disk, clean up images/logs or add storage", s.DiskEvictions))
	}
	if s.PIDEvictions > 0 {
		recs = append(recs, fmt.Sprintf("%d PID evictions — too many processes per node, reduce pod density or increase pid limit", s.PIDEvictions))
	}
	if len(byNode) > 0 && byNode[0].EvictionCount >= 5 {
		recs = append(recs, fmt.Sprintf("Node %s has %d evictions — consider cordoning or adding resources", byNode[0].NodeName, byNode[0].EvictionCount))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Eviction health score is %d/100 — cluster has active resource pressure", s.HealthScore))
	}
	if s.EvictedPods == 0 {
		recs = append(recs, "No pod evictions detected — good resource management posture")
	}

	return recs
}

func peGetOrCreateNode(m map[string]*PENodeEntry, name string) *PENodeEntry {
	if e, ok := m[name]; ok {
		return e
	}
	e := &PENodeEntry{NodeName: name}
	m[name] = e
	return e
}

func peGetOrCreateNS(m map[string]*PENSEntry, ns string) *PENSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &PENSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func peTruncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func peIssueRank(s string) int {
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
