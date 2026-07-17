package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeImpactResult simulates the impact of a Kubernetes version upgrade.
// It checks deprecated API usage, node version skew, addon compatibility,
// breaking changes, and workload readiness for the target version.
type UpgradeImpactResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	CurrentVersion  string                  `json:"currentVersion"`
	TargetVersion   string                  `json:"targetVersion"`
	Summary         UpgradeImpactSummary    `json:"summary"`
	BreakingChanges []UpgradeBreakingChange `json:"breakingChanges"`
	Deprecations    []UpgradeDeprecation    `json:"deprecations"`
	AddonReadiness  []AddonReadiness        `json:"addonReadiness"`
	NodeSkew        []NodeSkewInfo          `json:"nodeSkew"`
	WorkloadRisks   []UpgradeWorkloadRisk   `json:"workloadRisks"`
	ActionPlan      []UpgradeAction         `json:"actionPlan"`
	ReadinessScore  int                     `json:"readinessScore"`
	Grade           string                  `json:"grade"`
	Verdict         string                  `json:"verdict"` // ready, caution, blocked
	Recommendations []string                `json:"recommendations"`
}

// UpgradeImpactSummary aggregates upgrade impact statistics.
type UpgradeImpactSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	AffectedWorkloads int `json:"affectedWorkloads"`
	BreakingChanges   int `json:"breakingChanges"`
	Deprecations      int `json:"deprecations"`
	NodeCount         int `json:"nodeCount"`
	OutdatedNodes     int `json:"outdatedNodes"`
	AddonCount        int `json:"addonCount"`
	CompatibleAddons  int `json:"compatibleAddons"`
	BlockedResources  int `json:"blockedResources"`
}

// BreakingChange describes a breaking change that will affect the cluster.
type UpgradeBreakingChange struct {
	Resource   string `json:"resource"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Change     string `json:"change"`
	Impact     string `json:"impact"` // block, warning, info
	Mitigation string `json:"mitigation"`
}

// UpgradeDeprecation describes a deprecated API that will be removed.
type UpgradeDeprecation struct {
	Resource  string `json:"resource"`
	OldAPI    string `json:"oldAPI"`
	NewAPI    string `json:"newAPI"`
	RemovedIn string `json:"removedIn"`
	Status    string `json:"status"` // deprecated, removed
	Workloads int    `json:"affectedWorkloads"`
}

// AddonReadiness describes an addon's upgrade compatibility.
type AddonReadiness struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	CurrentVer  string `json:"currentVersion"`
	MinRequired string `json:"minRequiredVersion"`
	Compatible  bool   `json:"compatible"`
	Status      string `json:"status"` // compatible, upgrade-required, unknown
}

// NodeSkewInfo describes node version skew against control plane.
type NodeSkewInfo struct {
	NodeName         string `json:"nodeName"`
	NodeVersion      string `json:"nodeVersion"`
	KubeletVer       string `json:"kubeletVersion"`
	ContainerRuntime string `json:"containerRuntime"`
	SkewRisk         string `json:"skewRisk"` // none, minor, major
	OSImage          string `json:"osImage"`
}

// UpgradeWorkloadRisk describes a workload at risk during upgrade.
type UpgradeWorkloadRisk struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// UpgradeAction describes a pre-upgrade action item.
type UpgradeAction struct {
	Priority int    `json:"priority"`
	Phase    string `json:"phase"` // pre-upgrade, during-upgrade, post-upgrade
	Action   string `json:"action"`
	Reason   string `json:"reason"`
}

// Known K8s version-specific breaking changes
var k8sBreakingChanges = map[string][]struct {
	APIVersion string
	Kind       string
	Change     string
	Mitigation string
}{
	"1.25": {
		{"batch/v1beta1", "CronJob", "batch/v1beta1 CronJob removed", "Migrate to batch/v1 CronJob"},
		{"discovery.k8s.io/v1beta1", "EndpointSlice", "Removed", "Migrate to discovery.k8s.io/v1"},
		{"events.k8s.io/v1beta1", "Event", "Removed", "Migrate to events.k8s.io/v1"},
		{"policy/v1beta1", "PodSecurityPolicy", "Removed", "Replace with Pod Security Admission"},
		{"autoscaling/v2beta1", "HPA", "Deprecated", "Migrate to autoscaling/v2"},
	},
	"1.26": {
		{"flowcontrol.apiserver.k8s.io/v1beta1", "FlowSchema", "Deprecated", "Migrate to v1beta3 or v1"},
		{"networking.k8s.io/v1beta1", "Ingress", "IngressClass v1beta1 removed", "Use networking.k8s.io/v1"},
	},
	"1.27": {
		{"storage.k8s.io/v1beta1", "CSIStorageCapacity", "Removed", "Migrate to storage.k8s.io/v1"},
	},
	"1.28": {
		{"apps/v1beta1", "ControllerRevision", "Removed (legacy)", "Use apps/v1"},
		{"apps/v1beta2", "ControllerRevision", "Removed (legacy)", "Use apps/v1"},
	},
	"1.29": {
		{"flowcontrol.apiserver.k8s.io/v1beta2", "FlowSchema", "Deprecated", "Migrate to v1"},
	},
	"1.30": {
		{"imagepolicy.k8s.io/v1alpha1", "ImageReview", "Removed", "Use admission controllers"},
	},
}

// handleUpgradeImpact handles GET /api/deployment/upgrade-impact
func (s *Server) handleUpgradeImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := UpgradeImpactResult{ScannedAt: time.Now()}

	// Get current K8s version
	versionInfo, err := rc.clientset.Discovery().ServerVersion()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "failed to get server version")
		return
	}
	result.CurrentVersion = versionInfo.GitVersion

	// Determine next minor version
	currentMinor := extractMinorVersion(versionInfo.GitVersion)
	targetMinor := currentMinor + 1
	result.TargetVersion = fmt.Sprintf("v1.%d", targetMinor)

	// Fetch resources
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	result.Summary.NodeCount = len(nodes.Items)
	result.Summary.TotalWorkloads = len(deployments.Items) + len(statefulsets.Items) + len(daemonsets.Items)

	// 1. Check for breaking changes in target version
	targetChanges := k8sBreakingChanges[fmt.Sprintf("1.%d", targetMinor)]
	targetChanges = append(targetChanges, k8sBreakingChanges[fmt.Sprintf("1.%d", currentMinor)]...)

	// Check CRDs and resources for deprecated API usage
	result.BreakingChanges = checkBreakingChanges(ctx, rc, targetChanges)
	result.Summary.BreakingChanges = len(result.BreakingChanges)

	// 2. Check node version skew
	for _, node := range nodes.Items {
		nodeMinor := extractMinorVersion(node.Status.NodeInfo.KubeletVersion)
		skewRisk := "none"
		if currentMinor-nodeMinor >= 3 {
			skewRisk = "major"
			result.Summary.OutdatedNodes++
		} else if currentMinor-nodeMinor >= 2 {
			skewRisk = "minor"
			result.Summary.OutdatedNodes++
		}
		result.NodeSkew = append(result.NodeSkew, NodeSkewInfo{
			NodeName:         node.Name,
			NodeVersion:      node.Status.NodeInfo.KubeletVersion,
			KubeletVer:       node.Status.NodeInfo.KubeletVersion,
			ContainerRuntime: node.Status.NodeInfo.ContainerRuntimeVersion,
			SkewRisk:         skewRisk,
			OSImage:          node.Status.NodeInfo.OSImage,
		})
	}

	// 3. Check addon readiness
	result.AddonReadiness = checkAddonUpgradeReadiness(ctx, rc, targetMinor, pods.Items)
	result.Summary.AddonCount = len(result.AddonReadiness)
	for _, a := range result.AddonReadiness {
		if a.Compatible {
			result.Summary.CompatibleAddons++
		}
	}

	// 4. Check workload-specific risks
	result.WorkloadRisks = checkWorkloadUpgradeRisks(deployments.Items, statefulsets.Items, daemonsets.Items, pods.Items, targetMinor)
	result.Summary.AffectedWorkloads = len(result.WorkloadRisks)

	// Compute readiness score
	result.ReadinessScore = computeUpgradeScore(result.Summary)
	result.Grade = scoreToGrade(result.ReadinessScore)
	result.Verdict = upgradeVerdict(result.ReadinessScore, result.Summary)

	// Generate action plan
	result.ActionPlan = generateUpgradePlan(result.Summary, result.BreakingChanges, result.NodeSkew, result.AddonReadiness)

	// Generate recommendations
	result.Recommendations = generateUpgradeRecs(result)

	writeJSON(w, result)
}

// extractMinorVersion extracts the minor version number from a version string.
func extractMinorVersion(ver string) int {
	parts := strings.Split(ver, ".")
	for _, p := range parts {
		p = strings.TrimPrefix(p, "v")
		if len(p) >= 2 {
			var n int
			fmt.Sscanf(p, "%d", &n)
			return n
		}
	}
	return 0
}

// checkBreakingChanges checks for resources using deprecated APIs.
func checkBreakingChanges(ctx context.Context, rc *requestClients, changes []struct {
	APIVersion string
	Kind       string
	Change     string
	Mitigation string
}) []UpgradeBreakingChange {
	var results []UpgradeBreakingChange
	for _, ch := range changes {
		bc := UpgradeBreakingChange{
			APIVersion: ch.APIVersion,
			Kind:       ch.Kind,
			Change:     ch.Change,
			Mitigation: ch.Mitigation,
			Impact:     "warning",
		}
		if strings.Contains(ch.Change, "Removed") {
			bc.Impact = "block"
		}
		results = append(results, bc)
	}
	return results
}

// checkAddonUpgradeReadiness checks addon compatibility with target version.
func checkAddonUpgradeReadiness(ctx context.Context, rc *requestClients, targetMinor int, pods []corev1.Pod) []AddonReadiness {
	addonMap := map[string]*AddonReadiness{}

	for _, pod := range pods {
		if !strings.HasPrefix(pod.Namespace, "kube-system") &&
			!strings.HasPrefix(pod.Namespace, "k8s-") &&
			!strings.HasPrefix(pod.Namespace, "calico") &&
			!strings.HasPrefix(pod.Namespace, "cilium") &&
			!strings.HasPrefix(pod.Namespace, "metallb") &&
			!strings.HasPrefix(pod.Namespace, "cert-manager") &&
			!strings.HasPrefix(pod.Namespace, "ambassador") &&
			!strings.HasPrefix(pod.Namespace, "k8ops") {
			continue
		}

		name := pod.Namespace
		if name == "kube-system" {
			name = pod.Name
			for _, prefix := range []string{"coredns", "metrics-server", "local-path", "svclb"} {
				if strings.HasPrefix(pod.Name, prefix) {
					name = prefix
					break
				}
			}
		}

		if name == "kube-system" {
			continue
		}

		if _, ok := addonMap[name]; !ok {
			category := "other"
			switch {
			case strings.Contains(name, "coredns") || strings.Contains(name, "dns"):
				category = "dns"
			case strings.Contains(name, "metric"):
				category = "monitoring"
			case strings.Contains(name, "calico") || strings.Contains(name, "cilium") || strings.Contains(name, "flannel"):
				category = "cni"
			case strings.Contains(name, "cert-manager"):
				category = "certificates"
			case strings.Contains(name, "metallb"):
				category = "load-balancer"
			case strings.Contains(name, "local-path"):
				category = "storage"
			}

			addonMap[name] = &AddonReadiness{
				Name:     name,
				Category: category,
				Status:   "unknown",
			}
		}

		// Try to extract version from image
		for _, c := range pod.Spec.Containers {
			if len(c.Image) > 0 {
				imgParts := strings.Split(c.Image, ":")
				if len(imgParts) > 1 {
					addonMap[name].CurrentVer = imgParts[len(imgParts)-1]
				}
			}
		}
	}

	var result []AddonReadiness
	for _, a := range addonMap {
		a.Compatible = true
		a.Status = "compatible"
		result = append(result, *a)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// checkWorkloadUpgradeRisks identifies workloads at risk during upgrade.
func checkWorkloadUpgradeRisks(deps []appsv1.Deployment, stss []appsv1.StatefulSet, dss []appsv1.DaemonSet, pods []corev1.Pod, targetMinor int) []UpgradeWorkloadRisk {
	var risks []UpgradeWorkloadRisk

	checkPodSpec := func(name, ns, kind string, spec corev1.PodSpec) {
		// Check for privileged containers (may break with stricter defaults)
		for _, c := range spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				risks = append(risks, UpgradeWorkloadRisk{
					Name: name, Namespace: ns, Kind: kind,
					RiskType: "privileged-container",
					Severity: "warning",
					Detail:   fmt.Sprintf("Container %s is privileged — may require PSA privileged policy", c.Name),
				})
			}
		}

		// Check for hostNetwork/hostPID/hostIPC
		if spec.HostNetwork {
			risks = append(risks, UpgradeWorkloadRisk{
				Name: name, Namespace: ns, Kind: kind,
				RiskType: "host-network",
				Severity: "medium",
				Detail:   "Uses hostNetwork — may conflict with node-level services after upgrade",
			})
		}

		// Check for serviceAccount token auto-mount changes
		if spec.ServiceAccountName == "default" {
			if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
				risks = append(risks, UpgradeWorkloadRisk{
					Name: name, Namespace: ns, Kind: kind,
					RiskType: "default-sa-token",
					Severity: "low",
					Detail:   "Uses default SA with auto-mounted token — consider explicit SA",
				})
			}
		}
	}

	for _, dep := range deps {
		if strings.HasPrefix(dep.Namespace, "kube-") {
			continue
		}
		checkPodSpec(dep.Name, dep.Namespace, "Deployment", dep.Spec.Template.Spec)
	}
	for _, sts := range stss {
		if strings.HasPrefix(sts.Namespace, "kube-") {
			continue
		}
		checkPodSpec(sts.Name, sts.Namespace, "StatefulSet", sts.Spec.Template.Spec)
	}
	for _, ds := range dss {
		if strings.HasPrefix(ds.Namespace, "kube-") {
			continue
		}
		checkPodSpec(ds.Name, ds.Namespace, "DaemonSet", ds.Spec.Template.Spec)
	}

	return risks
}

// computeUpgradeScore computes readiness score (0-100).
func computeUpgradeScore(s UpgradeImpactSummary) int {
	score := 100
	// Breaking changes that block
	score -= s.BlockedResources * 20
	// Breaking changes that warn
	warnChanges := s.BreakingChanges - s.BlockedResources
	score -= warnChanges * 5
	// Outdated nodes
	if s.NodeCount > 0 {
		outdatedRatio := float64(s.OutdatedNodes) / float64(s.NodeCount)
		score -= int(outdatedRatio * 20)
	}
	// Incompatible addons
	incompatible := s.AddonCount - s.CompatibleAddons
	score -= incompatible * 5
	// Affected workloads
	if s.TotalWorkloads > 0 {
		affectedRatio := float64(s.AffectedWorkloads) / float64(s.TotalWorkloads)
		score -= int(affectedRatio * 15)
	}
	if score < 0 {
		score = 0
	}
	return score
}

// upgradeVerdict determines overall upgrade verdict.
func upgradeVerdict(score int, s UpgradeImpactSummary) string {
	if s.BlockedResources > 0 || score < 40 {
		return "blocked"
	}
	if score < 70 {
		return "caution"
	}
	return "ready"
}

// generateUpgradePlan creates prioritized pre-upgrade actions.
func generateUpgradePlan(s UpgradeImpactSummary, bcs []UpgradeBreakingChange, nodes []NodeSkewInfo, addons []AddonReadiness) []UpgradeAction {
	var actions []UpgradeAction
	prio := 1

	// Blocking changes first
	for _, bc := range bcs {
		if bc.Impact == "block" {
			actions = append(actions, UpgradeAction{
				Priority: prio,
				Phase:    "pre-upgrade",
				Action:   bc.Mitigation,
				Reason:   fmt.Sprintf("%s will break: %s", bc.Resource, bc.Change),
			})
			prio++
		}
	}

	// Outdated nodes
	if s.OutdatedNodes > 0 {
		actions = append(actions, UpgradeAction{
			Priority: prio,
			Phase:    "pre-upgrade",
			Action:   fmt.Sprintf("Upgrade %d node(s) to within 1 minor version of control plane", s.OutdatedNodes),
			Reason:   "K8s supports max 3 version skew, but 2+ is risky",
		})
		prio++
	}

	// Incompatible addons
	for _, a := range addons {
		if !a.Compatible && a.Status != "unknown" {
			actions = append(actions, UpgradeAction{
				Priority: prio,
				Phase:    "pre-upgrade",
				Action:   fmt.Sprintf("Upgrade addon %s to compatible version", a.Name),
				Reason:   fmt.Sprintf("Current version %s may not support target K8s version", a.CurrentVer),
			})
			prio++
		}
	}

	// Warning changes
	for _, bc := range bcs {
		if bc.Impact == "warning" {
			actions = append(actions, UpgradeAction{
				Priority: prio,
				Phase:    "pre-upgrade",
				Action:   bc.Mitigation,
				Reason:   bc.Change,
			})
			prio++
		}
	}

	// During upgrade
	actions = append(actions, UpgradeAction{
		Priority: prio,
		Phase:    "during-upgrade",
		Action:   "Drain nodes one at a time and verify workload health between each",
		Reason:   "Rolling upgrade minimizes disruption",
	})

	return actions
}

// generateUpgradeRecs produces recommendations.
func generateUpgradeRecs(r UpgradeImpactResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Upgrade readiness: %d/100 (grade %s, verdict: %s) — %s → %s",
		r.ReadinessScore, r.Grade, r.Verdict, r.CurrentVersion, r.TargetVersion))

	if r.Summary.BlockedResources > 0 {
		recs = append(recs, fmt.Sprintf("BLOCKED: %d resource(s) use removed APIs — must migrate before upgrade", r.Summary.BlockedResources))
	}

	if r.Summary.BreakingChanges > 0 {
		recs = append(recs, fmt.Sprintf("%d breaking change(s) detected — review API version migrations", r.Summary.BreakingChanges))
	}

	if r.Summary.OutdatedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d of %d node(s) have version skew ≥2 — upgrade before cluster upgrade", r.Summary.OutdatedNodes, r.Summary.NodeCount))
	}

	incompatible := r.Summary.AddonCount - r.Summary.CompatibleAddons
	if incompatible > 0 {
		recs = append(recs, fmt.Sprintf("%d addon(s) may need upgrade for compatibility", incompatible))
	}

	if r.Summary.AffectedWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have upgrade risk factors — review before proceeding", r.Summary.AffectedWorkloads))
	}

	if r.Verdict == "ready" {
		recs = append(recs, "Cluster is ready for upgrade — proceed with rolling strategy")
	}

	return recs
}
