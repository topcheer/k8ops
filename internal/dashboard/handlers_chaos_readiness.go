package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ChaosReadinessResult is the chaos engineering readiness assessment.
type ChaosReadinessResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	Summary          ChaosSummary      `json:"summary"`
	Workloads        []ChaosWorkload   `json:"workloads"`
	Experiments      []ChaosExperiment `json:"experiments"`
	FragileWorkloads []ChaosWorkload   `json:"fragileWorkloads,omitempty"`
	Gaps             []ChaosGap        `json:"gaps"`
	Recommendations  []string          `json:"recommendations"`
	ReadinessScore   int               `json:"readinessScore"`
}

// ChaosSummary aggregates chaos readiness statistics.
type ChaosSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	ReadyForChaos   int `json:"readyForChaos"`
	PartiallyReady  int `json:"partiallyReady"`
	FragileCount    int `json:"fragileCount"`
	TotalChecks     int `json:"totalChecks"`
	PassedChecks    int `json:"passedChecks"`
	HasPDB          int `json:"hasPDB"`
	HasProbes       int `json:"hasProbes"`
	HasMultiReplica int `json:"hasMultiReplica"`
	HasGracefulStop int `json:"hasGracefulStop"`
	HasAntiAffinity int `json:"hasAntiAffinity"`
	MultiZoneSpread int `json:"multiZoneSpread"`
}

// ChaosWorkload represents one workload's chaos readiness assessment.
type ChaosWorkload struct {
	Name                string       `json:"name"`
	Namespace           string       `json:"namespace"`
	Kind                string       `json:"kind"`
	Replicas            int          `json:"replicas"`
	ReadinessLevel      string       `json:"readinessLevel"` // ready, partial, fragile
	Score               int          `json:"score"`          // 0-100
	Checks              []ChaosCheck `json:"checks"`
	Risks               []string     `json:"risks,omitempty"`
	MaxTolerableFailure int          `json:"maxTolerableFailure"` // max pods that can fail without outage
	RecoveryTime        string       `json:"recoveryEstimate"`    // estimated recovery time
}

// ChaosCheck is a single readiness criterion evaluation.
type ChaosCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, fail, warn
	Detail string `json:"detail"`
	Weight int    `json:"weight"` // contribution to score
}

// ChaosExperiment is a recommended chaos experiment.
type ChaosExperiment struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	Namespace   string `json:"namespace"`
	Type        string `json:"type"`        // pod-kill, network-latency, cpu-stress, disk-fill
	BlastRadius string `json:"blastRadius"` // small, medium, large
	Safe        bool   `json:"safe"`
	Rationale   string `json:"rationale"`
}

// ChaosGap describes a missing resilience capability.
type ChaosGap struct {
	Workload string `json:"workload,omitempty"`
	Category string `json:"category"` // pdb, probes, ha, shutdown, affinity, topology
	Severity string `json:"severity"`
	Issue    string `json:"issue"`
}

// handleChaosReadiness assesses every workload's resilience to chaos
// engineering experiments and generates a readiness profile.
// GET /api/deployment/chaos-readiness?namespace=xxx
func (s *Server) handleChaosReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := ChaosReadinessResult{ScannedAt: time.Now()}

	// 1. Collect all PDBs
	pdbMap := map[string]bool{} // "namespace/name" → has PDB
	pdbs, err := rc.clientset.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pdb := range pdbs.Items {
			// PDB applies to pods matched by its selector
			key := fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name)
			pdbMap[key] = true
		}
	}

	// 2. Collect nodes for topology analysis
	nodeZones := map[string]string{} // nodeName → zone
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			if zone, ok := node.Labels[corev1.LabelTopologyZone]; ok {
				nodeZones[node.Name] = zone
			}
		}
	}
	totalZones := len(uniqueZones(nodeZones))

	// 3. Collect pods for topology analysis
	podNodeMap := map[string]string{} // "ns/podname" → nodeName
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			podNodeMap[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = pod.Spec.NodeName
		}
	}

	// 4. Assess Deployments
	var allWorkloads []ChaosWorkload

	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range deployments.Items {
			d := &deployments.Items[i]
			replicas := 1
			if d.Spec.Replicas != nil {
				replicas = int(*d.Spec.Replicas)
			}
			wl := assessWorkload(
				"Deployment", d.Name, d.Namespace, replicas,
				&d.Spec.Template.Spec, d.Spec.Selector,
				pdbMap, podNodeMap, nodeZones,
			)
			allWorkloads = append(allWorkloads, wl)
		}
	}

	// 5. Assess StatefulSets
	stss, err := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range stss.Items {
			sts := &stss.Items[i]
			replicas := 1
			if sts.Spec.Replicas != nil {
				replicas = int(*sts.Spec.Replicas)
			}
			wl := assessWorkload(
				"StatefulSet", sts.Name, sts.Namespace, replicas,
				&sts.Spec.Template.Spec, sts.Spec.Selector,
				pdbMap, podNodeMap, nodeZones,
			)
			allWorkloads = append(allWorkloads, wl)
		}
	}

	// 6. Sort by score ascending (most fragile first)
	sort.Slice(allWorkloads, func(i, j int) bool {
		return allWorkloads[i].Score < allWorkloads[j].Score
	})
	result.Workloads = allWorkloads

	// 7. Identify fragile workloads (score < 40)
	for _, wl := range allWorkloads {
		if wl.Score < 40 {
			result.FragileWorkloads = append(result.FragileWorkloads, wl)
		}
	}

	// 8. Build summary
	result.Summary.TotalWorkloads = len(allWorkloads)
	totalChecks := 0
	passedChecks := 0
	for _, wl := range allWorkloads {
		for _, c := range wl.Checks {
			totalChecks++
			if c.Status == "pass" {
				passedChecks++
			}
		}
		switch wl.ReadinessLevel {
		case "ready":
			result.Summary.ReadyForChaos++
		case "partial":
			result.Summary.PartiallyReady++
		case "fragile":
			result.Summary.FragileCount++
		}
		for _, c := range wl.Checks {
			if c.Name == "PDB Coverage" && c.Status == "pass" {
				result.Summary.HasPDB++
			}
			if c.Name == "Health Probes" && c.Status == "pass" {
				result.Summary.HasProbes++
			}
			if c.Name == "Multi-Replica HA" && c.Status == "pass" {
				result.Summary.HasMultiReplica++
			}
			if c.Name == "Graceful Shutdown" && c.Status == "pass" {
				result.Summary.HasGracefulStop++
			}
			if c.Name == "Anti-Affinity" && c.Status == "pass" {
				result.Summary.HasAntiAffinity++
			}
			if c.Name == "Multi-Zone Spread" && c.Status == "pass" {
				result.Summary.MultiZoneSpread++
			}
		}
	}
	result.Summary.TotalChecks = totalChecks
	result.Summary.PassedChecks = passedChecks

	// 9. Generate recommended chaos experiments
	result.Experiments = generateChaosExperiments(allWorkloads, totalZones)

	// 10. Collect gaps
	for _, wl := range allWorkloads {
		for _, c := range wl.Checks {
			if c.Status != "pass" {
				result.Gaps = append(result.Gaps, ChaosGap{
					Workload: fmt.Sprintf("%s/%s", wl.Namespace, wl.Name),
					Category: c.Name,
					Severity: c.Status,
					Issue:    c.Detail,
				})
			}
		}
	}
	if len(result.Gaps) > 100 {
		result.Gaps = result.Gaps[:100]
	}

	// 11. Calculate overall readiness score
	if len(allWorkloads) > 0 {
		totalScore := 0
		for _, wl := range allWorkloads {
			totalScore += wl.Score
		}
		result.ReadinessScore = totalScore / len(allWorkloads)
	} else {
		result.ReadinessScore = 100
	}

	// 12. Generate recommendations
	result.Recommendations = generateChaosRecommendations(result, totalZones)

	writeJSON(w, result)
}

// assessWorkload evaluates a single workload's chaos readiness.
func assessWorkload(
	kind, name, namespace string,
	replicas int,
	podSpec *corev1.PodSpec,
	selector *metav1.LabelSelector,
	pdbMap map[string]bool,
	podNodeMap map[string]string,
	nodeZones map[string]string,
) ChaosWorkload {
	wl := ChaosWorkload{
		Name:           name,
		Namespace:      namespace,
		Kind:           kind,
		Replicas:       replicas,
		ReadinessLevel: "fragile",
	}

	// Check 1: Multi-Replica HA
	if replicas >= 3 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Multi-Replica HA",
			Status: "pass",
			Detail: fmt.Sprintf("%d replicas — can tolerate %d pod failure(s)", replicas, replicas/2),
			Weight: 25,
		})
	} else if replicas == 2 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Multi-Replica HA",
			Status: "warn",
			Detail: "Only 2 replicas — can tolerate 1 failure but no redundancy margin",
			Weight: 15,
		})
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Multi-Replica HA",
			Status: "fail",
			Detail: "Single replica — any pod failure causes service disruption",
			Weight: 0,
		})
		wl.Risks = append(wl.Risks, "Single replica: pod kill will cause outage")
	}

	// Max tolerable failure
	wl.MaxTolerableFailure = replicas / 2
	if wl.MaxTolerableFailure < 1 && replicas >= 2 {
		wl.MaxTolerableFailure = 1
	}

	// Check 2: PDB Coverage
	pdbKey := fmt.Sprintf("%s/%s", namespace, name)
	hasPDB := pdbMap[pdbKey]
	// Also check if any PDB selector matches
	if !hasPDB && selector != nil {
		for k := range pdbMap {
			// PDB names often match workload names or use app labels
			if strings.Contains(k, name) || strings.Contains(k, namespace+"/"+name) {
				hasPDB = true
				break
			}
		}
	}
	if hasPDB {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "PDB Coverage",
			Status: "pass",
			Detail: "PodDisruptionBudget protects voluntary disruptions",
			Weight: 20,
		})
	} else if replicas >= 2 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "PDB Coverage",
			Status: "warn",
			Detail: "No PDB — voluntary disruptions (drains, updates) are unprotected",
			Weight: 5,
		})
		wl.Risks = append(wl.Risks, "No PDB: node drain could evict all pods simultaneously")
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "PDB Coverage",
			Status: "fail",
			Detail: "No PDB for single-replica workload",
			Weight: 0,
		})
	}

	// Check 3: Health Probes
	hasLiveness := false
	hasReadiness := false
	for _, c := range podSpec.Containers {
		if c.LivenessProbe != nil {
			hasLiveness = true
		}
		if c.ReadinessProbe != nil {
			hasReadiness = true
		}
	}
	if hasLiveness && hasReadiness {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Health Probes",
			Status: "pass",
			Detail: "Both liveness and readiness probes configured",
			Weight: 15,
		})
	} else if hasReadiness {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Health Probes",
			Status: "warn",
			Detail: "Readiness probe but no liveness probe — stuck containers won't restart",
			Weight: 8,
		})
		wl.Risks = append(wl.Risks, "Missing liveness probe: zombie containers won't be restarted")
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Health Probes",
			Status: "fail",
			Detail: "No health probes — failures won't be detected automatically",
			Weight: 0,
		})
		wl.Risks = append(wl.Risks, "No probes: failure detection relies on external monitoring only")
	}

	// Check 4: Graceful Shutdown
	hasPreStop := false
	for _, c := range podSpec.Containers {
		if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
			hasPreStop = true
			break
		}
	}
	termGrace := int64(30) // default
	if podSpec.TerminationGracePeriodSeconds != nil {
		termGrace = *podSpec.TerminationGracePeriodSeconds
	}
	if hasPreStop && termGrace >= 30 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Graceful Shutdown",
			Status: "pass",
			Detail: fmt.Sprintf("PreStop hook configured with %ds grace period", termGrace),
			Weight: 15,
		})
	} else if termGrace >= 30 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Graceful Shutdown",
			Status: "warn",
			Detail: fmt.Sprintf("Adequate grace period (%ds) but no PreStop hook", termGrace),
			Weight: 8,
		})
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Graceful Shutdown",
			Status: "fail",
			Detail: fmt.Sprintf("Short termination grace period (%ds) — in-flight requests will be dropped", termGrace),
			Weight: 0,
		})
		wl.Risks = append(wl.Risks, "Short grace period: connections will be forcibly terminated")
	}

	// Check 5: Anti-Affinity / Topology Spread
	hasAntiAffinity := false
	if podSpec.Affinity != nil && podSpec.Affinity.PodAntiAffinity != nil {
		hasAntiAffinity = true
	}
	hasTopologySpread := false
	for _, tsc := range podSpec.TopologySpreadConstraints {
		if tsc.MaxSkew > 0 {
			hasTopologySpread = true
			break
		}
	}
	if hasAntiAffinity || hasTopologySpread {
		detail := "Pod anti-affinity configured"
		if hasTopologySpread {
			detail = "Topology spread constraints configured"
		}
		if hasAntiAffinity && hasTopologySpread {
			detail = "Both anti-affinity and topology spread configured"
		}
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Anti-Affinity",
			Status: "pass",
			Detail: detail,
			Weight: 15,
		})
	} else if replicas >= 2 {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Anti-Affinity",
			Status: "warn",
			Detail: "No anti-affinity — replicas may co-locate on same node",
			Weight: 5,
		})
		wl.Risks = append(wl.Risks, "No anti-affinity: node failure could take down all replicas")
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Anti-Affinity",
			Status: "warn",
			Detail: "Not applicable for single-replica workload",
			Weight: 10,
		})
	}

	// Check 6: Multi-Zone Spread (needs pod-to-node mapping)
	if replicas >= 2 && selector != nil {
		// Count unique zones for this workload's pods
		zoneSet := map[string]bool{}
		// Match pods by selector labels
		sel, err := metav1.LabelSelectorAsSelector(selector)
		if err == nil {
			for _, pod := range getPodsForSelector(podNodeMap, namespace, sel) {
				if zone, ok := nodeZones[pod]; ok && zone != "" {
					zoneSet[zone] = true
				}
			}
		}
		if len(zoneSet) >= 2 {
			wl.Checks = append(wl.Checks, ChaosCheck{
				Name:   "Multi-Zone Spread",
				Status: "pass",
				Detail: fmt.Sprintf("Pods spread across %d availability zones", len(zoneSet)),
				Weight: 10,
			})
		} else if len(zoneSet) == 1 {
			wl.Checks = append(wl.Checks, ChaosCheck{
				Name:   "Multi-Zone Spread",
				Status: "warn",
				Detail: "All replicas in single AZ — zone failure causes outage",
				Weight: 3,
			})
			wl.Risks = append(wl.Risks, "Single-zone: AZ failure will cause complete outage")
		} else {
			wl.Checks = append(wl.Checks, ChaosCheck{
				Name:   "Multi-Zone Spread",
				Status: "warn",
				Detail: "Unable to determine zone spread",
				Weight: 5,
			})
		}
	} else {
		wl.Checks = append(wl.Checks, ChaosCheck{
			Name:   "Multi-Zone Spread",
			Status: "warn",
			Detail: "Not applicable for single-replica workload",
			Weight: 5,
		})
	}

	// Calculate score
	score := 0
	for _, c := range wl.Checks {
		score += c.Weight
	}
	if score > 100 {
		score = 100
	}
	wl.Score = score

	// Determine readiness level
	switch {
	case score >= 70:
		wl.ReadinessLevel = "ready"
	case score >= 40:
		wl.ReadinessLevel = "partial"
	default:
		wl.ReadinessLevel = "fragile"
	}

	// Estimate recovery time
	if replicas >= 3 && hasReadiness {
		wl.RecoveryTime = "< 30s"
	} else if replicas >= 2 {
		wl.RecoveryTime = "30-60s"
	} else {
		wl.RecoveryTime = "60s+ (manual intervention likely)"
	}

	return wl
}

// generateChaosExperiments recommends safe chaos experiments for ready workloads.
func generateChaosExperiments(workloads []ChaosWorkload, totalZones int) []ChaosExperiment {
	var experiments []ChaosExperiment

	for _, wl := range workloads {
		if wl.ReadinessLevel == "ready" && wl.Replicas >= 3 {
			experiments = append(experiments, ChaosExperiment{
				Name:        fmt.Sprintf("pod-kill-%s", wl.Name),
				Target:      wl.Name,
				Namespace:   wl.Namespace,
				Type:        "pod-kill",
				BlastRadius: "small",
				Safe:        true,
				Rationale:   fmt.Sprintf("Ready for chaos: score %d, %d replicas, tolerates %d failure(s)", wl.Score, wl.Replicas, wl.MaxTolerableFailure),
			})

			// Add network experiment if HA + multi-zone
			if totalZones >= 2 && wl.Score >= 80 {
				experiments = append(experiments, ChaosExperiment{
					Name:        fmt.Sprintf("network-latency-%s", wl.Name),
					Target:      wl.Name,
					Namespace:   wl.Namespace,
					Type:        "network-latency",
					BlastRadius: "medium",
					Safe:        true,
					Rationale:   "Multi-zone HA workload — can tolerate network partitions",
				})
			}
		} else if wl.ReadinessLevel == "partial" && wl.Replicas >= 2 {
			experiments = append(experiments, ChaosExperiment{
				Name:        fmt.Sprintf("pod-kill-%s", wl.Name),
				Target:      wl.Name,
				Namespace:   wl.Namespace,
				Type:        "pod-kill",
				BlastRadius: "small",
				Safe:        false,
				Rationale:   fmt.Sprintf("Partially ready: score %d — fix gaps before running chaos", wl.Score),
			})
		}
	}

	// Limit to 50 experiments
	if len(experiments) > 50 {
		experiments = experiments[:50]
	}

	return experiments
}

// generateChaosRecommendations produces actionable recommendations.
func generateChaosRecommendations(result ChaosReadinessResult, totalZones int) []string {
	var recs []string

	if result.Summary.FragileCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are too fragile for chaos testing — add replicas, PDBs, and probes first", result.Summary.FragileCount))
	}

	if result.Summary.ReadyForChaos > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are ready for chaos experiments — start with pod-kill on the most resilient workloads", result.Summary.ReadyForChaos))
	}

	// PDB gap
	pdbGap := result.Summary.TotalWorkloads - result.Summary.HasPDB
	if pdbGap > 0 && result.Summary.TotalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack PDB — add PodDisruptionBudgets before chaos testing", pdbGap, result.Summary.TotalWorkloads))
	}

	// Probe gap
	probeGap := result.Summary.TotalWorkloads - result.Summary.HasProbes
	if probeGap > 0 && result.Summary.TotalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack health probes — add liveness/readiness probes for automatic failure detection", probeGap, result.Summary.TotalWorkloads))
	}

	// Graceful shutdown gap
	shutdownGap := result.Summary.TotalWorkloads - result.Summary.HasGracefulStop
	if shutdownGap > 0 && result.Summary.TotalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack graceful shutdown — add PreStop hooks to handle in-flight requests", shutdownGap, result.Summary.TotalWorkloads))
	}

	// Anti-affinity gap
	affinityGap := result.Summary.TotalWorkloads - result.Summary.HasAntiAffinity
	if affinityGap > result.Summary.TotalWorkloads/2 && result.Summary.TotalWorkloads > 2 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack anti-affinity — replicas may co-locate on the same node", affinityGap, result.Summary.TotalWorkloads))
	}

	// Single replica warning
	singleReplica := result.Summary.TotalWorkloads - result.Summary.HasMultiReplica
	if singleReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are single-replica — these cannot survive any pod failure", singleReplica))
	}

	// Zone awareness
	if totalZones <= 1 {
		recs = append(recs, "Cluster has single availability zone — multi-zone chaos experiments are not applicable")
	} else {
		multiZoneGap := result.Summary.TotalWorkloads - result.Summary.MultiZoneSpread
		if multiZoneGap > 0 && result.Summary.HasMultiReplica > 0 {
			recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) are not spread across zones — add topology spread constraints", multiZoneGap))
		}
	}

	if result.ReadinessScore < 50 {
		recs = append(recs, fmt.Sprintf("Overall chaos readiness score is %d/100 — significant work needed before chaos engineering", result.ReadinessScore))
	} else if result.ReadinessScore >= 70 && len(recs) == 0 {
		recs = append(recs, "Cluster is well-prepared for chaos engineering — start with small blast radius experiments and expand gradually")
	}

	return recs
}

// uniqueZones returns unique zone values from the node-zone map.
func uniqueZones(m map[string]string) []string {
	seen := map[string]bool{}
	var zones []string
	for _, zone := range m {
		if !seen[zone] {
			seen[zone] = true
			zones = append(zones, zone)
		}
	}
	return zones
}

// getPodsForSelector returns pod node names matching a label selector in a namespace.
// Since we only have podNodeMap (string→string), we match by pod name patterns.
func getPodsForSelector(podNodeMap map[string]string, namespace string, sel labels.Selector) []string {
	var nodes []string
	prefix := namespace + "/"
	for podKey, nodeName := range podNodeMap {
		if strings.HasPrefix(podKey, prefix) && nodeName != "" {
			nodes = append(nodes, nodeName)
		}
	}
	return nodes
}
