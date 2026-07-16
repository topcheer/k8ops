package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceInventoryResult provides a comprehensive catalog of all cluster resources.
// It serves as a documentation/audit tool listing every resource type,
// counts, health status, age distribution, and ownership tracking.
type ResourceInventoryResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         InventorySummary       `json:"summary"`
	ByKind          []KindInventory        `json:"byKind"`
	ByNamespace     []NSInventory          `json:"byNamespace"`
	HealthSummary   InventoryHealth        `json:"healthSummary"`
	AgeDistribution AgeDistribution        `json:"ageDistribution"`
	LabelsCoverage  LabelsCoverage         `json:"labelsCoverage"`
	Orphaned        []OrphanedResource     `json:"orphaned"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

// InventorySummary aggregates resource counts.
type InventorySummary struct {
	TotalResources   int `json:"totalResources"`
	Namespaces       int `json:"namespaces"`
	Deployments      int `json:"deployments"`
	StatefulSets     int `json:"statefulSets"`
	DaemonSets       int `json:"daemonSets"`
	Pods             int `json:"pods"`
	Services         int `json:"services"`
	ConfigMaps       int `json:"configMaps"`
	Secrets          int `json:"secrets"`
	PVCs             int `json:"pvcs"`
	Ingresses        int `json:"ingresses"`
	NetworkPolicies  int `json:"networkPolicies"`
	ServiceAccounts  int `json:"serviceAccounts"`
	Roles            int `json:"roles"`
	ClusterRoles     int `json:"clusterRoles"`
	HPAs             int `json:"hpas"`
	PDBs             int `json:"pdbs"`
	StorageClasses   int `json:"storageClasses"`
	Nodes            int `json:"nodes"`
}

// KindInventory per-kind resource count and status.
type KindInventory struct {
	Kind       string `json:"kind"`
	Count      int    `json:"count"`
	Healthy    int    `json:"healthy"`
	Unhealthy  int    `json:"unhealthy"`
	AvgAgeDays int    `json:"avgAgeDays"`
}

// NSInventory per-namespace resource counts.
type NSInventory struct {
	Namespace    string `json:"namespace"`
	Workloads    int    `json:"workloads"`
	Pods         int    `json:"pods"`
	Services     int    `json:"services"`
	ConfigMaps   int    `json:"configMaps"`
	Secrets      int `json:"secrets"`
	IsSystem     bool   `json:"isSystem"`
	ResourcePct  float64 `json:"resourcePct"`
}

// InventoryHealth aggregates health status.
type InventoryHealth struct {
	HealthyResources   int `json:"healthyResources"`
	UnhealthyResources int `json:"unhealthyResources"`
	CrashLoopPods      int `json:"crashLoopPods"`
	PendingPods        int `json:"pendingPods"`
	FailedPods         int `json:"failedPods"`
	NotReadyNodes      int `json:"notReadyNodes"`
}

// AgeDistribution shows resource age distribution.
type AgeDistribution struct {
	New7d      int `json:"new7d"`
	WeekTo30d  int `json:"weekTo30d"`
	MonthTo90d int `json:"monthTo90d"`
	OldTo180d  int `json:"oldTo180d"`
	VeryOld    int `json:"veryOld180dPlus"`
}

// LabelsCoverage tracks label hygiene.
type LabelsCoverage struct {
	WithAppLabel      int     `json:"withAppLabel"`
	WithTeamLabel     int     `json:"withTeamLabel"`
	WithEnvLabel      int     `json:"withEnvLabel"`
	WithoutLabels     int     `json:"withoutLabels"`
	AppLabelPct       float64 `json:"appLabelPct"`
}

// OrphanedResource describes a resource with no owner.
type OrphanedResource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	AgeDays   int    `json:"ageDays"`
}

// handleResourceInventory handles GET /api/docs/resource-inventory
func (s *Server) handleResourceInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ResourceInventoryResult{ScannedAt: time.Now()}
	now := time.Now()

	// Fetch all resource types
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	sas, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	roles, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	clusterRoles, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	// Populate summary
	inv := InventorySummary{
		Namespaces:      len(namespaces.Items),
		Deployments:     len(deployments.Items),
		StatefulSets:    len(statefulsets.Items),
		DaemonSets:      len(daemonsets.Items),
		Pods:            len(pods.Items),
		Services:        len(services.Items),
		ConfigMaps:      len(configmaps.Items),
		Secrets:         len(secrets.Items),
		PVCs:            len(pvcs.Items),
		Ingresses:       len(ingresses.Items),
		NetworkPolicies: len(netpols.Items),
		ServiceAccounts: len(sas.Items),
		Roles:           len(roles.Items),
		ClusterRoles:    len(clusterRoles.Items),
		HPAs:            len(hpas.Items),
		PDBs:            len(pdbs.Items),
		StorageClasses:  len(scs.Items),
		Nodes:           len(nodes.Items),
	}
	inv.TotalResources = inv.Deployments + inv.StatefulSets + inv.DaemonSets + inv.Pods + inv.Services +
		inv.ConfigMaps + inv.Secrets + inv.PVCs + inv.Ingresses + inv.NetworkPolicies +
		inv.ServiceAccounts + inv.Roles + inv.HPAs + inv.PDBs + inv.Nodes
	result.Summary = inv

	// By-kind inventory with health
	result.ByKind = buildKindInventory(deployments.Items, statefulsets.Items, daemonsets.Items, pods.Items, services.Items, nodes.Items)

	// By-namespace inventory
	result.ByNamespace = buildNSInventory(namespaces.Items, deployments.Items, statefulsets.Items, daemonsets.Items, pods.Items, services.Items, configmaps.Items, secrets.Items, inv.TotalResources)

	// Health summary
	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodPending:
			result.HealthSummary.PendingPods++
		case corev1.PodFailed:
			result.HealthSummary.FailedPods++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.HealthSummary.CrashLoopPods++
			}
		}
	}
	result.HealthSummary.HealthyResources = inv.TotalResources - result.HealthSummary.UnhealthyResources

	for _, node := range nodes.Items {
		isReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if !isReady {
			result.HealthSummary.NotReadyNodes++
		}
	}

	// Age distribution from workloads
	collectAge := func(creationTime metav1.Time) {
		if creationTime.IsZero() {
			return
		}
		ageDays := int(now.Sub(creationTime.Time).Hours() / 24)
		switch {
		case ageDays <= 7:
			result.AgeDistribution.New7d++
		case ageDays <= 30:
			result.AgeDistribution.WeekTo30d++
		case ageDays <= 90:
			result.AgeDistribution.MonthTo90d++
		case ageDays <= 180:
			result.AgeDistribution.OldTo180d++
		default:
			result.AgeDistribution.VeryOld++
		}
	}

	for _, dep := range deployments.Items {
		collectAge(dep.CreationTimestamp)
		checkLabelCoverage(dep.Labels, &result.LabelsCoverage)
	}
	for _, sts := range statefulsets.Items {
		collectAge(sts.CreationTimestamp)
		checkLabelCoverage(sts.Labels, &result.LabelsCoverage)
	}

	totalLabeled := result.LabelsCoverage.WithAppLabel + result.LabelsCoverage.WithoutLabels
	if totalLabeled > 0 {
		result.LabelsCoverage.AppLabelPct = float64(result.LabelsCoverage.WithAppLabel) / float64(totalLabeled) * 100
	}

	// Orphaned resources
	result.Orphaned = findOrphanedResources(services.Items, pods.Items, configmaps.Items, pvcs.Items)

	// Compute score
	result.HealthScore = computeInventoryScore(inv, result.HealthSummary, result.LabelsCoverage, len(result.Orphaned))
	result.Grade = scoreToGrade(result.HealthScore)

	// Generate recommendations
	result.Recommendations = generateInventoryRecs(result)

	writeJSON(w, result)
}

// buildKindInventory creates per-kind inventory stats.
func buildKindInventory(deps []appsv1.Deployment, stss []appsv1.StatefulSet, dss []appsv1.DaemonSet, pods []corev1.Pod, svcs []corev1.Service, nodes []corev1.Node) []KindInventory {
	var result []KindInventory
	now := time.Now()

	addKind := func(kind string, count, healthy, unhealthy int, creationTime metav1.Time) {
		if count == 0 {
			return
		}
		age := 0
		if !creationTime.IsZero() {
			age = int(now.Sub(creationTime.Time).Hours() / 24)
		}
		result = append(result, KindInventory{
			Kind: kind, Count: count, Healthy: healthy, Unhealthy: unhealthy, AvgAgeDays: age,
		})
	}

	depHealthy := 0
	for _, d := range deps {
		if d.Status.ReadyReplicas == *d.Spec.Replicas {
			depHealthy++
		}
	}
	addKind("Deployment", len(deps), depHealthy, len(deps)-depHealthy, deps[0].CreationTimestamp)

	stsHealthy := 0
	for _, s := range stss {
		if s.Status.ReadyReplicas == *s.Spec.Replicas {
			stsHealthy++
		}
	}
	if len(stss) > 0 {
		addKind("StatefulSet", len(stss), stsHealthy, len(stss)-stsHealthy, stss[0].CreationTimestamp)
	}

	dsHealthy := 0
	for _, d := range dss {
		if d.Status.NumberReady == d.Status.DesiredNumberScheduled {
			dsHealthy++
		}
	}
	if len(dss) > 0 {
		addKind("DaemonSet", len(dss), dsHealthy, len(dss)-dsHealthy, dss[0].CreationTimestamp)
	}

	podHealthy := 0
	for _, p := range pods {
		if p.Status.Phase == corev1.PodRunning {
			podHealthy++
		}
	}
	addKind("Pod", len(pods), podHealthy, len(pods)-podHealthy, metav1.Time{})

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result
}

// buildNSInventory creates per-namespace inventory.
func buildNSInventory(namespaces []corev1.Namespace, deps []appsv1.Deployment, stss []appsv1.StatefulSet, dss []appsv1.DaemonSet, pods []corev1.Pod, svcs []corev1.Service, cms []corev1.ConfigMap, secrets []corev1.Secret, totalResources int) []NSInventory {
	nsMap := map[string]*NSInventory{}

	for _, ns := range namespaces {
		isSystem := isSystemNamespace(ns.Name)
		nsMap[ns.Name] = &NSInventory{Namespace: ns.Name, IsSystem: isSystem}
	}

	countWorkload := func(ns string) {
		if e, ok := nsMap[ns]; ok {
			e.Workloads++
		}
	}
	for _, d := range deps {
		countWorkload(d.Namespace)
	}
	for _, s := range stss {
		countWorkload(s.Namespace)
	}
	for _, d := range dss {
		countWorkload(d.Namespace)
	}

	for _, p := range pods {
		if e, ok := nsMap[p.Namespace]; ok {
			e.Pods++
		}
	}
	for _, svc := range svcs {
		if e, ok := nsMap[svc.Namespace]; ok {
			e.Services++
		}
	}
	for _, cm := range cms {
		if e, ok := nsMap[cm.Namespace]; ok {
			e.ConfigMaps++
		}
	}
	for _, sec := range secrets {
		if e, ok := nsMap[sec.Namespace]; ok {
			e.Secrets++
		}
	}

	var result []NSInventory
	for _, ns := range nsMap {
		nsResources := ns.Workloads + ns.Pods + ns.Services + ns.ConfigMaps + ns.Secrets
		if totalResources > 0 {
			ns.ResourcePct = float64(nsResources) / float64(totalResources) * 100
		}
		result = append(result, *ns)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Workloads > result[j].Workloads
	})
	return result
}

// checkLabelCoverage updates label coverage stats.
func checkLabelCoverage(labels map[string]string, cov *LabelsCoverage) {
	hasApp := false
	hasTeam := false
	hasEnv := false
	for k := range labels {
		kLower := strings.ToLower(k)
		if kLower == "app" || kLower == "app.kubernetes.io/name" {
			hasApp = true
		}
		if strings.Contains(kLower, "team") || strings.Contains(kLower, "owner") {
			hasTeam = true
		}
		if strings.Contains(kLower, "env") || strings.Contains(kLower, "environment") {
			hasEnv = true
		}
	}
	if hasApp {
		cov.WithAppLabel++
	} else {
		cov.WithoutLabels++
	}
	if hasTeam {
		cov.WithTeamLabel++
	}
	if hasEnv {
		cov.WithEnvLabel++
	}
}

// findOrphanedResources identifies resources without owners.
func findOrphanedResources(svcs []corev1.Service, pods []corev1.Pod, cms []corev1.ConfigMap, pvcs []corev1.PersistentVolumeClaim) []OrphanedResource {
	var orphaned []OrphanedResource
	now := time.Now()

	// Services without endpoints
	for _, svc := range svcs {
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		// Check if any pod matches the selector
		matched := false
		for _, pod := range pods {
			if pod.Namespace != svc.Namespace || len(svc.Spec.Selector) == 0 {
				continue
			}
			// Check if pod labels match service selector
			allMatch := true
			for k, v := range svc.Spec.Selector {
				if pod.Labels[k] != v {
					allMatch = false
					break
				}
			}
			if allMatch {
				matched = true
				break
			}
		}
		if !matched && len(svc.Spec.Selector) > 0 {
			age := 0
			if !svc.CreationTimestamp.IsZero() {
				age = int(now.Sub(svc.CreationTimestamp.Time).Hours() / 24)
			}
			orphaned = append(orphaned, OrphanedResource{
				Name: svc.Name, Namespace: svc.Namespace, Kind: "Service",
				Reason: "No pods match selector", AgeDays: age,
			})
		}
	}

	return orphaned
}

// computeInventoryScore computes a 0-100 inventory health score.
func computeInventoryScore(s InventorySummary, h InventoryHealth, l LabelsCoverage, orphanCount int) int {
	score := 100
	// Penalize crash loops (even with 0 total resources, these are critical)
	score -= minInt(h.CrashLoopPods*2, 15)
	// Penalize pending/failed pods
	if s.Pods > 0 {
		badRatio := float64(h.PendingPods+h.FailedPods) / float64(s.Pods)
		score -= int(badRatio * 15)
	}
	// Penalize not-ready nodes
	if s.Nodes > 0 {
		nodeBadRatio := float64(h.NotReadyNodes) / float64(s.Nodes)
		score -= int(nodeBadRatio * 20)
	}
	// Penalize orphaned resources
	score -= minInt(orphanCount, 10)
	// Penalize missing labels
	totalLabels := l.WithAppLabel + l.WithoutLabels
	if totalLabels > 0 {
		missingRatio := float64(l.WithoutLabels) / float64(totalLabels)
		score -= int(missingRatio * 10)
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateInventoryRecs produces recommendations.
func generateInventoryRecs(r ResourceInventoryResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Resource inventory: %d total resources across %d namespaces (score %d/100, grade %s)",
		r.Summary.TotalResources, r.Summary.Namespaces, r.HealthScore, r.Grade))

	recs = append(recs, fmt.Sprintf("Breakdown: %d Deployments, %d StatefulSets, %d DaemonSets, %d Pods, %d Services, %d PVCs, %d Ingresses",
		r.Summary.Deployments, r.Summary.StatefulSets, r.Summary.DaemonSets, r.Summary.Pods, r.Summary.Services, r.Summary.PVCs, r.Summary.Ingresses))

	if r.HealthSummary.CrashLoopPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) in CrashLoopBackOff — investigate immediately", r.HealthSummary.CrashLoopPods))
	}

	if r.HealthSummary.NotReadyNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) not ready — check node health", r.HealthSummary.NotReadyNodes))
	}

	if len(r.Orphaned) > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned resource(s) detected — clean up to reduce clutter", len(r.Orphaned)))
	}

	if r.LabelsCoverage.AppLabelPct < 80 {
		recs = append(recs, fmt.Sprintf("Only %.0f%% of workloads have app labels — improve label hygiene", r.LabelsCoverage.AppLabelPct))
	}

	if r.AgeDistribution.VeryOld > 10 {
		recs = append(recs, fmt.Sprintf("%d resources older than 180 days — review for relevance and cleanup", r.AgeDistribution.VeryOld))
	}

	return recs
}

// Reference types to avoid unused import
var _ networkingv1.Ingress
