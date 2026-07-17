package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformComparisonResult generates a comprehensive snapshot of the cluster
// state with historical context. It captures key metrics that can be compared
// across time to show improvement trends and regression detection.
type PlatformComparisonResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	SnapshotID      string               `json:"snapshotId"`
	Metrics         []ComparisonMetric   `json:"metrics"`
	Categories      []ComparisonCategory `json:"categories"`
	Highlights      []string             `json:"highlights"`
	Concerns        []string             `json:"concerns"`
	OverallScore    int                  `json:"overallScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type ComparisonMetric struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Score    int    `json:"score"`
	Status   string `json:"status"`
}

type ComparisonCategory struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Count  int    `json:"metricCount"`
	Status string `json:"status"`
}

// handlePlatformComparison handles GET /api/docs/platform-comparison
func (s *Server) handlePlatformComparison(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PlatformComparisonResult{
		ScannedAt:  time.Now(),
		SnapshotID: fmt.Sprintf("cmp-%d", time.Now().Unix()),
	}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// Count non-system resources
	deployCount := 0
	podCount := 0
	nsCount := 0
	secretCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
	}
	for _, p := range pods.Items {
		if !isSystemNamespace(p.Namespace) {
			podCount++
		}
	}
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}
	for _, sec := range secrets.Items {
		if !isSystemNamespace(sec.Namespace) {
			secretCount++
		}
	}

	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}

	// Calculate scores
	var metrics []ComparisonMetric

	// Availability
	availScore := 100
	if workerCount < 2 {
		availScore = 20
	}
	metrics = append(metrics, ComparisonMetric{
		Category: "Availability", Name: "Worker Nodes", Value: fmt.Sprintf("%d", workerCount),
		Score: availScore, Status: platformScoreStatus(availScore),
	})

	// Pod Health
	crashCount := 0
	for _, p := range pods.Items {
		if isSystemNamespace(p.Namespace) {
			continue
		}
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashCount++
			}
		}
	}
	podScore := 100
	if podCount > 0 {
		podScore = (podCount - crashCount) * 100 / podCount
	}
	metrics = append(metrics, ComparisonMetric{
		Category: "Stability", Name: "Pod Crashes", Value: fmt.Sprintf("%d/%d", crashCount, podCount),
		Score: podScore, Status: platformScoreStatus2(podScore),
	})

	// Resource Governance
	quotaScore := 0
	if nsCount > 0 {
		quotaScore = len(quotas.Items) * 100 / nsCount
		if quotaScore > 100 {
			quotaScore = 100
		}
	}
	metrics = append(metrics, ComparisonMetric{
		Category: "Governance", Name: "Resource Quotas", Value: fmt.Sprintf("%d/%d NS", len(quotas.Items), nsCount),
		Score: quotaScore, Status: platformScoreStatus2(quotaScore),
	})

	// Network Security
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
	metrics = append(metrics, ComparisonMetric{
		Category: "Security", Name: "Network Policies", Value: fmt.Sprintf("%d/%d NS", len(netpolNS), nsCount),
		Score: netScore, Status: platformScoreStatus2(netScore),
	})

	// PDB Coverage
	pdbScore := 0
	if deployCount > 0 {
		pdbScore = len(pdbs.Items) * 100 / deployCount
		if pdbScore > 100 {
			pdbScore = 100
		}
	}
	metrics = append(metrics, ComparisonMetric{
		Category: "Reliability", Name: "PDB Coverage", Value: fmt.Sprintf("%d/%d", len(pdbs.Items), deployCount),
		Score: pdbScore, Status: platformScoreStatus2(pdbScore),
	})

	// Autoscaling
	hpaScore := 20
	if deployCount > 0 {
		hpaScore = len(hpas.Items) * 100 / deployCount
		if hpaScore > 100 {
			hpaScore = 100
		}
	}
	metrics = append(metrics, ComparisonMetric{
		Category: "Scalability", Name: "HPA Coverage", Value: fmt.Sprintf("%d/%d", len(hpas.Items), deployCount),
		Score: hpaScore, Status: platformScoreStatus2(hpaScore),
	})

	// Secrets Hygiene
	metrics = append(metrics, ComparisonMetric{
		Category: "Security", Name: "Secrets Count", Value: fmt.Sprintf("%d", secretCount),
		Score: func() int {
			if secretCount > 200 {
				return 30
			}
			if secretCount > 100 {
				return 60
			}
			return 90
		}(),
		Status: platformScoreStatus2(70),
	})

	// Build categories
	catMap := make(map[string]*ComparisonCategory)
	for _, m := range metrics {
		if _, ok := catMap[m.Category]; !ok {
			catMap[m.Category] = &ComparisonCategory{Name: m.Category}
		}
		cat := catMap[m.Category]
		cat.Count++
		cat.Score += m.Score
	}
	totalScore := 0
	for _, cat := range catMap {
		cat.Score /= cat.Count
		cat.Status = platformScoreStatus2(cat.Score)
		totalScore += cat.Score
		result.Categories = append(result.Categories, *cat)
	}
	if len(catMap) > 0 {
		result.OverallScore = totalScore / len(catMap)
	}
	result.Grade = platformGradeFromScore(result.OverallScore)

	// Highlights and concerns
	for _, cat := range result.Categories {
		if cat.Score >= 80 {
			result.Highlights = append(result.Highlights, fmt.Sprintf("%s: %d/100 (%s)", cat.Name, cat.Score, cat.Status))
		} else if cat.Score < 40 {
			result.Concerns = append(result.Concerns, fmt.Sprintf("%s: %d/100 (%s)", cat.Name, cat.Score, cat.Status))
		}
	}

	result.Metrics = metrics
	sort.Slice(result.Categories, func(i, j int) bool {
		return result.Categories[i].Score > result.Categories[j].Score
	})

	result.Recommendations = []string{
		fmt.Sprintf("快照 %s: 平台总分 %d/100 (%s)", result.SnapshotID, result.OverallScore, result.Grade),
		fmt.Sprintf("%d 个亮点, %d 个需要关注的问题", len(result.Highlights), len(result.Concerns)),
		"保存此快照用于未来比较，追踪改进趋势",
	}
	writeJSON(w, result)
}

func platformScoreStatus(score int) string {
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

func platformScoreStatus2(score int) string {
	return platformScoreStatus(score)
}

func platformGradeFromScore(score int) string {
	switch {
	case score >= 80:
		return "A"
	case score >= 60:
		return "B"
	case score >= 40:
		return "C"
	default:
		return "D"
	}
}
