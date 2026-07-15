package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReplicaDistResult is the workload replica distribution & anti-affinity coverage audit.
type ReplicaDistResult struct {
	Timestamp       time.Time          `json:"timestamp"`
	Score           int                `json:"score"`
	Status          string             `json:"status"`
	Summary         ReplicaDistSummary `json:"summary"`
	Workloads       []DistWorkload     `json:"workloads"`
	NodeSpread      []NodeSpreadEntry  `json:"nodeSpread"`
	ZoneSpread      []ZoneSpreadEntry  `json:"zoneSpread"`
	AtRiskWorkloads []DistRiskEntry    `json:"atRiskWorkloads"`
	Issues          []DistIssue        `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

// ReplicaDistSummary holds aggregate distribution metrics.
type ReplicaDistSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	MultiReplica    int `json:"multiReplica"`
	SingleReplica   int `json:"singleReplica"`
	GoodSpread      int `json:"goodSpread"`
	PoorSpread      int `json:"poorSpread"`
	HasAntiAffinity int `json:"hasAntiAffinity"`
	NoAntiAffinity  int `json:"noAntiAffinity"`
	AtRiskCount     int `json:"atRiskCount"`
}

// DistWorkload shows replica distribution for one workload.
type DistWorkload struct {
	Namespace       string         `json:"namespace"`
	Name            string         `json:"name"`
	Kind            string         `json:"kind"`
	Replicas        int32          `json:"replicas"`
	ReadyReplicas   int32          `json:"readyReplicas"`
	Nodes           map[string]int `json:"nodes"`
	NodeCount       int            `json:"nodeCount"`
	Zones           map[string]int `json:"zones"`
	ZoneCount       int            `json:"zoneCount"`
	HasAntiAffinity bool           `json:"hasAntiAffinity"`
	SpreadScore     int            `json:"spreadScore"`
	RiskLevel       string         `json:"riskLevel"`
}

// NodeSpreadEntry shows pod count per node.
type NodeSpreadEntry struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

// ZoneSpreadEntry shows pod count per zone.
type ZoneSpreadEntry struct {
	Zone     string `json:"zone"`
	PodCount int    `json:"podCount"`
}

// DistRiskEntry identifies a workload with poor distribution.
type DistRiskEntry struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Replicas  int32  `json:"replicas"`
	NodeCount int    `json:"nodeCount"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
}

// DistIssue is a distribution issue.
type DistIssue struct {
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	Message   string `json:"message"`
}

func (s *Server) handleReplicaDistribution(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deploys, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list deployments: %v", err))
		return
	}

	stss, err := rc.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		stss = &appsv1.StatefulSetList{}
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	result := analyzeReplicaDistribution(deploys.Items, stss.Items, pods.Items)
	writeJSON(w, result)
}

func analyzeReplicaDistribution(deploys []appsv1.Deployment, stss []appsv1.StatefulSet, pods []corev1.Pod) ReplicaDistResult {
	now := time.Now()

	// Build pod index by owner
	podOwner := make(map[string][]corev1.Pod) // "ns/name" -> pods
	for _, pod := range pods {
		for _, ref := range pod.OwnerReferences {
			key := pod.Namespace + "/" + ref.Name
			podOwner[key] = append(podOwner[key], pod)
		}
	}

	// Node zone lookup
	nodeZones := make(map[string]string)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			nodeZones[pod.Spec.NodeName] = ""
		}
	}

	summary := ReplicaDistSummary{}
	var workloads []DistWorkload
	var atRisk []DistRiskEntry
	var issues []DistIssue
	nodePodCount := make(map[string]int)
	zonePodCount := make(map[string]int)

	analyzeWorkload := func(ns, name, kind string, replicas int32, hasAntiAffinity bool, podList []corev1.Pod) {
		if ns == "kube-system" {
			return
		}
		summary.TotalWorkloads++

		if replicas <= 1 {
			summary.SingleReplica++
			return
		}
		summary.MultiReplica++

		nodeMap := make(map[string]int)
		zoneMap := make(map[string]int)
		for _, pod := range podList {
			if pod.Spec.NodeName == "" {
				continue
			}
			nodeMap[pod.Spec.NodeName]++
			nodePodCount[pod.Spec.NodeName]++
			zone := pod.Spec.NodeName // fallback to node name
			zoneMap[zone]++
			zonePodCount[zone]++
		}

		nodeCount := len(nodeMap)
		zoneCount := len(zoneMap)

		// Spread score: how well distributed across nodes
		spreadScore := 0
		if replicas > 0 {
			ratio := float64(nodeCount) / float64(replicas)
			spreadScore = int(ratio * 100)
			if spreadScore > 100 {
				spreadScore = 100
			}
		}

		riskLevel := "low"
		if nodeCount == 1 && replicas > 1 {
			riskLevel = "critical"
			summary.PoorSpread++
			summary.AtRiskCount++
			atRisk = append(atRisk, DistRiskEntry{
				Namespace: ns, Name: name, Kind: kind,
				Replicas: replicas, NodeCount: nodeCount,
				RiskType: "AllPodsOnSingleNode", Severity: "critical",
			})
			issues = append(issues, DistIssue{
				Type: "Single node concentration", Severity: "critical",
				Namespace: ns, Workload: name,
				Message: fmt.Sprintf("%s/%s has %d replicas all on one node; single node failure will cause full outage", kind, name, replicas),
			})
		} else if nodeCount < int(replicas)/2+1 && replicas >= 3 {
			riskLevel = "high"
			summary.PoorSpread++
			summary.AtRiskCount++
			atRisk = append(atRisk, DistRiskEntry{
				Namespace: ns, Name: name, Kind: kind,
				Replicas: replicas, NodeCount: nodeCount,
				RiskType: "InsufficientSpread", Severity: "high",
			})
			issues = append(issues, DistIssue{
				Type: "Insufficient spread", Severity: "high",
				Namespace: ns, Workload: name,
				Message: fmt.Sprintf("%s/%s has %d replicas on only %d nodes; should spread across at least %d nodes", kind, name, replicas, nodeCount, int(replicas)/2+1),
			})
		} else {
			summary.GoodSpread++
		}

		if hasAntiAffinity {
			summary.HasAntiAffinity++
		} else {
			summary.NoAntiAffinity++
			if replicas >= 3 {
				issues = append(issues, DistIssue{
					Type: "MissingAntiAffinity", Severity: "medium",
					Namespace: ns, Workload: name,
					Message: fmt.Sprintf("%s/%s has %d replicas but no pod anti-affinity; replicas may concentrate on one node", kind, name, replicas),
				})
			}
		}

		workloads = append(workloads, DistWorkload{
			Namespace: ns, Name: name, Kind: kind,
			Replicas: replicas, Nodes: nodeMap, NodeCount: nodeCount,
			Zones: zoneMap, ZoneCount: zoneCount,
			HasAntiAffinity: hasAntiAffinity,
			SpreadScore:     spreadScore, RiskLevel: riskLevel,
		})
	}

	// Deployments
	for _, dep := range deploys {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		hasAA := hasPodAntiAffinity(dep.Spec.Template.Spec.Affinity)
		key := dep.Namespace + "/" + dep.Name
		analyzeWorkload(dep.Namespace, dep.Name, "Deployment", replicas, hasAA, podOwner[key])
	}

	// StatefulSets
	for _, sts := range stss {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		hasAA := hasPodAntiAffinity(sts.Spec.Template.Spec.Affinity)
		key := sts.Namespace + "/" + sts.Name
		analyzeWorkload(sts.Namespace, sts.Name, "StatefulSet", replicas, hasAA, podOwner[key])
	}

	// Build node/zone spread
	var nodeSpread []NodeSpreadEntry
	for n, c := range nodePodCount {
		nodeSpread = append(nodeSpread, NodeSpreadEntry{Node: n, PodCount: c})
	}
	sort.Slice(nodeSpread, func(i, j int) bool { return nodeSpread[i].PodCount > nodeSpread[j].PodCount })

	var zoneSpread []ZoneSpreadEntry
	for z, c := range zonePodCount {
		zoneSpread = append(zoneSpread, ZoneSpreadEntry{Zone: z, PodCount: c})
	}
	sort.Slice(zoneSpread, func(i, j int) bool { return zoneSpread[i].PodCount > zoneSpread[j].PodCount })

	// Sort workloads by risk
	sort.Slice(workloads, func(i, j int) bool {
		return workloads[i].SpreadScore < workloads[j].SpreadScore
	})

	// Score
	score := 100
	score -= summary.AtRiskCount * 10
	score -= summary.NoAntiAffinity * 2
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if summary.AtRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) at risk due to poor replica distribution; add pod anti-affinity rules", summary.AtRiskCount))
	}
	if summary.NoAntiAffinity > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) lack pod anti-affinity; add preferredDuringScheduling anti-affinity for HA", summary.NoAntiAffinity))
	}
	if summary.SingleReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) with single replica; no HA redundancy if node fails", summary.SingleReplica))
	}
	if len(recs) == 0 {
		recs = append(recs, "Workload replica distribution looks well-spread across nodes")
	}

	return ReplicaDistResult{
		Timestamp: now, Score: score, Status: status,
		Summary: summary, Workloads: workloads,
		NodeSpread: nodeSpread, ZoneSpread: zoneSpread,
		AtRiskWorkloads: atRisk, Issues: issues,
		Recommendations: recs,
	}
}

func hasPodAntiAffinity(affinity *corev1.Affinity) bool {
	if affinity == nil {
		return false
	}
	if affinity.PodAntiAffinity != nil {
		return len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 ||
			len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0
	}
	return false
}

// init registers the handler name to avoid unused import warnings.
var _ = strings.Contains
