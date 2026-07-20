package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostOptimizationRoadmapResult generates a prioritized cost optimization plan.
type CostOptimizationRoadmapResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CostRoadmapSummary `json:"summary"`
	QuickWins       []CostRoadmapItem  `json:"quickWins"`
	MediumTerm      []CostRoadmapItem  `json:"mediumTerm"`
	LongTerm        []CostRoadmapItem  `json:"longTerm"`
	TotalSavings    float64            `json:"estimatedTotalSavingsPerMonth"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type CostRoadmapSummary struct {
	TotalActions  int     `json:"totalActions"`
	QuickWinCount int     `json:"quickWinCount"`
	EstSavingsMo  float64 `json:"estimatedSavingsPerMonth"`
	CurrentSpend  float64 `json:"currentMonthlySpend"`
	SavingsPct    float64 `json:"savingsPct"`
}

type CostRoadmapItem struct {
	Title      string  `json:"title"`
	Category   string  `json:"category"`
	Effort     string  `json:"effort"`
	Impact     string  `json:"impact"`
	EstSavings float64 `json:"estimatedSavingsPerMonth"`
	Priority   int     `json:"priority"`
}

// handleCostOptimizationRoadmap handles GET /api/docs/cost-optimization-roadmap
func (s *Server) handleCostOptimizationRoadmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CostOptimizationRoadmapResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var totalCPUReq, totalMemReq float64
	noLimitCount := 0
	singleReplica := 0
	highReqCount := 0

	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas < 2 {
			singleReplica++
		}

		var cpuReq, memReq float64
		for _, c := range dep.Spec.Template.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuReq += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq += float64(req.Value()) / (1024 * 1024 * 1024)
			}
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
				noLimitCount++
			}
		}
		totalCPUReq += cpuReq * float64(replicas)
		totalMemReq += memReq * float64(replicas)
		if cpuReq > 1.0 {
			highReqCount++
		}
	}

	// Estimate current spend
	result.Summary.CurrentSpend = totalCPUReq*20 + totalMemReq*3

	// Quick wins: low effort, immediate savings
	if noLimitCount > 0 {
		savings := float64(noLimitCount) * 0.2 * 20 // ~0.2 core waste each
		result.QuickWins = append(result.QuickWins, CostRoadmapItem{
			Title:    fmt.Sprintf("Set resource limits on %d unbounded containers", noLimitCount),
			Category: "Resource Governance", Effort: "low", Impact: "medium",
			EstSavings: savings, Priority: 90,
		})
	}

	// Orphaned PVs quick win
	orphanPVCs := 0
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	mountedPVCs := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				mountedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		if !mountedPVCs[pvc.Namespace+"/"+pvc.Name] {
			orphanPVCs++
		}
	}
	if orphanPVCs > 0 {
		savings := float64(orphanPVCs) * 5.0 // ~$5/GB/mo per PVC
		result.QuickWins = append(result.QuickWins, CostRoadmapItem{
			Title:    fmt.Sprintf("Delete %d orphaned PVCs", orphanPVCs),
			Category: "Storage Cleanup", Effort: "low", Impact: "low",
			EstSavings: savings, Priority: 80,
		})
	}

	// Medium term: right-size
	if highReqCount > 0 {
		savings := float64(highReqCount) * 0.5 * 20 // 0.5 core over-provisioned each
		result.MediumTerm = append(result.MediumTerm, CostRoadmapItem{
			Title:    fmt.Sprintf("Right-size %d high-CPU deployments", highReqCount),
			Category: "Right-Sizing", Effort: "medium", Impact: "high",
			EstSavings: savings, Priority: 70,
		})
	}

	// Medium: implement HPA
	if singleReplica > 10 {
		savings := float64(singleReplica) * 0.3 * 20 // 30% savings from HPA scaling
		result.MediumTerm = append(result.MediumTerm, CostRoadmapItem{
			Title:    fmt.Sprintf("Implement HPA for %d single-replica deployments", singleReplica),
			Category: "Autoscaling", Effort: "medium", Impact: "high",
			EstSavings: savings, Priority: 60,
		})
	}

	// Long term: cluster autoscaler, spot instances
	result.LongTerm = append(result.LongTerm, CostRoadmapItem{
		Title:    "Implement Cluster Autoscaler",
		Category: "Infrastructure", Effort: "high", Impact: "high",
		EstSavings: result.Summary.CurrentSpend * 0.15, Priority: 50,
	})
	result.LongTerm = append(result.LongTerm, CostRoadmapItem{
		Title:    "Evaluate spot/preemptible nodes for non-critical workloads",
		Category: "Infrastructure", Effort: "high", Impact: "medium",
		EstSavings: result.Summary.CurrentSpend * 0.20, Priority: 40,
	})

	// Calculate totals
	for _, item := range result.QuickWins {
		result.TotalSavings += item.EstSavings
		result.Summary.QuickWinCount++
	}
	for _, item := range result.MediumTerm {
		result.TotalSavings += item.EstSavings
	}
	for _, item := range result.LongTerm {
		result.TotalSavings += item.EstSavings
	}
	result.Summary.TotalActions = len(result.QuickWins) + len(result.MediumTerm) + len(result.LongTerm)
	result.Summary.EstSavingsMo = result.TotalSavings
	if result.Summary.CurrentSpend > 0 {
		result.Summary.SavingsPct = result.TotalSavings / result.Summary.CurrentSpend * 100
	}

	sort.Slice(result.QuickWins, func(i, j int) bool { return result.QuickWins[i].Priority > result.QuickWins[j].Priority })
	sort.Slice(result.MediumTerm, func(i, j int) bool { return result.MediumTerm[i].Priority > result.MediumTerm[j].Priority })

	result.HealthScore = 100 - int(result.Summary.SavingsPct)
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("成本优化路线图: %d 行动项, %d 快赢, 月省 $%.2f (%.0f%% 当前 $%.2f/月)",
			result.Summary.TotalActions, result.Summary.QuickWinCount,
			result.TotalSavings, result.Summary.SavingsPct, result.Summary.CurrentSpend),
	}
	if len(result.QuickWins) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("快赢: %s ($%.0f/月)", result.QuickWins[0].Title, result.QuickWins[0].EstSavings))
	}
	writeJSON(w, result)
}
