package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TopoViolationResult detects topology spread constraint violations.
type TopoViolationResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         TopoViolationSummary `json:"summary"`
	ByWorkload      []TopoViolationEntry `json:"byWorkload"`
	Violations      []TopoViolationEntry `json:"violations"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type TopoViolationSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	WithTopoSpread int `json:"withTopologySpread"`
	MultiReplica   int `json:"multiReplica"`
	SingleNode     int `json:"allReplicasSameNode"`
	UnevenSpread   int `json:"unevenSpread"`
}

type TopoViolationEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Replicas     int32    `json:"replicas"`
	NodesUsed    int      `json:"nodesUsed"`
	HasTopoConst bool     `json:"hasTopologyConstraint"`
	MaxSkew      int32    `json:"maxSkew"`
	RiskLevel    string   `json:"riskLevel"`
	Issues       []string `json:"issues"`
}

// handleTopologySpreadViolation handles GET /api/scalability/topology-spread-violation
func (s *Server) handleTopologySpreadViolation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TopoViolationResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node distribution per deployment
	depNodes := make(map[string]map[string]int) // ns/name -> node -> count
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		depName := getOwnerDeployName(&pod)
		if depName == "" {
			continue
		}
		key := pod.Namespace + "/" + depName
		if depNodes[key] == nil {
			depNodes[key] = make(map[string]int)
		}
		depNodes[key][pod.Spec.NodeName]++
	}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		entry := TopoViolationEntry{
			Name: dep.Name, Namespace: dep.Namespace, Replicas: replicas,
		}

		// Check topology spread constraints
		constraints := dep.Spec.Template.Spec.TopologySpreadConstraints
		if len(constraints) > 0 {
			entry.HasTopoConst = true
			result.Summary.WithTopoSpread++
			entry.MaxSkew = constraints[0].MaxSkew
		}

		if replicas >= 2 {
			result.Summary.MultiReplica++
		} else {
			result.ByWorkload = append(result.ByWorkload, entry)
			continue
		}

		// Analyze actual distribution
		key := dep.Namespace + "/" + dep.Name
		nodeDist := depNodes[key]
		entry.NodesUsed = len(nodeDist)

		var issues []string
		if entry.NodesUsed <= 1 && replicas >= 2 {
			result.Summary.SingleNode++
			issues = append(issues, fmt.Sprintf("%d replicas on 1 node", replicas))
		} else if entry.NodesUsed > 0 {
			// Check skew
			max, min := 0, int(^uint(0)>>1)
			for _, count := range nodeDist {
				if count > max {
					max = count
				}
				if count < min {
					min = count
				}
			}
			skew := max - min
			if skew > 1 {
				result.Summary.UnevenSpread++
				issues = append(issues, fmt.Sprintf("uneven spread: max %d min %d (skew %d)", max, min, skew))
			}
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 2:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.Violations = append(result.Violations, entry)
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.MultiReplica > 0 {
		wellSpread := result.Summary.MultiReplica - result.Summary.SingleNode - result.Summary.UnevenSpread
		result.HealthScore = wellSpread * 100 / result.Summary.MultiReplica
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("拓扑扩展审计: %d 部署, %d 多副本, %d 同节点, %d 不均匀, %d 有约束",
			result.Summary.TotalWorkloads, result.Summary.MultiReplica,
			result.Summary.SingleNode, result.Summary.UnevenSpread,
			result.Summary.WithTopoSpread),
	}
	if result.Summary.SingleNode > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个多副本部署全部在同一节点", result.Summary.SingleNode))
	}
	writeJSON(w, result)
}
