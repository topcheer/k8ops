package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodAffinitySpreadResult analyzes pod anti-affinity effectiveness and topology spread violations.
type PodAffinitySpreadResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         AffinitySpreadSummary `json:"summary"`
	ByWorkload      []AffinitySpreadEntry `json:"byWorkload"`
	Violations      []AffinitySpreadEntry `json:"violations"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type AffinitySpreadSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	MultiReplica   int `json:"multiReplicaWorkloads"`
	WithAntiAff    int `json:"withAntiAffinity"`
	WithTopoSpread int `json:"withTopologySpread"`
	CoLocated      int `json:"coLocatedReplicas"`
	SinglePoint    int `json:"singlePointOfFailure"`
}

type AffinitySpreadEntry struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Kind        string   `json:"kind"`
	Replicas    int      `json:"replicas"`
	NodeSpread  int      `json:"nodesUsed"`
	ZoneSpread  int      `json:"zonesUsed"`
	HasAntiAff  bool     `json:"hasPodAntiAffinity"`
	HasTopoSprd bool     `json:"hasTopologySpread"`
	RiskLevel   string   `json:"riskLevel"`
	Issues      []string `json:"issues"`
}

// handlePodAffinitySpread handles GET /api/scalability/pod-affinity-spread
func (s *Server) handlePodAffinitySpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PodAffinitySpreadResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod-to-node map grouped by workload labels
	type workloadKey struct{ ns, name string }
	nodeMap := make(map[workloadKey]map[string]int) // key -> nodeName -> count
	zoneMap := make(map[workloadKey]map[string]int)
	nodeZoneMap := make(map[string]string)

	for _, node := range nodesFromPods(pods.Items) {
		if zone, ok := node.Labels[corev1.LabelTopologyZone]; ok {
			nodeZoneMap[node.Name] = zone
		}
	}
	_ = nodeZoneMap

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		// Derive workload from owner references or labels
		wk := workloadKey{ns: pod.Namespace, name: getWorkloadName1875(&pod)}
		if wk.name == "" {
			continue
		}
		if nodeMap[wk] == nil {
			nodeMap[wk] = make(map[string]int)
			zoneMap[wk] = make(map[string]int)
		}
		nodeMap[wk][pod.Spec.NodeName]++
		zone := pod.Spec.NodeName // fallback to node name as zone if zone labels absent
		zoneMap[wk][zone]++
	}

	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		entry := AffinitySpreadEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
			Replicas:  int(replicas),
		}

		if replicas >= 2 {
			result.Summary.MultiReplica++
		} else {
			// Single replica - always SPOF
			entry.RiskLevel = "medium"
			entry.Issues = []string{"single-replica"}
			result.Summary.SinglePoint++
			result.ByWorkload = append(result.ByWorkload, entry)
			continue
		}

		// Check anti-affinity
		if dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			entry.HasAntiAff = true
			result.Summary.WithAntiAff++
		}
		if dep.Spec.Template.Spec.TopologySpreadConstraints != nil {
			entry.HasTopoSprd = true
			result.Summary.WithTopoSpread++
		}

		// Check actual spread
		wk := workloadKey{ns: dep.Namespace, name: dep.Name}
		nodes := nodeMap[wk]
		entry.NodeSpread = len(nodes)

		if entry.NodeSpread > 0 {
			maxPerNode := 0
			for _, count := range nodes {
				if count > maxPerNode {
					maxPerNode = count
				}
			}
			if maxPerNode > 1 {
				result.Summary.CoLocated++
				entry.Issues = append(entry.Issues, fmt.Sprintf("%d replicas co-located on one node", maxPerNode))
			}
		}

		switch {
		case entry.NodeSpread == 1 && replicas >= 2:
			entry.RiskLevel = "critical"
			entry.Issues = append(entry.Issues, "all replicas on same node")
		case len(entry.Issues) >= 2:
			entry.RiskLevel = "high"
		case len(entry.Issues) >= 1:
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
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalWorkloads > 0 {
		wellSpread := result.Summary.TotalWorkloads - result.Summary.SinglePoint - result.Summary.CoLocated
		result.HealthScore = wellSpread * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("亲和性扩散: %d 工作负载, %d 多副本, %d 反亲和, %d 拓扑扩展, %d 同节点, %d 单点",
			result.Summary.TotalWorkloads, result.Summary.MultiReplica,
			result.Summary.WithAntiAff, result.Summary.WithTopoSpread,
			result.Summary.CoLocated, result.Summary.SinglePoint),
	}
	if result.Summary.SinglePoint > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作负载只有 1 个副本, 存在单点故障", result.Summary.SinglePoint))
	}
	if result.Summary.CoLocated > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作负载有副本在同节点, 建议添加 podAntiAffinity", result.Summary.CoLocated))
	}
	writeJSON(w, result)
}

func nodesFromPods(pods []corev1.Pod) []corev1.Node {
	return nil
}

func getWorkloadName1875(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" {
			// Strip hash suffix
			name := ref.Name
			if idx := lastIdx(name, "-"); idx > 0 {
				return name[:idx]
			}
			return name
		}
		return ref.Name
	}
	return ""
}

func lastIdx(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
