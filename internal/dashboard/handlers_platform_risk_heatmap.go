package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformRiskHeatmapResult generates a multi-dimensional risk heatmap across the cluster.
type PlatformRiskHeatmapResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         PlatformHeatmapSummary `json:"summary"`
	ByNamespace     []HeatmapNsEntry       `json:"byNamespace"`
	TopRisks        []HeatmapRiskEntry     `json:"topRisks"`
	OverallScore    int                    `json:"overallScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type PlatformHeatmapSummary struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	CriticalNS       int `json:"criticalNamespaces"`
	HighRiskNS       int `json:"highRiskNamespaces"`
	MediumRiskNS     int `json:"mediumRiskNamespaces"`
	LowRiskNS        int `json:"lowRiskNamespaces"`
	TotalRiskFactors int `json:"totalRiskFactors"`
}

type HeatmapNsEntry struct {
	Namespace string         `json:"namespace"`
	RiskScore int            `json:"riskScore"`
	RiskLevel string         `json:"riskLevel"`
	PodCount  int            `json:"podCount"`
	Factors   map[string]int `json:"factors"`
}

type HeatmapRiskEntry struct {
	Namespace string `json:"namespace"`
	Factor    string `json:"factor"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// handlePlatformRiskHeatmap handles GET /api/docs/platform-risk-heatmap
func (s *Server) handlePlatformRiskHeatmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PlatformRiskHeatmapResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Build namespace risk scores
	nsData := make(map[string]*HeatmapNsEntry)
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		nsData[ns.Name] = &HeatmapNsEntry{
			Namespace: ns.Name,
			Factors:   make(map[string]int),
		}
		result.Summary.TotalNamespaces++
	}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		entry, ok := nsData[pod.Namespace]
		if !ok {
			entry = &HeatmapNsEntry{Namespace: pod.Namespace, Factors: make(map[string]int)}
			nsData[pod.Namespace] = entry
			result.Summary.TotalNamespaces++
		}
		entry.PodCount++

		// Risk factor: pods not ready
		if pod.Status.Phase != corev1.PodRunning {
			entry.Factors["pods-not-running"]++
			result.Summary.TotalRiskFactors++
		}

		// Risk factor: high restart count
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 {
				entry.Factors["high-restart"]++
				result.Summary.TotalRiskFactors++
			}
			if cs.RestartCount > 0 {
				entry.Factors["any-restart"]++
			}
		}

		// Risk factor: no resource limits
		hasLimit := false
		for _, c := range pod.Spec.Containers {
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				hasLimit = true
			}
		}
		if !hasLimit && len(pod.Spec.Containers) > 0 {
			entry.Factors["no-resource-limits"]++
			result.Summary.TotalRiskFactors++
		}

		// Risk factor: runs as root
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				entry.Factors["runs-as-root"]++
				break
			}
		}

		// Risk factor: no readiness probe
		for _, c := range pod.Spec.Containers {
			if c.ReadinessProbe == nil && len(pod.Spec.Containers) > 0 {
				entry.Factors["no-readiness-probe"]++
				break
			}
		}
	}

	// PVC without storage class check
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		entry, ok := nsData[pvc.Namespace]
		if ok && pvc.Status.Phase != corev1.ClaimBound {
			entry.Factors["unbound-pvc"]++
			result.Summary.TotalRiskFactors++
		}
	}

	// Calculate risk scores
	var allEntries []HeatmapNsEntry
	for _, entry := range nsData {
		score := 0
		for factor, count := range entry.Factors {
			weight := 1
			switch factor {
			case "pods-not-running":
				weight = 15
			case "high-restart":
				weight = 10
			case "unbound-pvc":
				weight = 8
			case "no-resource-limits":
				weight = 5
			case "runs-as-root":
				weight = 5
			case "no-readiness-probe":
				weight = 3
			}
			score += count * weight

			if weight >= 8 && count > 0 {
				severity := "medium"
				if weight >= 10 {
					severity = "high"
				}
				if weight >= 15 {
					severity = "critical"
				}
				result.TopRisks = append(result.TopRisks, HeatmapRiskEntry{
					Namespace: entry.Namespace,
					Factor:    factor,
					Severity:  severity,
					Detail:    fmt.Sprintf("%d occurrences", count),
				})
			}
		}
		entry.RiskScore = score
		if entry.PodCount > 0 {
			entry.RiskScore = score / entry.PodCount
			if entry.RiskScore > 100 {
				entry.RiskScore = 100
			}
		}

		switch {
		case entry.RiskScore >= 50:
			entry.RiskLevel = "critical"
			result.Summary.CriticalNS++
		case entry.RiskScore >= 30:
			entry.RiskLevel = "high"
			result.Summary.HighRiskNS++
		case entry.RiskScore >= 15:
			entry.RiskLevel = "medium"
			result.Summary.MediumRiskNS++
		default:
			entry.RiskLevel = "low"
			result.Summary.LowRiskNS++
		}

		allEntries = append(allEntries, *entry)
	}

	// Sort by risk score descending
	for i := 0; i < len(allEntries); i++ {
		for j := i + 1; j < len(allEntries); j++ {
			if allEntries[j].RiskScore > allEntries[i].RiskScore {
				allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
			}
		}
	}
	result.ByNamespace = allEntries

	if result.Summary.TotalNamespaces > 0 {
		cleanNS := result.Summary.LowRiskNS + result.Summary.MediumRiskNS
		result.OverallScore = cleanNS * 100 / result.Summary.TotalNamespaces
	}
	gradeFromScore(&result.Grade, result.OverallScore)

	result.Recommendations = []string{
		fmt.Sprintf("风险热力图: %d 命名空间, %d 严重, %d 高风险, %d 中风险, %d 低风险",
			result.Summary.TotalNamespaces, result.Summary.CriticalNS, result.Summary.HighRiskNS,
			result.Summary.MediumRiskNS, result.Summary.LowRiskNS),
	}
	if len(result.TopRisks) > 0 {
		limit := minInt1872(len(result.TopRisks), 3)
		for _, risk := range result.TopRisks[:limit] {
			result.Recommendations = append(result.Recommendations,
				fmt.Sprintf("[%s] %s/%s: %s", risk.Severity, risk.Namespace, risk.Factor, risk.Detail))
		}
	}
	writeJSON(w, result)
}

func minInt1872(a, b int) int {
	if a < b {
		return a
	}
	return b
}
