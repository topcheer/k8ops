package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// SVCResult is the service endpoint & connectivity health analysis.
type SVCResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         SVCSummary         `json:"summary"`
	Unhealthy       []SVCEntry         `json:"unhealthyServices"`
	ByType          []SVCTypeStat      `json:"byType"`
	ByNamespace     []SVCNamespaceStat `json:"byNamespace"`
	SelectorGaps    []SVCEntry         `json:"selectorGaps"` // services whose selector matches no pods
	Recommendations []string           `json:"recommendations"`
}

// SVCSummary aggregates service health across the cluster.
type SVCSummary struct {
	TotalServices     int `json:"totalServices"`
	HealthyServices   int `json:"healthyServices"`
	UnhealthyServices int `json:"unhealthyServices"`
	ZeroEndpoints     int `json:"zeroEndpoints"`     // no endpoints at all
	NotReadyEndpoints int `json:"notReadyEndpoints"` // has endpoints but none ready
	ClusterIP         int `json:"clusterIP"`
	NodePort          int `json:"nodePort"`
	LoadBalancer      int `json:"loadBalancer"`
	ExternalName      int `json:"externalName"`
	Headless          int `json:"headless"`
	HealthScore       int `json:"healthScore"`
}

// SVCEntry describes a single unhealthy service.
type SVCEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Type           string `json:"type"`
	Selector       string `json:"selector"`
	EndpointCount  int    `json:"endpointCount"`
	ReadyEndpoints int    `json:"readyEndpoints"`
	Issue          string `json:"issue"`
	Severity       string `json:"severity"`
}

// SVCTypeStat shows service count by type.
type SVCTypeStat struct {
	Type    string `json:"type"`
	Count   int    `json:"count"`
	Healthy int    `json:"healthy"`
}

// SVCNamespaceStat shows service health per namespace.
type SVCNamespaceStat struct {
	Namespace string `json:"namespace"`
	Total     int    `json:"total"`
	Healthy   int    `json:"healthy"`
	Unhealthy int    `json:"unhealthy"`
	IsSystem  bool   `json:"isSystem"`
}

// handleServiceConnectivity analyzes Service endpoint health and connectivity.
// GET /api/product/service-connectivity
func (s *Server) handleServiceConnectivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build endpoint map: ns/name -> ready count, total count
	type epInfo struct {
		ready int
		total int
	}
	epMap := map[string]*epInfo{}
	for _, ep := range endpoints.Items {
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		info := &epInfo{}
		for _, subset := range ep.Subsets {
			info.total += len(subset.Addresses) + len(subset.NotReadyAddresses)
			info.ready += len(subset.Addresses)
		}
		epMap[key] = info
	}

	// Build pod map for selector matching: ns -> []pod
	podsByNs := map[string][]corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod)
	}

	now := time.Now()
	result := SVCResult{ScannedAt: now}
	result.Summary.TotalServices = len(services.Items)

	typeStats := map[string]*SVCTypeStat{}
	nsStats := map[string]*SVCNamespaceStat{}

	for _, svc := range services.Items {
		svcType := string(svc.Spec.Type)
		if svc.Spec.ClusterIP == "None" {
			svcType = "Headless"
		}

		// Update type stats
		ts, ok := typeStats[svcType]
		if !ok {
			ts = &SVCTypeStat{Type: svcType}
			typeStats[svcType] = ts
		}
		ts.Count++

		// Update namespace stats
		isSys := isSystemNamespace(svc.Namespace)
		nsStat, ok := nsStats[svc.Namespace]
		if !ok {
			nsStat = &SVCNamespaceStat{Namespace: svc.Namespace, IsSystem: isSys}
			nsStats[svc.Namespace] = nsStat
		}
		nsStat.Total++

		// Count type in summary
		switch svcType {
		case "ClusterIP":
			result.Summary.ClusterIP++
		case "NodePort":
			result.Summary.NodePort++
		case "LoadBalancer":
			result.Summary.LoadBalancer++
		case "ExternalName":
			result.Summary.ExternalName++
		case "Headless":
			result.Summary.Headless++
		}

		// Skip ExternalName services — they don't have endpoints
		if svcType == "ExternalName" {
			result.Summary.HealthyServices++
			ts.Healthy++
			nsStat.Healthy++
			continue
		}

		// Check endpoint health
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		ep, hasEP := epMap[key]

		entry := SVCEntry{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      svcType,
			Selector:  formatSelector(svc.Spec.Selector),
		}

		if hasEP {
			entry.EndpointCount = ep.total
			entry.ReadyEndpoints = ep.ready
		}

		healthy := true
		severity := "low"
		issue := ""

		if !hasEP || ep.total == 0 {
			healthy = false
			result.Summary.ZeroEndpoints++
			severity = "high"
			issue = "Service has zero endpoints — no backing pods"

			// Check if selector matches any pods
			if len(svc.Spec.Selector) > 0 {
				matchingPods := countMatchingPods(podsByNs[svc.Namespace], svc.Spec.Selector)
				if matchingPods == 0 {
					issue = fmt.Sprintf("Selector %s matches no pods in namespace", entry.Selector)
					severity = "critical"
					result.SelectorGaps = append(result.SelectorGaps, entry)
				}
			}
		} else if ep.ready == 0 {
			healthy = false
			result.Summary.NotReadyEndpoints++
			severity = "high"
			issue = fmt.Sprintf("Service has %d endpoints but none are ready", ep.total)
		} else if ep.ready < ep.total {
			// Partial health — some endpoints not ready
			severity = "medium"
			issue = fmt.Sprintf("Service has %d/%d endpoints ready", ep.ready, ep.total)
		}

		entry.Issue = issue
		entry.Severity = severity

		if healthy {
			result.Summary.HealthyServices++
			ts.Healthy++
			nsStat.Healthy++
		} else {
			result.Summary.UnhealthyServices++
			nsStat.Unhealthy++
			result.Unhealthy = append(result.Unhealthy, entry)
		}
	}

	// Build type stats
	for _, ts := range typeStats {
		result.ByType = append(result.ByType, *ts)
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].IsSystem != result.ByNamespace[j].IsSystem {
			return !result.ByNamespace[i].IsSystem
		}
		return result.ByNamespace[i].Unhealthy > result.ByNamespace[j].Unhealthy
	})

	// Sort unhealthy by severity
	sort.Slice(result.Unhealthy, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Unhealthy[i].Severity] < sevOrder[result.Unhealthy[j].Severity]
	})
	if len(result.Unhealthy) > 50 {
		result.Unhealthy = result.Unhealthy[:50]
	}
	if len(result.SelectorGaps) > 30 {
		result.SelectorGaps = result.SelectorGaps[:30]
	}

	result.Summary.HealthScore = svcHealthScore(result.Summary)
	result.Recommendations = svcRecommendations(&result)

	writeJSON(w, result)
}

// formatSelector converts a label selector to a readable string.
func formatSelector(selector map[string]string) string {
	if len(selector) == 0 {
		return "<none>"
	}
	var parts []string
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, selector[k]))
	}
	return strings.Join(parts, ",")
}

// countMatchingPods counts pods matching the given selector labels.
func countMatchingPods(pods []corev1.Pod, selector map[string]string) int {
	count := 0
	for _, pod := range pods {
		matches := true
		for k, v := range selector {
			if pod.Labels == nil || pod.Labels[k] != v {
				matches = false
				break
			}
		}
		if matches {
			count++
		}
	}
	return count
}

// svcHealthScore computes a 0-100 health score.
func svcHealthScore(s SVCSummary) int {
	if s.TotalServices == 0 {
		return 100
	}
	healthyRatio := float64(s.HealthyServices) / float64(s.TotalServices)
	score := int(healthyRatio * 100)

	// Extra penalty for critical issues
	if s.ZeroEndpoints > 0 {
		ratio := float64(s.ZeroEndpoints) / float64(s.TotalServices)
		score -= int(ratio * 10) // additional penalty
	}

	if score < 0 {
		score = 0
	}
	return score
}

// svcRecommendations generates actionable recommendations.
func svcRecommendations(r *SVCResult) []string {
	var recs []string

	if r.Summary.ZeroEndpoints > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d service(s) have zero endpoints — check pod readiness, selector labels, and controller health",
			r.Summary.ZeroEndpoints,
		))
	}

	if r.Summary.NotReadyEndpoints > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d service(s) have endpoints but none are ready — investigate pod readiness probes and startup issues",
			r.Summary.NotReadyEndpoints,
		))
	}

	if len(r.SelectorGaps) > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d service(s) have selectors that match no pods — verify label selectors match pod labels",
			len(r.SelectorGaps),
		))
	}

	if r.Summary.LoadBalancer > 0 && r.Summary.HealthyServices == 0 {
		recs = append(recs, "LoadBalancer services are unhealthy — check cloud provider integration and external traffic policies")
	}

	if len(recs) == 0 {
		recs = append(recs, "All services have healthy endpoints — service connectivity is in good shape")
	}

	return recs
}

// Ensure intstr import is used.
var _ = intstr.FromInt
