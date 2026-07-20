package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APICoverageGapResult analyzes which Kubernetes resource types are
// underrepresented in API coverage, identifying blind spots in observability.
type APICoverageGapResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         APICoverageGapSummary `json:"summary"`
	ByResource      []APICoverageEntry    `json:"byResource"`
	BlindSpots      []APICoverageEntry    `json:"blindSpots"`
	CoverageScore   int                   `json:"coverageScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type APICoverageGapSummary struct {
	TotalResourceTypes int `json:"totalResourceTypes"`
	ObservedTypes      int `json:"observedTypes"`
	UnobservedTypes    int `json:"unobservedTypes"`
	CriticalGaps       int `json:"criticalGaps"`
	TotalResources     int `json:"totalResources"`
}

type APICoverageEntry struct {
	ResourceType string `json:"resourceType"`
	Group        string `json:"group"`
	Version      string `json:"version"`
	Count        int    `json:"count"`
	HasCoverage  bool   `json:"hasCoverage"`
	GapLevel     string `json:"gapLevel"`
	Impact       string `json:"impact"`
}

// handleAPICoverageGap handles GET /api/docs/api-coverage-gap
func (s *Server) handleAPICoverageGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APICoverageGapResult{ScannedAt: time.Now()}

	// Count resources by type
	resourceTypes := make(map[string]*APICoverageEntry)

	addType := func(rtype, group, version string, count int) {
		key := group + "/" + version + "/" + rtype
		if _, ok := resourceTypes[key]; !ok {
			resourceTypes[key] = &APICoverageEntry{
				ResourceType: rtype,
				Group:        group,
				Version:      version,
			}
		}
		resourceTypes[key].Count += count
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	addType("Pod", "core", "v1", len(pods.Items))

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	addType("Service", "core", "v1", len(services.Items))

	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	addType("ConfigMap", "core", "v1", len(cms.Items))

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	addType("Secret", "core", "v1", len(secrets.Items))

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	addType("Namespace", "core", "v1", len(namespaces.Items))

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	addType("Node", "core", "v1", len(nodes.Items))

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	addType("PVC", "core", "v1", len(pvcs.Items))

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	addType("Deployment", "apps", "v1", len(deployments.Items))

	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	addType("StatefulSet", "apps", "v1", len(sts.Items))

	ds, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	addType("DaemonSet", "apps", "v1", len(ds.Items))

	rss, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	addType("ReplicaSet", "apps", "v1", len(rss.Items))

	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	addType("HPA", "autoscaling", "v2", len(hpas.Items))

	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	addType("PDB", "policy", "v1", len(pdbs.Items))

	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	addType("Ingress", "networking.k8s.io", "v1", len(ingresses.Items))

	// Define which types have API coverage in our platform
	coveredTypes := map[string]bool{
		"core/v1/Pod":                  true,
		"core/v1/Service":              true,
		"core/v1/Node":                 true,
		"apps/v1/Deployment":           true,
		"apps/v1/StatefulSet":          true,
		"apps/v1/DaemonSet":            true,
		"policy/v1/PDB":                true,
		"networking.k8s.io/v1/Ingress": true,
	}

	// Critical types that should always have coverage
	criticalTypes := map[string]bool{
		"core/v1/Secret":     true,
		"autoscaling/v2/HPA": true,
		"core/v1/PVC":        true,
		"core/v1/ConfigMap":  true,
		"core/v1/Namespace":  true,
	}

	var entries []APICoverageEntry
	totalResources := 0
	for _, e := range resourceTypes {
		key := e.Group + "/" + e.Version + "/" + e.ResourceType
		result.Summary.TotalResourceTypes++
		totalResources += e.Count

		if coveredTypes[key] {
			e.HasCoverage = true
			e.GapLevel = "none"
			result.Summary.ObservedTypes++
		} else {
			e.HasCoverage = false
			result.Summary.UnobservedTypes++
			if criticalTypes[key] {
				e.GapLevel = "critical"
				e.Impact = fmt.Sprintf("%d %s resources lack dedicated monitoring", e.Count, e.ResourceType)
				result.Summary.CriticalGaps++
				result.BlindSpots = append(result.BlindSpots, *e)
			} else {
				e.GapLevel = "medium"
				e.Impact = fmt.Sprintf("%d resources, no dedicated API", e.Count)
			}
		}

		entries = append(entries, *e)
	}

	result.Summary.TotalResources = totalResources

	sort.Slice(entries, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "medium": 1, "none": 2}
		return rank[entries[i].GapLevel] < rank[entries[j].GapLevel]
	})
	result.ByResource = entries

	if result.Summary.TotalResourceTypes > 0 {
		result.CoverageScore = result.Summary.ObservedTypes * 100 / result.Summary.TotalResourceTypes
	}
	gradeFromScore(&result.Grade, result.CoverageScore)

	result.Recommendations = []string{
		fmt.Sprintf("API 覆盖: %d/%d 资源类型有监控 (%d%%), %d 关键缺口", result.Summary.ObservedTypes, result.Summary.TotalResourceTypes, result.CoverageScore, result.Summary.CriticalGaps),
		fmt.Sprintf("总资源数: %d, 覆盖类型: %d", totalResources, result.Summary.ObservedTypes),
	}
	if result.Summary.CriticalGaps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个关键资源类型缺少监控 (Secret, PVC, HPA 等)", result.Summary.CriticalGaps))
	}
	if result.CoverageScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 为 Secret/PVC/HPA/ConfigMap 添加专门的审计 API")
	}

	_ = corev1.Pod{} // keep import
	writeJSON(w, result)
}
