package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.28 — Scalability & HA Dimension (Round 7 Final)
// 1. Pod Restart Rate Limiter — anti-flapping analysis
// 2. Node Affinity Compliance — scheduling constraint health
// 3. Resource Quota Pressure Index — quota exhaustion forecasting
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Restart Rate Limiter — anti-flapping analysis
// ---------------------------------------------------------------

type RestartRateResult1928 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         RestartRateSummary1928 `json:"summary"`
	Pods            []RestartRateEntry1928 `json:"pods"`
	FlappingPods    []RestartFlapEntry1928 `json:"flappingPods"`
	Recommendations []string               `json:"recommendations"`
}

type RestartRateSummary1928 struct {
	TotalPods      int     `json:"totalPods"`
	WithRestarts   int     `json:"withRestarts"`
	TotalRestarts  int     `json:"totalRestarts"`
	MaxRestarts    int     `json:"maxRestarts"`
	FlappingCount  int     `json:"flappingCount"`
	AvgRestarts    float64 `json:"avgRestarts"`
	CrashLoopCount int     `json:"crashLoopCount"`
}

type RestartRateEntry1928 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Restarts    int    `json:"restarts"`
	Age         string `json:"age"`
	RestartRate string `json:"restartRate"` // restarts per day
	Status      string `json:"status"`
}

type RestartFlapEntry1928 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Restarts    int    `json:"restarts"`
	RestartRate string `json:"restartRate"`
	Reason      string `json:"reason"`
	Severity    string `json:"severity"`
}

func (s *Server) handleRestartRate(w http.ResponseWriter, r *http.Request) {
	result := RestartRateResult1928{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	var totalRestarts int
	var maxRestarts int

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		restartCount := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restartCount += int(cs.RestartCount)
		}
		if restartCount == 0 {
			continue
		}

		ageHours := time.Since(pod.CreationTimestamp.Time).Hours()
		if ageHours < 1 {
			ageHours = 1
		}
		restartRate := float64(restartCount) / (ageHours / 24)

		status := "stable"
		severity := "low"
		reason := ""
		if restartRate > 10 {
			status = "crash-loop"
			severity = "critical"
			reason = fmt.Sprintf("%.1f restarts/day — crash loop detected", restartRate)
			result.Summary.CrashLoopCount++
			score -= 10
		} else if restartRate > 5 {
			status = "flapping"
			severity = "high"
			reason = fmt.Sprintf("%.1f restarts/day — pod is flapping", restartRate)
			result.Summary.FlappingCount++
			score -= 5
		} else if restartRate > 1 {
			status = "unstable"
			severity = "medium"
			reason = fmt.Sprintf("%.1f restarts/day — elevated restart rate", restartRate)
			score -= 2
		}

		entry := RestartRateEntry1928{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Restarts:    restartCount,
			Age:         fmt.Sprintf("%.0fd", ageHours/24),
			RestartRate: fmt.Sprintf("%.1f/day", restartRate),
			Status:      status,
		}
		result.Pods = append(result.Pods, entry)
		result.Summary.WithRestarts++
		totalRestarts += restartCount
		if restartCount > maxRestarts {
			maxRestarts = restartCount
		}

		if severity != "low" {
			result.FlappingPods = append(result.FlappingPods, RestartFlapEntry1928{
				Name:        pod.Name,
				Namespace:   pod.Namespace,
				Restarts:    restartCount,
				RestartRate: fmt.Sprintf("%.1f/day", restartRate),
				Reason:      reason,
				Severity:    severity,
			})
		}
	}

	result.Summary.TotalRestarts = totalRestarts
	result.Summary.MaxRestarts = maxRestarts
	if result.Summary.WithRestarts > 0 {
		result.Summary.AvgRestarts = float64(totalRestarts) / float64(result.Summary.WithRestarts)
	}

	sort.Slice(result.Pods, func(i, j int) bool {
		return result.Pods[i].Restarts > result.Pods[j].Restarts
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.CrashLoopCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods in crash loop — investigate application logs immediately", result.Summary.CrashLoopCount))
	}
	if result.Summary.FlappingCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d flapping pods (>5 restarts/day) — check resource limits and health probes", result.Summary.FlappingCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Affinity Compliance — scheduling constraint health
// ---------------------------------------------------------------

type NodeAffinityResult1928 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         NodeAffinitySummary1928     `json:"summary"`
	Constraints     []NodeAffinityEntry1928     `json:"constraints"`
	Violations      []NodeAffinityViolation1928 `json:"violations"`
	NodeLabels      []NodeLabelStat1928         `json:"nodeLabels"`
	Recommendations []string                    `json:"recommendations"`
}

type NodeAffinitySummary1928 struct {
	TotalPods         int `json:"totalPods"`
	WithNodeSelector  int `json:"withNodeSelector"`
	WithNodeAffinity  int `json:"withNodeAffinity"`
	WithNodeAntiAff   int `json:"withNodeAntiAffinity"`
	Violations        int `json:"violations"`
	UnschedulableRisk int `json:"unschedulableRisk"`
	TotalNodes        int `json:"totalNodes"`
	UniqueNodeLabels  int `json:"uniqueNodeLabels"`
}

type NodeAffinityEntry1928 struct {
	PodName      string            `json:"podName"`
	Namespace    string            `json:"namespace"`
	NodeSelector map[string]string `json:"nodeSelector"`
	HasAffinity  bool              `json:"hasAffinity"`
	HasAntiAff   bool              `json:"hasAntiAffinity"`
	NodeName     string            `json:"nodeName"`
}

type NodeAffinityViolation1928 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

type NodeLabelStat1928 struct {
	Label string `json:"label"`
	Count int    `json:"nodeCount"`
}

func (s *Server) handleNodeAffinityCompliance(w http.ResponseWriter, r *http.Request) {
	result := NodeAffinityResult1928{
		ScannedAt: time.Now(),
	}
	score := 100

	// Collect node labels
	nodeList, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	result.Summary.TotalNodes = len(nodeList.Items)
	labelStats := make(map[string]int)
	for _, node := range nodeList.Items {
		for k := range node.Labels {
			labelStats[k]++
		}
	}
	for label, count := range labelStats {
		result.NodeLabels = append(result.NodeLabels, NodeLabelStat1928{Label: label, Count: count})
	}
	result.Summary.UniqueNodeLabels = len(labelStats)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Build label availability map
	nodeLabelMap := make(map[string]map[string]bool)
	for _, node := range nodeList.Items {
		nodeLabelMap[node.Name] = make(map[string]bool)
		for k, v := range node.Labels {
			nodeLabelMap[node.Name][k+"="+v] = true
		}
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasSelector := len(pod.Spec.NodeSelector) > 0
		hasAffinity := pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil
		hasAntiAff := pod.Spec.Affinity != nil && pod.Spec.Affinity.PodAntiAffinity != nil

		if !hasSelector && !hasAffinity && !hasAntiAff {
			continue
		}

		entry := NodeAffinityEntry1928{
			PodName:      pod.Name,
			Namespace:    pod.Namespace,
			NodeSelector: pod.Spec.NodeSelector,
			HasAffinity:  hasAffinity,
			HasAntiAff:   hasAntiAff,
			NodeName:     pod.Spec.NodeName,
		}
		result.Constraints = append(result.Constraints, entry)

		if hasSelector {
			result.Summary.WithNodeSelector++
		}
		if hasAffinity {
			result.Summary.WithNodeAffinity++
		}
		if hasAntiAff {
			result.Summary.WithNodeAntiAff++
		}

		// Check if node selector labels match any node
		if hasSelector {
			matchingNodes := 0
			for _, node := range nodeList.Items {
				match := true
				for k, v := range pod.Spec.NodeSelector {
					if node.Labels[k] != v {
						match = false
						break
					}
				}
				if match {
					matchingNodes++
				}
			}
			if matchingNodes == 0 {
				result.Violations = append(result.Violations, NodeAffinityViolation1928{
					PodName: pod.Name, Namespace: pod.Namespace,
					Violation: "Node selector matches 0 nodes — unschedulable if pod restarts",
					Severity:  "critical",
				})
				result.Summary.Violations++
				result.Summary.UnschedulableRisk++
				score -= 10
			} else if matchingNodes == 1 {
				result.Violations = append(result.Violations, NodeAffinityViolation1928{
					PodName: pod.Name, Namespace: pod.Namespace,
					Violation: "Node selector matches only 1 node — single point of failure",
					Severity:  "medium",
				})
				result.Summary.Violations++
				score -= 3
			}
		}

		// Check for overly strict required affinity
		if hasAffinity && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
			if len(terms) > 2 {
				result.Violations = append(result.Violations, NodeAffinityViolation1928{
					PodName: pod.Name, Namespace: pod.Namespace,
					Violation: fmt.Sprintf("%d required affinity terms — overly restrictive", len(terms)),
					Severity:  "low",
				})
			}
		}
	}

	sort.Slice(result.NodeLabels, func(i, j int) bool {
		return result.NodeLabels[i].Count > result.NodeLabels[j].Count
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.UnschedulableRisk > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with unschedulable node selectors — add matching node labels", result.Summary.UnschedulableRisk))
	}
	if result.Summary.Violations > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d affinity constraint violations — relax or add nodes", result.Summary.Violations))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Resource Quota Pressure Index — quota exhaustion forecasting
// ---------------------------------------------------------------

type QuotaPressureResult1928 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         QuotaPressureSummary1928 `json:"summary"`
	Namespaces      []QuotaPressureEntry1928 `json:"namespaces"`
	CriticalNS      []QuotaCriticalEntry1928 `json:"criticalNamespaces"`
	Recommendations []string                 `json:"recommendations"`
}

type QuotaPressureSummary1928 struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	WithQuota       int     `json:"withQuota"`
	WithoutQuota    int     `json:"withoutQuota"`
	CriticalCount   int     `json:"criticalCount"`
	MaxUtilization  float64 `json:"maxUtilization"`
	AvgUtilization  float64 `json:"avgUtilization"`
	TotalQuotas     int     `json:"totalQuotas"`
}

type QuotaPressureEntry1928 struct {
	Namespace      string              `json:"namespace"`
	QuotaName      string              `json:"quotaName"`
	Resources      []QuotaResource1928 `json:"resources"`
	MaxUtilization float64             `json:"maxUtilization"`
}

type QuotaResource1928 struct {
	Name        string  `json:"name"`
	Hard        string  `json:"hard"`
	Used        string  `json:"used"`
	Utilization float64 `json:"utilization"`
	Status      string  `json:"status"`
}

type QuotaCriticalEntry1928 struct {
	Namespace   string  `json:"namespace"`
	Resource    string  `json:"resource"`
	Used        string  `json:"used"`
	Hard        string  `json:"hard"`
	Utilization float64 `json:"utilization"`
}

func (s *Server) handleQuotaPressure(w http.ResponseWriter, r *http.Request) {
	result := QuotaPressureResult1928{
		ScannedAt: time.Now(),
	}
	score := 100

	nsList, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	rqList, err := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Group quotas by namespace
	nsQuotas := make(map[string][]corev1.ResourceQuota)
	for _, rq := range rqList.Items {
		nsQuotas[rq.Namespace] = append(nsQuotas[rq.Namespace], rq)
	}

	var totalUtil float64
	var utilCount int

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		quotas, hasQuota := nsQuotas[ns.Name]
		if !hasQuota {
			result.Summary.WithoutQuota++
			continue
		}
		result.Summary.WithQuota++
		result.Summary.TotalQuotas += len(quotas)

		for _, rq := range quotas {
			resources := make([]QuotaResource1928, 0)
			maxUtil := 0.0

			for resName, hardQty := range rq.Status.Hard {
				usedQty, hasUsed := rq.Status.Used[resName]
				if !hasUsed {
					continue
				}

				util := 0.0
				if !hardQty.IsZero() {
					hardVal := hardQty.AsApproximateFloat64()
					usedVal := usedQty.AsApproximateFloat64()
					if hardVal > 0 {
						util = usedVal / hardVal * 100
					}
				}

				status := "healthy"
				if util >= 90 {
					status = "critical"
					result.CriticalNS = append(result.CriticalNS, QuotaCriticalEntry1928{
						Namespace: ns.Name, Resource: string(resName),
						Used: usedQty.String(), Hard: hardQty.String(),
						Utilization: util,
					})
					result.Summary.CriticalCount++
					score -= 5
				} else if util >= 75 {
					status = "warning"
					score -= 1
				}

				resources = append(resources, QuotaResource1928{
					Name:        string(resName),
					Hard:        hardQty.String(),
					Used:        usedQty.String(),
					Utilization: util,
					Status:      status,
				})

				if util > maxUtil {
					maxUtil = util
				}
				totalUtil += util
				utilCount++
			}

			result.Namespaces = append(result.Namespaces, QuotaPressureEntry1928{
				Namespace:      ns.Name,
				QuotaName:      rq.Name,
				Resources:      resources,
				MaxUtilization: maxUtil,
			})

			if maxUtil > result.Summary.MaxUtilization {
				result.Summary.MaxUtilization = maxUtil
			}
		}
	}

	if utilCount > 0 {
		result.Summary.AvgUtilization = totalUtil / float64(utilCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.CriticalCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d quotas at >90%% utilization — increase limits or reduce usage", result.Summary.CriticalCount))
	}
	if result.Summary.WithoutQuota > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without ResourceQuota — add for resource governance", result.Summary.WithoutQuota))
	}
	if result.Summary.MaxUtilization > 80 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Max quota utilization %.0f%% — plan capacity expansion", result.Summary.MaxUtilization))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
