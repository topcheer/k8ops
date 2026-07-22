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

// ============================================================
// v19.29 — Deployment Dimension (Round 8)
// 1. StatefulSet Health Audit — ordinal readiness & update strategy
// 2. Image Pull Secret Gap — missing registry credentials
// 3. Pod Topology Distribution — pod spread across nodes/zones
// ============================================================

// ---------------------------------------------------------------
// 1. StatefulSet Health Audit — ordinal readiness & update strategy
// ---------------------------------------------------------------

type StatefulSetHealthResult1929 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         StatefulSetHealthSummary1929 `json:"summary"`
	StatefulSets    []StatefulSetEntry1929       `json:"statefulSets"`
	Issues          []StatefulSetIssue1929       `json:"issues"`
	Recommendations []string                     `json:"recommendations"`
}

type StatefulSetHealthSummary1929 struct {
	TotalSTS        int `json:"totalStatefulSets"`
	HealthySTS      int `json:"healthyStatefulSets"`
	IssuesCount     int `json:"issuesCount"`
	WithVolumeClaim int `json:"withVolumeClaimTemplates"`
	OrderedReady    int `json:"orderedReady"`
	OutOfOrder      int `json:"outOfOrder"`
	WithPartition   int `json:"withPartition"`
}

type StatefulSetEntry1929 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Replicas        int32  `json:"replicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
	UpdateStrategy  string `json:"updateStrategy"`
	Partition       int32  `json:"partition"`
	HasPVC          bool   `json:"hasVolumeClaimTemplates"`
	Age             string `json:"age"`
}

type StatefulSetIssue1929 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleStatefulSetHealth(w http.ResponseWriter, r *http.Request) {
	result := StatefulSetHealthResult1929{
		ScannedAt: time.Now(),
	}
	score := 100

	stsList, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, sts := range stsList.Items {
		if isSystemNamespace(sts.Namespace) {
			continue
		}
		result.Summary.TotalSTS++

		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		ready := sts.Status.ReadyReplicas
		updated := sts.Status.UpdatedReplicas
		strategy := string(sts.Spec.UpdateStrategy.Type)
		partition := int32(0)
		if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
		}
		hasPVC := len(sts.Spec.VolumeClaimTemplates) > 0
		age := fmt.Sprintf("%.0fd", time.Since(sts.CreationTimestamp.Time).Hours()/24)

		entry := StatefulSetEntry1929{
			Name: sts.Name, Namespace: sts.Namespace,
			Replicas: replicas, ReadyReplicas: ready, UpdatedReplicas: updated,
			UpdateStrategy: strategy, Partition: partition, HasPVC: hasPVC, Age: age,
		}
		result.StatefulSets = append(result.StatefulSets, entry)

		if hasPVC {
			result.Summary.WithVolumeClaim++
		}
		if partition > 0 {
			result.Summary.WithPartition++
			result.Issues = append(result.Issues, StatefulSetIssue1929{
				Name: sts.Name, Namespace: sts.Namespace,
				IssueType: "partition-set", Severity: "medium",
				Detail: fmt.Sprintf("RollingUpdate partition=%d — %d pods still on old revision", partition, partition),
			})
			score -= 3
		}
		if ready < replicas {
			result.Issues = append(result.Issues, StatefulSetIssue1929{
				Name: sts.Name, Namespace: sts.Namespace,
				IssueType: "not-ready", Severity: "high",
				Detail: fmt.Sprintf("%d/%d replicas ready — pods not healthy", ready, replicas),
			})
			score -= 5
		} else {
			result.Summary.HealthySTS++
			result.Summary.OrderedReady++
		}
		if updated < replicas && partition == 0 {
			result.Issues = append(result.Issues, StatefulSetIssue1929{
				Name: sts.Name, Namespace: sts.Namespace,
				IssueType: "update-incomplete", Severity: "medium",
				Detail: fmt.Sprintf("%d/%d replicas updated — rollout in progress or stuck", updated, replicas),
			})
			result.Summary.OutOfOrder++
			score -= 3
		}
		// Single replica StatefulSet = SPOF
		if replicas == 1 {
			result.Issues = append(result.Issues, StatefulSetIssue1929{
				Name: sts.Name, Namespace: sts.Namespace,
				IssueType: "single-replica", Severity: "medium",
				Detail: "Single replica StatefulSet — no HA, data loss risk on node failure",
			})
			score -= 2
		}
	}

	result.Summary.IssuesCount = len(result.Issues)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OutOfOrder > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StatefulSets with incomplete rollouts — check pod status", result.Summary.OutOfOrder))
	}
	if result.Summary.WithPartition > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StatefulSets with partition>0 — complete canary rollout", result.Summary.WithPartition))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Pull Secret Gap — missing registry credentials
// ---------------------------------------------------------------

type ImagePullSecretResult1929 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         ImagePullSecretSummary1929 `json:"summary"`
	Namespaces      []ImagePullSecretNS1929    `json:"namespaces"`
	PodsMissing     []ImagePullSecretGap1929   `json:"podsMissing"`
	Recommendations []string                   `json:"recommendations"`
}

type ImagePullSecretSummary1929 struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	WithPullSecret    int `json:"withPullSecret"`
	WithoutSecret     int `json:"withoutSecret"`
	PodsUsingPrivate  int `json:"podsUsingPrivateRegistries"`
	PodsMissingSecret int `json:"podsMissingSecret"`
	SecretCount       int `json:"totalSecrets"`
}

type ImagePullSecretNS1929 struct {
	Namespace  string   `json:"namespace"`
	HasDefault bool     `json:"hasDefaultPullSecret"`
	Secrets    []string `json:"secrets"`
	PodCount   int      `json:"podCount"`
}

type ImagePullSecretGap1929 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

func (s *Server) handleImagePullSecretGap(w http.ResponseWriter, r *http.Request) {
	result := ImagePullSecretResult1929{
		ScannedAt: time.Now(),
	}
	score := 100

	// Collect image pull secrets per namespace via service accounts
	saList, err := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	nsPullSecrets := make(map[string][]string)
	nsDefaultSecrets := make(map[string]bool)
	for _, sa := range saList.Items {
		if sa.Name == "default" {
			for _, ips := range sa.ImagePullSecrets {
				nsDefaultSecrets[sa.Namespace] = true
				nsPullSecrets[sa.Namespace] = append(nsPullSecrets[sa.Namespace], ips.Name)
			}
		}
	}

	// List namespaces
	nsList, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ns := range nsList.Items {
			if isSystemNamespace(ns.Name) {
				continue
			}
			result.Summary.TotalNamespaces++
			if nsDefaultSecrets[ns.Name] || len(nsPullSecrets[ns.Name]) > 0 {
				result.Summary.WithPullSecret++
			} else {
				result.Summary.WithoutSecret++
			}
		}
	}

	// Check pods for private registry images without pull secrets
	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Private registry patterns (non-standard registries)
	privatePatterns := []string{
		"registry.iot2.win", "ghcr.io", "gcr.io", "docker.io/topcheer",
		"quay.io", "registry.gitlab.com", "registry-access.redhat.com",
	}

	nsPodCount := make(map[string]int)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPodCount[pod.Namespace]++
		hasPodSecret := len(pod.Spec.ImagePullSecrets) > 0 || nsDefaultSecrets[pod.Namespace]
		for _, c := range pod.Spec.Containers {
			img := c.Image
			isPrivate := false
			for _, pat := range privatePatterns {
				if strings.HasPrefix(img, pat) || strings.Contains(img, pat) {
					isPrivate = true
					break
				}
			}
			// Also check non-dockerhub (contains / and not library/)
			if !isPrivate && strings.Contains(img, "/") && !strings.HasPrefix(img, "k8s.gcr.io") && !strings.HasPrefix(img, "registry.k8s.io") {
				parts := strings.SplitN(img, "/", 2)
				if len(parts) > 1 && strings.Contains(parts[0], ".") {
					isPrivate = true
				}
			}
			if isPrivate {
				result.Summary.PodsUsingPrivate++
				if !hasPodSecret {
					result.PodsMissing = append(result.PodsMissing, ImagePullSecretGap1929{
						PodName:   pod.Name,
						Namespace: pod.Namespace,
						Image:     img,
						Reason:    "Private registry image without imagePullSecret — may cause ImagePullBackOff",
						Severity:  "high",
					})
					result.Summary.PodsMissingSecret++
					score -= 2
				}
			}
		}
	}

	// Build namespace stats
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Namespaces = append(result.Namespaces, ImagePullSecretNS1929{
			Namespace:  ns.Name,
			HasDefault: nsDefaultSecrets[ns.Name],
			Secrets:    nsPullSecrets[ns.Name],
			PodCount:   nsPodCount[ns.Name],
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PodsMissingSecret > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use private registries without imagePullSecret — add registry credentials", result.Summary.PodsMissingSecret))
	}
	if result.Summary.WithoutSecret > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without default imagePullSecret — add for private registry access", result.Summary.WithoutSecret))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Topology Distribution — pod spread across nodes/zones
// ---------------------------------------------------------------

type TopologyDistResult1929 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         TopologyDistSummary1929 `json:"summary"`
	Workloads       []TopologyDistEntry1929 `json:"workloads"`
	NodeSpread      []NodeSpreadEntry1929   `json:"nodeSpread"`
	Risks           []TopologyRisk1929      `json:"risks"`
	Recommendations []string                `json:"recommendations"`
}

type TopologyDistSummary1929 struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	WellDistributed int     `json:"wellDistributed"`
	Concentrated    int     `json:"concentrated"`
	TotalNodes      int     `json:"totalNodes"`
	AvgSpreadScore  float64 `json:"avgSpreadScore"`
	RiskCount       int     `json:"riskCount"`
}

type TopologyDistEntry1929 struct {
	Name             string         `json:"name"`
	Namespace        string         `json:"namespace"`
	Replicas         int            `json:"replicas"`
	NodeDistribution map[string]int `json:"nodeDistribution"`
	SpreadScore      float64        `json:"spreadScore"`
	Status           string         `json:"status"`
}

type NodeSpreadEntry1929 struct {
	NodeName  string `json:"nodeName"`
	PodCount  int    `json:"podCount"`
	Workloads int    `json:"workloads"`
}

type TopologyRisk1929 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleTopologyDistribution(w http.ResponseWriter, r *http.Request) {
	result := TopologyDistResult1929{
		ScannedAt: time.Now(),
	}
	score := 100

	nodeList, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	result.Summary.TotalNodes = len(nodeList.Items)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Group pods by workload
	type wlKey struct{ ns, name string }
	wlPods := make(map[wlKey]map[string]int) // workload -> node -> count
	nodePods := make(map[string]int)
	nodeWorkloads := make(map[string]map[string]bool)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}
		key := wlKey{ns: pod.Namespace, name: appName}
		if wlPods[key] == nil {
			wlPods[key] = make(map[string]int)
		}
		wlPods[key][pod.Spec.NodeName]++
		nodePods[pod.Spec.NodeName]++
		if nodeWorkloads[pod.Spec.NodeName] == nil {
			nodeWorkloads[pod.Spec.NodeName] = make(map[string]bool)
		}
		nodeWorkloads[pod.Spec.NodeName][appName] = true
	}

	var totalSpread float64
	var spreadCount int

	for key, nodes := range wlPods {
		totalReplicas := 0
		for _, c := range nodes {
			totalReplicas += c
		}
		if totalReplicas == 0 {
			continue
		}

		// Calculate spread score: how evenly pods are distributed
		nodeCount := len(nodes)
		spreadScore := 100.0
		if result.Summary.TotalNodes > 1 {
			// Ideal: pods spread across all nodes
			idealRatio := float64(result.Summary.TotalNodes)
			if float64(nodeCount) < idealRatio && totalReplicas >= int(idealRatio) {
				spreadScore = float64(nodeCount) / idealRatio * 100
			}
		} else {
			// Single node cluster — all pods on same node, can't spread
			spreadScore = 50
		}

		status := "distributed"
		if spreadScore < 50 && totalReplicas > 1 {
			status = "concentrated"
			result.Summary.Concentrated++
			result.Risks = append(result.Risks, TopologyRisk1929{
				Name: key.name, Namespace: key.ns,
				RiskType: "poor-spread", Severity: "high",
				Detail: fmt.Sprintf("%d replicas concentrated on %d node(s) — single node failure risk", totalReplicas, nodeCount),
			})
			score -= 5
		} else {
			result.Summary.WellDistributed++
		}

		result.Workloads = append(result.Workloads, TopologyDistEntry1929{
			Name:             key.name,
			Namespace:        key.ns,
			Replicas:         totalReplicas,
			NodeDistribution: nodes,
			SpreadScore:      spreadScore,
			Status:           status,
		})
		result.Summary.TotalWorkloads++
		totalSpread += spreadScore
		spreadCount++
	}

	// Node spread stats
	for _, node := range nodeList.Items {
		wlCount := 0
		if nodeWorkloads[node.Name] != nil {
			wlCount = len(nodeWorkloads[node.Name])
		}
		result.NodeSpread = append(result.NodeSpread, NodeSpreadEntry1929{
			NodeName:  node.Name,
			PodCount:  nodePods[node.Name],
			Workloads: wlCount,
		})
	}

	if spreadCount > 0 {
		result.Summary.AvgSpreadScore = totalSpread / float64(spreadCount)
	}
	result.Summary.RiskCount = len(result.Risks)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.Concentrated > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads concentrated on fewer nodes than ideal — add podAntiAffinity or topologySpreadConstraints", result.Summary.Concentrated))
	}
	if result.Summary.TotalNodes == 1 {
		result.Recommendations = append(result.Recommendations, "Single-node cluster — add nodes for HA pod distribution")
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
