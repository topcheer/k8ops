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

// NodeUpgradeResult is the node upgrade readiness & K8s version compatibility auditor.
type NodeUpgradeResult struct {
	ScannedAt        time.Time            `json:"scannedAt"`
	Summary          NodeUpgradeSummary   `json:"summary"`
	CurrentVersion   string               `json:"currentVersion"`
	TargetVersions   []string             `json:"targetVersions"`
	NodeGroups       []NodeUpgradeGroup   `json:"nodeGroups"`
	Blockers         []UpgradeBlocker     `json:"blockers,omitempty"`
	DeprecatedAPIs   []DeprecatedAPIUsage `json:"deprecatedAPIs,omitempty"`
	ImpactAssessment UpgradeImpact        `json:"impactAssessment"`
	Recommendations  []string             `json:"recommendations"`
	ReadinessScore   int                  `json:"readinessScore"`
}

// NodeUpgradeSummary aggregates upgrade readiness statistics.
type NodeUpgradeSummary struct {
	TotalNodes      int    `json:"totalNodes"`
	VersionSkew     bool   `json:"versionSkew"`     // nodes running different K8s versions
	MaxSkewVersions int    `json:"maxSkewVersions"` // max version difference
	NodesAtOldest   int    `json:"nodesAtOldest"`   // nodes at oldest version
	NodesAtLatest   int    `json:"nodesAtLatest"`   // nodes at latest version
	BlockerCount    int    `json:"blockerCount"`
	UpgradePath     string `json:"upgradePath"` // e.g. "1.28 -> 1.29"
	CanUpgrade      bool   `json:"canUpgrade"`
}

// NodeUpgradeGroup groups nodes by Kubernetes version.
type NodeUpgradeGroup struct {
	Version      string   `json:"version"`
	NodeCount    int      `json:"nodeCount"`
	NodeNames    []string `json:"nodeNames"`
	NeedsUpgrade bool     `json:"needsUpgrade"`
	Roles        []string `json:"roles"` // master, worker
}

// UpgradeBlocker describes a node upgrade blocking issue.
type UpgradeBlocker struct {
	Severity string `json:"severity"`
	Type     string `json:"type"` // version-skew, deprecated-api, pod-disruption, resource-pressure
	Detail   string `json:"detail"`
	NodeName string `json:"nodeName,omitempty"`
}

// DeprecatedAPIUsage identifies APIs deprecated in target K8s version.
type DeprecatedAPIUsage struct {
	API         string `json:"api"`
	Resource    string `json:"resource"`
	Count       int    `json:"count"`
	RemovedIn   string `json:"removedIn"`
	Replacement string `json:"replacement"`
}

// UpgradeImpact assesses the impact of upgrading.
type UpgradeImpact struct {
	AffectedWorkloads int    `json:"affectedWorkloads"`
	PodsToReschedule  int    `json:"podsToReschedule"`
	EstimatedDowntime string `json:"estimatedDowntime"`
	RiskLevel         string `json:"riskLevel"`
}

// handleNodeUpgrade audits node upgrade readiness and K8s version compatibility.
// GET /api/scalability/node-upgrade-audit
func (s *Server) handleNodeUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeUpgradeResult{ScannedAt: time.Now()}

	// 1. Collect nodes and group by version
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	versionMap := map[string]*NodeUpgradeGroup{}
	var allVersions []string

	for _, node := range nodes.Items {
		ver := node.Status.NodeInfo.KubeletVersion
		if ver == "" {
			ver = "unknown"
		}
		// Normalize version (strip v prefix)
		ver = strings.TrimPrefix(ver, "v")

		if versionMap[ver] == nil {
			versionMap[ver] = &NodeUpgradeGroup{
				Version: ver,
			}
			allVersions = append(allVersions, ver)
		}
		versionMap[ver].NodeCount++
		if len(versionMap[ver].NodeNames) < 20 {
			versionMap[ver].NodeNames = append(versionMap[ver].NodeNames, node.Name)
		}

		// Determine role
		isControlPlane := false
		for _, taint := range node.Spec.Taints {
			if strings.Contains(taint.Key, "control-plane") || strings.Contains(taint.Key, "master") {
				isControlPlane = true
			}
		}
		role := "worker"
		if isControlPlane {
			role = "control-plane"
		}
		if node.Labels["kubernetes.io/role"] == "master" {
			role = "control-plane"
		}
		versionMap[ver].Roles = appendUniqueStr(versionMap[ver].Roles, role)
	}

	// Sort versions
	sort.Strings(allVersions)
	if len(allVersions) > 0 {
		result.CurrentVersion = allVersions[len(allVersions)-1]
		result.Summary.UpgradePath = fmt.Sprintf("%s → next minor", result.CurrentVersion)

		// Suggest next versions
		for i := len(allVersions) - 1; i >= 0 && len(result.TargetVersions) < 3; i-- {
			result.TargetVersions = append(result.TargetVersions, allVersions[i])
		}
	}

	// Build node groups
	for _, ver := range allVersions {
		group := versionMap[ver]
		group.NeedsUpgrade = ver != result.CurrentVersion
		result.NodeGroups = append(result.NodeGroups, *group)
	}

	result.Summary.TotalNodes = len(nodes.Items)
	if versionMap[result.CurrentVersion] != nil {
		result.Summary.NodesAtLatest = versionMap[result.CurrentVersion].NodeCount
	}
	if len(allVersions) > 0 && versionMap[allVersions[0]] != nil {
		result.Summary.NodesAtOldest = versionMap[allVersions[0]].NodeCount
	}
	result.Summary.VersionSkew = len(allVersions) > 1
	result.Summary.MaxSkewVersions = len(allVersions)

	// 2. Check for upgrade blockers
	var blockers []UpgradeBlocker

	// Version skew
	if len(allVersions) > 2 {
		blockers = append(blockers, UpgradeBlocker{
			Severity: "high",
			Type:     "version-skew",
			Detail:   fmt.Sprintf("Cluster has %d different K8s versions (%s) — skew exceeds Kubernetes skew policy (max +2/-1)", len(allVersions), strings.Join(allVersions, ", ")),
		})
	} else if len(allVersions) == 2 {
		blockers = append(blockers, UpgradeBlocker{
			Severity: "medium",
			Type:     "version-skew",
			Detail:   fmt.Sprintf("Minor version skew detected (%s) — plan upgrade to align all nodes", strings.Join(allVersions, ", ")),
		})
	}

	// Node pressure
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if pods != nil {
		nodePodCount := map[string]int{}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
				nodePodCount[pod.Spec.NodeName]++
			}
		}
		for _, node := range nodes.Items {
			_ = nodePodCount[node.Name]
			// Check if node is under pressure
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
					blockers = append(blockers, UpgradeBlocker{
						Severity: "high",
						Type:     "resource-pressure",
						Detail:   fmt.Sprintf("Node %s has MemoryPressure — draining during upgrade may cascade failures", node.Name),
						NodeName: node.Name,
					})
				}
				if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
					blockers = append(blockers, UpgradeBlocker{
						Severity: "warning",
						Type:     "resource-pressure",
						Detail:   fmt.Sprintf("Node %s has DiskPressure — resolve before upgrading", node.Name),
						NodeName: node.Name,
					})
				}
			}
		}
	}

	// PDB coverage for upgrade
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	podsWithPDB := 0
	podsWithoutPDB := 0
	if pods != nil {
		pdbSelectors := []string{}
		for _, pdb := range pdbs.Items {
			if pdb.Spec.Selector != nil {
				for k, v := range pdb.Spec.Selector.MatchLabels {
					pdbSelectors = append(pdbSelectors, fmt.Sprintf("%s=%s", k, v))
				}
			}
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
				continue
			}
			hasPDB := false
			for _, sel := range pdbSelectors {
				parts := strings.SplitN(sel, "=", 2)
				if len(parts) == 2 && pod.Labels[parts[0]] == parts[1] {
					hasPDB = true
					break
				}
			}
			if hasPDB {
				podsWithPDB++
			} else {
				podsWithoutPDB++
			}
		}
	}
	if podsWithoutPDB > podsWithPDB && podsWithoutPDB > 10 {
		blockers = append(blockers, UpgradeBlocker{
			Severity: "medium",
			Type:     "pod-disruption",
			Detail:   fmt.Sprintf("%d running pods without PDB — upgrade drain may cause unexpected disruption", podsWithoutPDB),
		})
	}

	result.Blockers = blockers
	result.Summary.BlockerCount = len(blockers)
	result.Summary.CanUpgrade = len(blockers) == 0 || allLowSeverity(blockers)

	// 3. Deprecated API detection (check CRDs and workloads for removed APIs)
	var deprecatedAPIs []DeprecatedAPIUsage

	// Check for deprecated API usage patterns in deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if deployments != nil {
		for _, d := range deployments.Items {
			for range d.Spec.Template.Spec.Containers {
				// Check for deprecated volume types
				for _, v := range d.Spec.Template.Spec.Volumes {
					if v.ScaleIO != nil {
						deprecatedAPIs = append(deprecatedAPIs, DeprecatedAPIUsage{
							API: "volumes.scaleio", Resource: d.Name,
							Count: 1, RemovedIn: "1.22", Replacement: "use CSI driver",
						})
					}
					if v.StorageOS != nil {
						deprecatedAPIs = append(deprecatedAPIs, DeprecatedAPIUsage{
							API: "volumes.storageos", Resource: d.Name,
							Count: 1, RemovedIn: "1.22", Replacement: "use CSI driver",
						})
					}
				}
			}
		}
	}

	result.DeprecatedAPIs = deprecatedAPIs

	// 4. Impact assessment
	runningPods := 0
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
				runningPods++
			}
		}
	}
	// Estimate: upgrading oldest nodes requires draining them
	oldestNodeCount := result.Summary.NodesAtOldest
	affectedPods := runningPods
	if result.Summary.NodesAtLatest > 0 {
		affectedPods = runningPods * oldestNodeCount / result.Summary.TotalNodes
	}
	result.ImpactAssessment = UpgradeImpact{
		AffectedWorkloads: len(deployments.Items),
		PodsToReschedule:  affectedPods,
		EstimatedDowntime: "rolling (0-5 min per node with proper PDB)",
	}
	if len(blockers) > 0 {
		hasHigh := false
		for _, b := range blockers {
			if b.Severity == "high" {
				hasHigh = true
			}
		}
		if hasHigh {
			result.ImpactAssessment.RiskLevel = "high"
		} else {
			result.ImpactAssessment.RiskLevel = "medium"
		}
	} else {
		result.ImpactAssessment.RiskLevel = "low"
	}

	// 5. Readiness score
	score := 100
	score -= len(blockers) * 10
	if result.Summary.VersionSkew {
		score -= 15
	}
	if len(deprecatedAPIs) > 0 {
		score -= 5 * len(deprecatedAPIs)
	}
	if score < 0 {
		score = 0
	}
	result.ReadinessScore = score

	// 6. Recommendations
	result.Recommendations = generateNodeUpgradeRecs(result)

	writeJSON(w, result)
}

// allLowSeverity checks if all blockers are low severity.
func allLowSeverity(blockers []UpgradeBlocker) bool {
	for _, b := range blockers {
		if b.Severity == "high" || b.Severity == "critical" {
			return false
		}
	}
	return true
}

// appendUniqueStr appends a unique string to a slice.
func appendUniqueStr(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// generateNodeUpgradeRecs produces recommendations.
func generateNodeUpgradeRecs(result NodeUpgradeResult) []string {
	var recs []string

	if result.Summary.VersionSkew {
		recs = append(recs, fmt.Sprintf("Version skew detected (%d versions: %s) — align all nodes to %s before next upgrade",
			result.Summary.MaxSkewVersions, result.CurrentVersion, result.CurrentVersion))
	}

	if len(result.Blockers) > 0 {
		highCount := 0
		for _, b := range result.Blockers {
			if b.Severity == "high" {
				highCount++
			}
		}
		recs = append(recs, fmt.Sprintf("%d upgrade blocker(s) (%d high severity) — resolve before proceeding", len(result.Blockers), highCount))
	}

	if len(result.DeprecatedAPIs) > 0 {
		recs = append(recs, fmt.Sprintf("%d deprecated API(s) in use — migrate before upgrading (removed in target version)", len(result.DeprecatedAPIs)))
	}

	if result.ReadinessScore >= 80 {
		recs = append(recs, fmt.Sprintf("Upgrade readiness: %d/100 — cluster is ready for upgrade", result.ReadinessScore))
	} else if result.ReadinessScore < 50 {
		recs = append(recs, fmt.Sprintf("Upgrade readiness: %d/100 — significant preparation needed before upgrade", result.ReadinessScore))
	}

	if result.ImpactAssessment.RiskLevel == "high" {
		recs = append(recs, "Upgrade risk is HIGH — consider canary upgrade or staging validation first")
	}

	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("Cluster at %s, all nodes aligned, no blockers — ready for upgrade", result.CurrentVersion))
	}

	return recs
}
