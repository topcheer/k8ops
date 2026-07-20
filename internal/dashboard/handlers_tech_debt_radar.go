package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TechDebtRadarResult tracks technical debt across the cluster with severity scoring.
type TechDebtRadarResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         TechDebtSummary    `json:"summary"`
	DebtItems       []TechDebtItem     `json:"debtItems"`
	ByCategory      []TechDebtCategory `json:"byCategory"`
	TotalDebtScore  int                `json:"totalDebtScore"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type TechDebtSummary struct {
	TotalItems   int `json:"totalItems"`
	CriticalDebt int `json:"criticalDebt"`
	HighDebt     int `json:"highDebt"`
	MediumDebt   int `json:"mediumDebt"`
	LowDebt      int `json:"lowDebt"`
}

type TechDebtItem struct {
	Category  string `json:"category"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
	Impact    string `json:"impact"`
	FixEffort string `json:"fixEffort"`
}

type TechDebtCategory struct {
	Category  string `json:"category"`
	ItemCount int    `json:"itemCount"`
	DebtScore int    `json:"debtScore"`
	Severity  string `json:"topSeverity"`
}

// handleTechDebtRadar handles GET /api/docs/tech-debt-radar
func (s *Server) handleTechDebtRadar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TechDebtRadarResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	catMap := make(map[string]*TechDebtCategory)
	addItem := func(cat, detail, severity, impact, effort string) {
		result.Summary.TotalItems++
		item := TechDebtItem{Category: cat, Detail: detail, Severity: severity, Impact: impact, FixEffort: effort}
		result.DebtItems = append(result.DebtItems, item)
		if catMap[cat] == nil {
			catMap[cat] = &TechDebtCategory{Category: cat}
		}
		catMap[cat].ItemCount++
		weight := 0
		switch severity {
		case "critical":
			weight = 10
			result.Summary.CriticalDebt++
		case "high":
			weight = 7
			result.Summary.HighDebt++
		case "medium":
			weight = 4
			result.Summary.MediumDebt++
		case "low":
			weight = 1
			result.Summary.LowDebt++
		}
		catMap[cat].DebtScore += weight
		if weight > 7 {
			catMap[cat].Severity = severity
		} else if catMap[cat].Severity == "" && weight > 4 {
			catMap[cat].Severity = severity
		} else if catMap[cat].Severity == "" {
			catMap[cat].Severity = severity
		}
	}

	// Scan for tech debt patterns
	noLimitCount := 0
	noProbeCount := 0
	rootCount := 0
	latestTagCount := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
				noLimitCount++
			}
			if c.ReadinessProbe == nil {
				noProbeCount++
			}
			if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				rootCount++
			}
			// Check latest tag
			img := c.Image
			if len(img) > 7 && img[len(img)-7:] == ":latest" {
				latestTagCount++
			}
		}
	}

	singleReplica := 0
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
	}

	userNS := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			userNS++
		}
	}

	// Build debt items
	if noLimitCount > 0 {
		sev := "high"
		if noLimitCount > 50 {
			sev = "critical"
		}
		addItem("Resource Governance", fmt.Sprintf("%d containers without resource limits", noLimitCount), sev, "Noisy neighbor risk, scheduling impact", "medium")
	}
	if noProbeCount > 0 {
		sev := "high"
		if noProbeCount > 50 {
			sev = "critical"
		}
		addItem("Observability", fmt.Sprintf("%d containers without readiness probes", noProbeCount), sev, "Traffic routed to unready pods", "low")
	}
	if rootCount > 0 {
		sev := "medium"
		if rootCount > 50 {
			sev = "high"
		}
		addItem("Security", fmt.Sprintf("%d containers running as root", rootCount), sev, "Privilege escalation risk", "medium")
	}
	if latestTagCount > 0 {
		addItem("Image Hygiene", fmt.Sprintf("%d containers using :latest tag", latestTagCount), "medium", "Non-reproducible deployments", "low")
	}
	if singleReplica > 0 {
		sev := "medium"
		if singleReplica > 30 {
			sev = "high"
		}
		addItem("High Availability", fmt.Sprintf("%d single-replica deployments", singleReplica), sev, "No rolling update, SPOF", "high")
	}
	addItem("Namespace Governance", fmt.Sprintf("%d namespaces, many without ResourceQuota", userNS), "high", "Resource exhaustion risk", "low")

	for _, c := range catMap {
		result.ByCategory = append(result.ByCategory, *c)
		result.TotalDebtScore += c.DebtScore
	}

	result.HealthScore = 100
	if result.TotalDebtScore > 0 {
		result.HealthScore = 100 - result.TotalDebtScore
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("技术债务雷达: %d 问题 (%d 严重, %d 高, %d 中, %d 低), 总债务分: %d",
			result.Summary.TotalItems, result.Summary.CriticalDebt, result.Summary.HighDebt,
			result.Summary.MediumDebt, result.Summary.LowDebt, result.TotalDebtScore),
	}
	for _, c := range result.ByCategory {
		if c.Severity == "critical" || c.Severity == "high" {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("[%s] %s: %d 项, 分数 %d", c.Severity, c.Category, c.ItemCount, c.DebtScore))
		}
	}
	writeJSON(w, result)
}
