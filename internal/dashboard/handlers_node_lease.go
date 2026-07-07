package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NLResult is the node lease & heartbeat health analysis.
type NLResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         NLSummary      `json:"summary"`
	ByNode          []NLEntry      `json:"byNode"`
	StaleHeartbeat  []NLEntry      `json:"staleHeartbeat"` // >40s since last heartbeat
	NoLease         []NLEntry      `json:"noLease"`        // no lease object found
	ByCondition     map[string]int `json:"byCondition"`    // node condition counts
	Issues          []NLIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// NLSummary aggregates node heartbeat statistics.
type NLSummary struct {
	TotalNodes         int     `json:"totalNodes"`
	ReadyNodes         int     `json:"readyNodes"`
	NotReadyNodes      int     `json:"notReadyNodes"`
	StaleHeartbeat     int     `json:"staleHeartbeat"`     // last heartbeat >40s
	VeryStaleHeartbeat int     `json:"veryStaleHeartbeat"` // last heartbeat >2min
	NoLease            int     `json:"noLease"`            // no Lease object
	AvgHeartbeatAgeSec float64 `json:"avgHeartbeatAgeSec"` // avg seconds since last heartbeat
	OldestHeartbeatSec float64 `json:"oldestHeartbeatSec"`
	HealthScore        int     `json:"healthScore"` // 0-100
}

// NLEntry describes one node's heartbeat status.
type NLEntry struct {
	NodeName          string    `json:"nodeName"`
	Ready             bool      `json:"ready"`
	LastHeartbeatTime time.Time `json:"lastHeartbeatTime"`
	HeartbeatAgeSec   float64   `json:"heartbeatAgeSec"`
	LeaseExists       bool      `json:"leaseExists"`
	LeaseHolder       string    `json:"leaseHolder,omitempty"`
	KubeletVersion    string    `json:"kubeletVersion,omitempty"`
	Conditions        []string  `json:"conditions,omitempty"` // active negative conditions
	RiskLevel         string    `json:"riskLevel"`
}

// NLIssue is a detected heartbeat problem.
type NLIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleNodeLease checks node lease & heartbeat health across the cluster.
// GET /api/operations/node-lease
func (s *Server) handleNodeLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get all leases in kube-node-lease namespace
	leases, err := rc.clientset.CoordinationV1().Leases("kube-node-lease").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build lease map: node name → lease
	leaseMap := make(map[string]*coordinationv1.Lease)
	if leases != nil {
		for i := range leases.Items {
			lease := &leases.Items[i]
			leaseMap[lease.Name] = lease
		}
	}

	now := time.Now()
	result := NLResult{ScannedAt: now, ByCondition: make(map[string]int)}
	var totalAge float64
	var nodesWithHeartbeat int

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		entry := NLEntry{
			NodeName: node.Name,
		}

		// Check Ready condition
		entry.Ready = isNodeReady(&node)
		if entry.Ready {
			result.Summary.ReadyNodes++
		} else {
			result.Summary.NotReadyNodes++
		}

		// Kubelet version
		nodeInfo := node.Status.NodeInfo
		entry.KubeletVersion = nodeInfo.KubeletVersion

		// Collect active negative conditions
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				entry.Conditions = append(entry.Conditions, string(cond.Type))
				result.ByCondition[string(cond.Type)]++
			}
		}

		// Check lease
		lease, hasLease := leaseMap[node.Name]
		entry.LeaseExists = hasLease

		if hasLease && lease.Spec.RenewTime != nil {
			entry.LastHeartbeatTime = lease.Spec.RenewTime.Time
			if lease.Spec.HolderIdentity != nil {
				entry.LeaseHolder = *lease.Spec.HolderIdentity
			}
			age := now.Sub(lease.Spec.RenewTime.Time).Seconds()
			entry.HeartbeatAgeSec = age
			totalAge += age
			nodesWithHeartbeat++

			if age > result.Summary.OldestHeartbeatSec {
				result.Summary.OldestHeartbeatSec = age
			}

			// Stale heartbeat detection
			if age > 120 {
				result.Summary.VeryStaleHeartbeat++
				result.Summary.StaleHeartbeat++
				entry.RiskLevel = "critical"
				result.StaleHeartbeat = append(result.StaleHeartbeat, entry)
				result.Issues = append(result.Issues, NLIssue{
					Severity: "critical", Type: "very-stale-heartbeat",
					Resource: node.Name,
					Message:  fmt.Sprintf("Node %s heartbeat is %.0fs old (>2min) — kubelet may be dead, node is effectively offline", node.Name, age),
				})
			} else if age > 40 {
				result.Summary.StaleHeartbeat++
				entry.RiskLevel = "high"
				result.StaleHeartbeat = append(result.StaleHeartbeat, entry)
				result.Issues = append(result.Issues, NLIssue{
					Severity: "warning", Type: "stale-heartbeat",
					Resource: node.Name,
					Message:  fmt.Sprintf("Node %s heartbeat is %.0fs old (>40s) — kubelet heartbeat delayed, possible network or resource issue", node.Name, age),
				})
			} else if !entry.Ready {
				entry.RiskLevel = "high"
				result.Issues = append(result.Issues, NLIssue{
					Severity: "warning", Type: "not-ready",
					Resource: node.Name,
					Message:  fmt.Sprintf("Node %s is NotReady despite having a recent heartbeat — check node conditions", node.Name),
				})
			} else {
				entry.RiskLevel = "low"
			}
		} else {
			result.Summary.NoLease++
			entry.RiskLevel = "critical"
			entry.LastHeartbeatTime = time.Time{}
			entry.HeartbeatAgeSec = -1 // unknown
			result.NoLease = append(result.NoLease, entry)
			result.Issues = append(result.Issues, NLIssue{
				Severity: "critical", Type: "no-lease",
				Resource: node.Name,
				Message:  fmt.Sprintf("Node %s has no Lease object in kube-node-lease — kubelet heartbeat not registered, node may be offline or kubelet not running", node.Name),
			})
		}

		result.ByNode = append(result.ByNode, entry)
	}

	// Calculate averages
	if nodesWithHeartbeat > 0 {
		result.Summary.AvgHeartbeatAgeSec = totalAge / float64(nodesWithHeartbeat)
	}

	// Sort
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].HeartbeatAgeSec > result.ByNode[j].HeartbeatAgeSec
	})
	sort.Slice(result.StaleHeartbeat, func(i, j int) bool {
		return result.StaleHeartbeat[i].HeartbeatAgeSec > result.StaleHeartbeat[j].HeartbeatAgeSec
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return nlIssueRank(result.Issues[i].Severity) < nlIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = nlScore(result.Summary)
	result.Recommendations = nlGenRecs(result.Summary, result.StaleHeartbeat, result.NoLease)

	writeJSON(w, result)
}

// nlScore computes health score 0-100.
func nlScore(s NLSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	score := 100
	score -= s.NoLease * 15
	score -= s.VeryStaleHeartbeat * 12
	score -= (s.StaleHeartbeat - s.VeryStaleHeartbeat) * 6
	score -= s.NotReadyNodes * 8
	if score < 0 {
		score = 0
	}
	return score
}

// nlGenRecs produces actionable advice.
func nlGenRecs(s NLSummary, stale []NLEntry, noLease []NLEntry) []string {
	var recs []string

	if s.NoLease > 0 {
		top := ""
		if len(noLease) > 0 {
			top = fmt.Sprintf(" (e.g. %s)", noLease[0].NodeName)
		}
		recs = append(recs, fmt.Sprintf("%d node(s) have no Lease object%s — kubelet may not be running, check node health directly", s.NoLease, top))
	}
	if s.VeryStaleHeartbeat > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have heartbeat >2min old — these nodes are effectively offline, pods should be rescheduled", s.VeryStaleHeartbeat))
	}
	if s.StaleHeartbeat > 0 {
		top := ""
		if len(stale) > 0 {
			top = fmt.Sprintf(" (e.g. %s: %.0fs old)", stale[0].NodeName, stale[0].HeartbeatAgeSec)
		}
		recs = append(recs, fmt.Sprintf("%d node(s) have stale heartbeat (>40s)%s — check kubelet, network, and node resource usage", s.StaleHeartbeat, top))
	}
	if s.NotReadyNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are NotReady — investigate kubelet, container runtime, and node conditions", s.NotReadyNodes))
	}
	if s.AvgHeartbeatAgeSec > 20 {
		recs = append(recs, fmt.Sprintf("Average heartbeat age is %.0fs — cluster heartbeat latency is elevated, check network and control plane health", s.AvgHeartbeatAgeSec))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Node heartbeat health score is %d/100 — multiple nodes have heartbeat issues", s.HealthScore))
	}
	if s.NoLease == 0 && s.StaleHeartbeat == 0 && s.NotReadyNodes == 0 {
		recs = append(recs, "All nodes have fresh heartbeats — good cluster health posture")
	}

	return recs
}

func nlIssueRank(s string) int {
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
