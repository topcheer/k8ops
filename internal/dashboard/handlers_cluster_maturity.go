package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterMaturityResult evaluates the cluster against a Kubernetes
// maturity model (Level 1-5), identifying which capabilities are
// implemented and which are missing for the next maturity level.
type ClusterMaturityResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	CurrentLevel    int                  `json:"currentLevel"`
	TargetLevel     int                  `json:"targetLevel"`
	LevelName       string               `json:"levelName"`
	NextLevelName   string               `json:"nextLevelName"`
	Capabilities    []MaturityCapability `json:"capabilities"`
	Gaps            []MaturityGap        `json:"gaps"`
	ScorePct        int                  `json:"scorePct"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type MaturityCapability struct {
	Category   string `json:"category"`
	Capability string `json:"capability"`
	Level      int    `json:"level"`
	Achieved   bool   `json:"achieved"`
	Detail     string `json:"detail"`
}

type MaturityGap struct {
	Capability string `json:"capability"`
	Level      int    `json:"level"`
	Detail     string `json:"detail"`
	Action     string `json:"action"`
}

var maturityLevels = map[int]string{
	1: "Ad Hoc",
	2: "Managed",
	3: "Defined",
	4: "Quantitatively Managed",
	5: "Optimizing",
}

// handleClusterMaturity handles GET /api/docs/cluster-maturity
func (s *Server) handleClusterMaturity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ClusterMaturityResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	var caps []MaturityCapability

	// Level 1: Basic workloads running
	podCount := 0
	for _, p := range pods.Items {
		if !isSystemNamespace(p.Namespace) && p.Status.Phase == "Running" {
			podCount++
		}
	}
	caps = append(caps, MaturityCapability{
		Category: "Base", Capability: "Workloads Running", Level: 1, Achieved: podCount > 0,
		Detail: fmt.Sprintf("%d running pods", podCount),
	})

	deployCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
	}
	caps = append(caps, MaturityCapability{
		Category: "Base", Capability: "Declarative Deployments", Level: 1, Achieved: deployCount > 0,
		Detail: fmt.Sprintf("%d deployments", deployCount),
	})

	// Level 2: Managed
	withLimits := 0
	totalContainers := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			totalContainers++
			if !c.Resources.Limits.Cpu().IsZero() && !c.Resources.Limits.Memory().IsZero() {
				withLimits++
			}
		}
	}
	limitPct := 0
	if totalContainers > 0 {
		limitPct = withLimits * 100 / totalContainers
	}
	caps = append(caps, MaturityCapability{
		Category: "Resources", Capability: "Resource Limits", Level: 2, Achieved: limitPct >= 80,
		Detail: fmt.Sprintf("%d/%d containers with limits (%d%%)", withLimits, totalContainers, limitPct),
	})

	withProbes := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe != nil && c.ReadinessProbe != nil {
				withProbes++
			}
		}
	}
	probePct := 0
	if totalContainers > 0 {
		probePct = withProbes * 100 / totalContainers
	}
	caps = append(caps, MaturityCapability{
		Category: "Reliability", Capability: "Health Probes", Level: 2, Achieved: probePct >= 70,
		Detail: fmt.Sprintf("%d/%d containers with probes (%d%%)", withProbes, totalContainers, probePct),
	})

	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	caps = append(caps, MaturityCapability{
		Category: "Availability", Capability: "Multi-Node", Level: 2, Achieved: workerCount >= 2,
		Detail: fmt.Sprintf("%d worker nodes", workerCount),
	})

	// Level 3: Defined
	nsTotal := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsTotal++
		}
	}
	nsWithQuota := 0
	for _, q := range quotas.Items {
		if !isSystemNamespace(q.Namespace) {
			nsWithQuota++
		}
	}
	quotaPct := 0
	if nsTotal > 0 {
		quotaPct = nsWithQuota * 100 / nsTotal
	}
	caps = append(caps, MaturityCapability{
		Category: "Governance", Capability: "Resource Quotas", Level: 3, Achieved: quotaPct >= 50,
		Detail: fmt.Sprintf("%d/%d namespaces with quota (%d%%)", nsWithQuota, nsTotal, quotaPct),
	})

	netpolNS := make(map[string]bool)
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			netpolNS[np.Namespace] = true
		}
	}
	netpolPct := 0
	if nsTotal > 0 {
		netpolPct = len(netpolNS) * 100 / nsTotal
	}
	caps = append(caps, MaturityCapability{
		Category: "Security", Capability: "Network Policies", Level: 3, Achieved: netpolPct >= 50,
		Detail: fmt.Sprintf("%d/%d namespaces with netpol (%d%%)", len(netpolNS), nsTotal, netpolPct),
	})

	caps = append(caps, MaturityCapability{
		Category: "Reliability", Capability: "PDB Coverage", Level: 3, Achieved: len(pdbs.Items) >= deployCount/3,
		Detail: fmt.Sprintf("%d PDBs / %d deployments", len(pdbs.Items), deployCount),
	})

	// Level 4: Quantitatively Managed
	caps = append(caps, MaturityCapability{
		Category: "Scaling", Capability: "HPA Autoscaling", Level: 4, Achieved: len(hpas.Items) > 0,
		Detail: fmt.Sprintf("%d HPAs", len(hpas.Items)),
	})

	withAffinity := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if d.Spec.Template.Spec.Affinity != nil || len(d.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			withAffinity++
		}
	}
	caps = append(caps, MaturityCapability{
		Category: "HA", Capability: "Anti-Affinity", Level: 4, Achieved: withAffinity >= deployCount/4,
		Detail: fmt.Sprintf("%d/%d deployments with anti-affinity", withAffinity, deployCount),
	})

	// Level 5: Optimizing
	psaEnforced := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) && ns.Labels["pod-security.kubernetes.io/enforce"] != "" {
			psaEnforced++
		}
	}
	caps = append(caps, MaturityCapability{
		Category: "Security", Capability: "Pod Security Admission", Level: 5, Achieved: psaEnforced >= nsTotal/2,
		Detail: fmt.Sprintf("%d/%d namespaces with PSA", psaEnforced, nsTotal),
	})

	// Calculate current level
	levelScores := make(map[int]struct{ achieved, total int })
	for _, c := range caps {
		s := levelScores[c.Level]
		s.total++
		if c.Achieved {
			s.achieved++
		}
		levelScores[c.Level] = s
	}

	currentLevel := 1
	for lvl := 1; lvl <= 5; lvl++ {
		s := levelScores[lvl]
		if s.total > 0 && s.achieved*100/s.total >= 60 {
			currentLevel = lvl
		} else {
			break
		}
	}

	result.CurrentLevel = currentLevel
	result.TargetLevel = currentLevel + 1
	if result.TargetLevel > 5 {
		result.TargetLevel = 5
	}
	result.LevelName = maturityLevels[currentLevel]
	result.NextLevelName = maturityLevels[result.TargetLevel]

	// Build gaps
	for _, c := range caps {
		if !c.Achieved && c.Level <= result.TargetLevel {
			result.Gaps = append(result.Gaps, MaturityGap{
				Capability: c.Capability, Level: c.Level,
				Detail: c.Detail, Action: gapAction(c.Capability),
			})
		}
	}

	// Score
	totalCaps := len(caps)
	achievedCaps := 0
	for _, c := range caps {
		if c.Achieved {
			achievedCaps++
		}
	}
	if totalCaps > 0 {
		result.ScorePct = achievedCaps * 100 / totalCaps
	}

	switch {
	case result.ScorePct >= 80:
		result.Grade = "A"
	case result.ScorePct >= 60:
		result.Grade = "B"
	case result.ScorePct >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Capabilities = caps
	result.Recommendations = buildMaturityRecs(&result)
	writeJSON(w, result)
}

func gapAction(cap string) string {
	actions := map[string]string{
		"Resource Limits":        "Set resources.limits on all containers",
		"Health Probes":          "Add livenessProbe and readinessProbe",
		"Multi-Node":             "Add worker nodes for HA",
		"Resource Quotas":        "Create ResourceQuota per namespace",
		"Network Policies":       "Add default-deny NetworkPolicy",
		"PDB Coverage":           "Create PodDisruptionBudget for multi-replica workloads",
		"HPA Autoscaling":        "Create HPA for critical workloads",
		"Anti-Affinity":          "Add podAntiAffinity to spread replicas",
		"Pod Security Admission": "Set pod-security.kubernetes.io/enforce=restricted",
	}
	if a, ok := actions[cap]; ok {
		return a
	}
	return "Refer to Kubernetes best practices"
}

func buildMaturityRecs(r *ClusterMaturityResult) []string {
	recs := []string{
		fmt.Sprintf("Current: Level %d (%s), Score %d%%", r.CurrentLevel, r.LevelName, r.ScorePct),
	}
	if len(r.Gaps) > 0 {
		recs = append(recs, fmt.Sprintf("Reach Level %d (%s) by fixing %d gaps:", r.TargetLevel, r.NextLevelName, len(r.Gaps)))
		shown := 0
		for _, g := range r.Gaps {
			if g.Level <= r.TargetLevel {
				recs = append(recs, fmt.Sprintf("  - [%s] %s", g.Capability, g.Action))
				shown++
				if shown >= 5 {
					break
				}
			}
		}
	}
	return recs
}
