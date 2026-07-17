package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformInsightsResult provides a unified executive summary of the
// entire platform health by aggregating key metrics from multiple audit
// endpoints. It's designed for dashboards and status pages.
type PlatformInsightsResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	OverallScore    int               `json:"overallScore"`
	Grade           string            `json:"grade"`
	Categories      []InsightCategory `json:"categories"`
	CriticalAlerts  []InsightAlert    `json:"criticalAlerts"`
	TrendIndicators []TrendIndicator  `json:"trends"`
	QuickStats      map[string]int    `json:"quickStats"`
	Recommendations []string          `json:"recommendations"`
}

type InsightCategory struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type InsightAlert struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action"`
}

type TrendIndicator struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
	Trend  string `json:"trend"`
}

// handlePlatformInsights handles GET /api/docs/platform-insights
func (s *Server) handlePlatformInsights(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PlatformInsightsResult{
		ScannedAt:  time.Now(),
		QuickStats: make(map[string]int),
	}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// Quick stats
	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}
	result.QuickStats["nodes"] = len(nodes.Items)
	result.QuickStats["namespaces"] = nsCount
	result.QuickStats["deployments"] = len(deployments.Items)
	result.QuickStats["pods"] = len(pods.Items)
	result.QuickStats["secrets"] = len(secrets.Items)
	result.QuickStats["hpas"] = len(hpas.Items)
	result.QuickStats["pdbs"] = len(pdbs.Items)
	result.QuickStats["quotas"] = len(quotas.Items)
	result.QuickStats["netpols"] = len(netpols.Items)

	var categories []InsightCategory

	// 1. Availability
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	availScore := 100
	if workerCount < 2 {
		availScore = 20
	}
	categories = append(categories, InsightCategory{
		Name: "Availability", Score: availScore, Status: insightStatus(availScore),
		Detail: fmt.Sprintf("%d worker nodes", workerCount),
	})

	// 2. Pod Health
	runningPods := 0
	crashPods := 0
	highRestart := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if pod.Status.Phase == "Running" {
			runningPods++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashPods++
			}
			if cs.RestartCount >= 5 {
				highRestart++
			}
		}
	}
	podScore := 100
	if runningPods > 0 {
		podScore = (runningPods - crashPods) * 100 / runningPods
	}
	categories = append(categories, InsightCategory{
		Name: "Pod Health", Score: podScore, Status: insightStatus(podScore),
		Detail: fmt.Sprintf("%d running, %d crash, %d high-restart", runningPods, crashPods, highRestart),
	})

	// 3. Resource Governance
	quotaScore := 0
	if nsCount > 0 {
		quotaScore = len(quotas.Items) * 100 / nsCount
		if quotaScore > 100 {
			quotaScore = 100
		}
	}
	categories = append(categories, InsightCategory{
		Name: "Resource Governance", Score: quotaScore, Status: insightStatus(quotaScore),
		Detail: fmt.Sprintf("%d quotas / %d namespaces", len(quotas.Items), nsCount),
	})

	// 4. Network Security
	netpolNS := make(map[string]bool)
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			netpolNS[np.Namespace] = true
		}
	}
	netScore := 0
	if nsCount > 0 {
		netScore = len(netpolNS) * 100 / nsCount
	}
	categories = append(categories, InsightCategory{
		Name: "Network Security", Score: netScore, Status: insightStatus(netScore),
		Detail: fmt.Sprintf("%d/%d namespaces with netpol", len(netpolNS), nsCount),
	})

	// 5. Reliability (PDB + probes)
	deployCount := 0
	missingProbes := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes++
			}
		}
	}
	pdbScore := 50
	if deployCount > 0 {
		pdbScore = len(pdbs.Items) * 100 / deployCount
		if pdbScore > 100 {
			pdbScore = 100
		}
	}
	categories = append(categories, InsightCategory{
		Name: "Reliability", Score: pdbScore, Status: insightStatus(pdbScore),
		Detail: fmt.Sprintf("%d PDBs, %d missing probes", len(pdbs.Items), missingProbes),
	})

	// 6. Autoscaling
	asScore := 20
	if deployCount > 0 && len(hpas.Items) > 0 {
		asScore = len(hpas.Items) * 100 / deployCount
		if asScore > 100 {
			asScore = 100
		}
	}
	categories = append(categories, InsightCategory{
		Name: "Autoscaling", Score: asScore, Status: insightStatus(asScore),
		Detail: fmt.Sprintf("%d HPAs / %d deployments", len(hpas.Items), deployCount),
	})

	// Overall score
	totalScore := 0
	for _, c := range categories {
		totalScore += c.Score
	}
	if len(categories) > 0 {
		result.OverallScore = totalScore / len(categories)
	}
	switch {
	case result.OverallScore >= 80:
		result.Grade = "A"
	case result.OverallScore >= 60:
		result.Grade = "B"
	case result.OverallScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	// Critical alerts
	for _, c := range categories {
		if c.Score < 40 {
			result.CriticalAlerts = append(result.CriticalAlerts, InsightAlert{
				Category: c.Name, Severity: "critical",
				Message: c.Detail, Action: "Immediate attention required",
			})
		} else if c.Score < 60 {
			result.CriticalAlerts = append(result.CriticalAlerts, InsightAlert{
				Category: c.Name, Severity: "warning",
				Message: c.Detail, Action: "Plan improvement",
			})
		}
	}

	// Trends
	result.TrendIndicators = []TrendIndicator{
		{Metric: "Workload Count", Value: fmt.Sprintf("%d", deployCount), Trend: "stable"},
		{Metric: "Pod Density", Value: fmt.Sprintf("%d pods", runningPods), Trend: "stable"},
		{Metric: "Crash Rate", Value: fmt.Sprintf("%d", crashPods), Trend: crashTrend(crashPods)},
	}

	result.Categories = categories
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Score < categories[j].Score
	})
	result.Categories = categories

	result.Recommendations = buildInsightsRecs(&result)
	writeJSON(w, result)
}

func insightStatus(score int) string {
	if score >= 80 {
		return "healthy"
	}
	if score >= 60 {
		return "warning"
	}
	if score >= 40 {
		return "at-risk"
	}
	return "critical"
}

func crashTrend(crashCount int) string {
	if crashCount > 5 {
		return "degrading"
	}
	if crashCount > 0 {
		return "watch"
	}
	return "stable"
}

func buildInsightsRecs(r *PlatformInsightsResult) []string {
	recs := []string{
		fmt.Sprintf("Platform overall score: %d/100 (%s)", r.OverallScore, r.Grade),
	}
	for _, c := range r.Categories {
		if c.Score < 50 {
			recs = append(recs, fmt.Sprintf("[%s] %d/100 - %s", c.Name, c.Score, c.Detail))
		}
	}
	if len(r.CriticalAlerts) > 0 {
		recs = append(recs, fmt.Sprintf("%d critical alerts need attention", len(r.CriticalAlerts)))
	}
	return recs
}
