package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeDecommResult analyzes node decommissioning readiness and lifecycle rotation.
type NodeDecommResult struct {
	ScannedAt          time.Time         `json:"scannedAt"`
	Summary            NodeDecommSummary `json:"summary"`
	RotationCandidates []NodeRotation    `json:"rotationCandidates"`
	HealthScore        int               `json:"healthScore"`
	Grade              string            `json:"grade"`
	Recommendations    []string          `json:"recommendations"`
}

type NodeDecommSummary struct {
	TotalNodes      int     `json:"totalNodes"`
	OldNodes        int     `json:"oldNodes"`
	NotReadyNodes   int     `json:"notReadyNodes"`
	AvgAge          string  `json:"avgAge"`
	RotationUrgency string  `json:"rotationUrgency"`
	PodsPerNode     float64 `json:"podsPerNode"`
}

type NodeRotation struct {
	Name     string `json:"name"`
	AgeDays  int    `json:"ageDays"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
}

// handleNodeDecomm analyzes node decommissioning readiness.
// GET /api/scalability/node-decomm
func (s *Server) handleNodeDecomm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeDecommResult{ScannedAt: time.Now()}
	now := time.Now()

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	totalAge := 0
	notReady := 0
	oldNodes := 0
	podsPerNode := 0.0

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		ageDays := int(now.Sub(node.CreationTimestamp.Time).Hours() / 24)
		totalAge += ageDays

		status := "Ready"
		isReady := true
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status != "True" {
				status = "NotReady"
				isReady = false
				notReady++
			}
		}

		// Count pods on this node
		nodePods := 0
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == node.Name {
				nodePods++
			}
		}
		podsPerNode += float64(nodePods)

		// Rotation candidates
		reason := ""
		severity := "low"
		if !isReady {
			reason = "Node not Ready — cordon and drain"
			severity = "critical"
		} else if ageDays > 730 {
			reason = fmt.Sprintf("Node %d days old (>2yr) — rotate for security patches", ageDays)
			severity = "high"
			oldNodes++
		} else if ageDays > 365 {
			reason = fmt.Sprintf("Node %d days old (>1yr) — plan rotation", ageDays)
			severity = "medium"
			oldNodes++
		}

		if reason != "" {
			result.RotationCandidates = append(result.RotationCandidates, NodeRotation{
				Name: node.Name, AgeDays: ageDays, Status: status,
				Reason: reason, Severity: severity,
			})
		}
	}

	result.Summary.NotReadyNodes = notReady
	result.Summary.OldNodes = oldNodes
	if result.Summary.TotalNodes > 0 {
		avgAge := totalAge / result.Summary.TotalNodes
		result.Summary.AvgAge = fmt.Sprintf("%dd", avgAge)
		result.Summary.PodsPerNode = podsPerNode / float64(result.Summary.TotalNodes)
	}

	urgency := "low"
	if notReady > 0 || oldNodes > result.Summary.TotalNodes/2 {
		urgency = "high"
	} else if oldNodes > 0 {
		urgency = "medium"
	}
	result.Summary.RotationUrgency = urgency

	// Score
	score := 100
	score -= notReady * 30
	score -= oldNodes * 10
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.RotationCandidates, func(i, j int) bool {
		return result.RotationCandidates[i].Severity > result.RotationCandidates[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Node decommissioning: %d/100 (grade %s) — %d nodes, %d old, urgency: %s", result.HealthScore, result.Grade, result.Summary.TotalNodes, oldNodes, urgency))
	if notReady > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes not Ready — drain and replace immediately", notReady))
	}
	if oldNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes older than 1 year — plan rotation for security patches", oldNodes))
	}
	if len(recs) == 1 {
		recs = append(recs, "Node lifecycle is healthy — no rotation needed")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
