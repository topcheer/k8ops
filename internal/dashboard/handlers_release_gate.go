package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReleaseGateResult evaluates whether the cluster is ready for a new
// deployment release. It acts as a pre-deployment checklist that verifies
// all reliability, security, and operational prerequisites.
type ReleaseGateResult struct {
	ScannedAt      time.Time           `json:"scannedAt"`
	OverallVerdict string              `json:"overallVerdict"` // pass, conditional, fail
	GateScore      int                 `json:"gateScore"`
	Checks         []ReleaseGateCheck  `json:"checks"`
	Blockers       []ReleaseBlocker    `json:"blockers"`
	Warnings       []ReleaseWarning    `json:"warnings"`
	PassRate       float64             `json:"passRate"`
	ByCategory     []GateCategory      `json:"byCategory"`
	Recommendations []string           `json:"recommendations"`
}

type ReleaseGateCheck struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Status      string `json:"status"` // pass, warn, fail
	Description string `json:"description"`
	Detail      string `json:"detail"`
	Weight      int    `json:"weight"`
}

type ReleaseBlocker struct {
	Check     string `json:"check"`
	Severity  string `json:"severity"`
	Action    string `json:"action"`
}

type ReleaseWarning struct {
	Check   string `json:"check"`
	Message string `json:"message"`
}

type GateCategory struct {
	Category string `json:"category"`
	Total    int    `json:"total"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
	Warnings int    `json:"warnings"`
	Score    int    `json:"score"`
}

// handleReleaseGate handles GET /api/deployment/release-gate
func (s *Server) handleReleaseGate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ReleaseGateResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	var checks []ReleaseGateCheck
	var blockers []ReleaseBlocker
	var warnings []ReleaseWarning

	addCheck := func(name, category, status, desc, detail string, weight int) {
		checks = append(checks, ReleaseGateCheck{
			Name: name, Category: category, Status: status,
			Description: desc, Detail: detail, Weight: weight,
		})
	}

	// Check 1: PDB Coverage
	pdbCount := len(pdbs.Items)
	multiReplica := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if ptrInt32(d.Spec.Replicas) >= 2 {
			multiReplica++
		}
	}
	pdbStatus := "pass"
	if multiReplica > 0 && pdbCount < multiReplica/2 {
		pdbStatus = "fail"
		blockers = append(blockers, ReleaseBlocker{
			Check: "PDB Coverage", Severity: "high",
			Action: fmt.Sprintf("仅 %d/%d 多副本工作负载有 PDB", pdbCount, multiReplica),
		})
	} else if pdbCount < multiReplica {
		pdbStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "PDB Coverage", Message: "PDB 覆盖不完整"})
	}
	addCheck("PDB Coverage", "Reliability", pdbStatus,
		"多副本工作负载是否有 PDB 保护",
		fmt.Sprintf("%d PDBs / %d multi-replica workloads", pdbCount, multiReplica), 15)

	// Check 2: Health Probes
	missingProbes := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes++
			}
		}
	}
	probeStatus := "pass"
	if missingProbes > 5 {
		probeStatus = "fail"
		blockers = append(blockers, ReleaseBlocker{Check: "Health Probes", Severity: "high", Action: fmt.Sprintf("%d 个容器缺少探针", missingProbes)})
	} else if missingProbes > 0 {
		probeStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "Health Probes", Message: fmt.Sprintf("%d 个容器缺少探针", missingProbes)})
	}
	addCheck("Health Probes", "Reliability", probeStatus,
		"所有容器都有存活和就绪探针",
		fmt.Sprintf("%d missing probes", missingProbes), 12)

	// Check 3: Resource Limits
	missingLimits := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				missingLimits++
			}
		}
	}
	limitStatus := "pass"
	if missingLimits > 10 {
		limitStatus = "fail"
		blockers = append(blockers, ReleaseBlocker{Check: "Resource Limits", Severity: "high", Action: fmt.Sprintf("%d 个容器缺少限制", missingLimits)})
	} else if missingLimits > 0 {
		limitStatus = "warn"
	}
	addCheck("Resource Limits", "Reliability", limitStatus,
		"所有容器都设置了 CPU 和内存限制",
		fmt.Sprintf("%d without limits", missingLimits), 10)

	// Check 4: Security Context
	missingSC := 0
	privileged := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.SecurityContext == nil {
				missingSC++
			} else if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				privileged++
			}
		}
	}
	scStatus := "pass"
	if privileged > 0 {
		scStatus = "fail"
		blockers = append(blockers, ReleaseBlocker{Check: "Security Context", Severity: "critical", Action: fmt.Sprintf("%d 个特权容器", privileged)})
	} else if missingSC > 5 {
		scStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "Security Context", Message: fmt.Sprintf("%d 个容器缺少 securityContext", missingSC)})
	}
	addCheck("Security Context", "Security", scStatus,
		"容器安全上下文配置正确",
		fmt.Sprintf("%d missing SC, %d privileged", missingSC, privileged), 15)

	// Check 5: Multi-Node HA
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			if _, ok2 := n.Labels["node-role.kubernetes.io/master"]; !ok2 {
				workerCount++
			}
		}
	}
	haStatus := "pass"
	if workerCount < 2 {
		haStatus = "fail"
		blockers = append(blockers, ReleaseBlocker{Check: "Multi-Node HA", Severity: "critical", Action: fmt.Sprintf("仅 %d 个工作节点", workerCount)})
	}
	addCheck("Multi-Node HA", "Availability", haStatus,
		"集群至少有 2 个工作节点",
		fmt.Sprintf("%d worker nodes", workerCount), 20)

	// Check 6: Update Strategy
	badStrategy := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType && ptrInt32(d.Spec.Replicas) > 1 {
			badStrategy++
		}
	}
	stratStatus := "pass"
	if badStrategy > 0 {
		stratStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "Update Strategy", Message: fmt.Sprintf("%d 个多副本 Deployment 使用 Recreate", badStrategy)})
	}
	addCheck("Update Strategy", "Deployment", stratStatus,
		"多副本工作负载使用 RollingUpdate",
		fmt.Sprintf("%d using Recreate", badStrategy), 8)

	// Check 7: PSA Labels
	nsWithoutPSA := 0
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		if ns.Labels["pod-security.kubernetes.io/enforce"] == "" {
			nsWithoutPSA++
		}
	}
	psaStatus := "pass"
	if nsWithoutPSA > 5 {
		psaStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "PSA Labels", Message: fmt.Sprintf("%d 个命名空间缺少 PSA", nsWithoutPSA)})
	}
	addCheck("PSA Labels", "Security", psaStatus,
		"命名空间启用了 Pod Security Admission",
		fmt.Sprintf("%d namespaces without PSA", nsWithoutPSA), 10)

	// Check 8: Anti-Affinity
	noAffinity := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if ptrInt32(d.Spec.Replicas) >= 2 {
			if d.Spec.Template.Spec.Affinity == nil || d.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
				if len(d.Spec.Template.Spec.TopologySpreadConstraints) == 0 {
					noAffinity++
				}
			}
		}
	}
	affStatus := "pass"
	if noAffinity > 3 {
		affStatus = "warn"
		warnings = append(warnings, ReleaseWarning{Check: "Anti-Affinity", Message: fmt.Sprintf("%d 个多副本工作负载缺少反亲和性", noAffinity)})
	}
	addCheck("Anti-Affinity", "Availability", affStatus,
		"多副本工作负载配置了 podAntiAffinity",
		fmt.Sprintf("%d without anti-affinity", noAffinity), 10)

	// Calculate results
	result.Checks = checks
	result.Blockers = blockers
	result.Warnings = warnings

	passed := 0
	for _, c := range checks {
		if c.Status == "pass" {
			passed++
		}
	}
	result.PassRate = float64(passed) / float64(len(checks)) * 100

	// Category breakdown
	catMap := make(map[string]*GateCategory)
	for _, c := range checks {
		if _, ok := catMap[c.Category]; !ok {
			catMap[c.Category] = &GateCategory{Category: c.Category}
		}
		cat := catMap[c.Category]
		cat.Total++
		switch c.Status {
		case "pass":
			cat.Passed++
			cat.Score += c.Weight
		case "warn":
			cat.Warnings++
			cat.Score += c.Weight / 2
		case "fail":
			cat.Failed++
		}
	}
	maxScore := 0
	achievedScore := 0
	for _, c := range checks {
		maxScore += c.Weight
		if c.Status == "pass" {
			achievedScore += c.Weight
		} else if c.Status == "warn" {
			achievedScore += c.Weight / 2
		}
	}
	if maxScore > 0 {
		result.GateScore = achievedScore * 100 / maxScore
	}
	for _, cat := range catMap {
		result.ByCategory = append(result.ByCategory, *cat)
	}
	sort.Slice(result.ByCategory, func(i, j int) bool {
		return result.ByCategory[i].Score < result.ByCategory[j].Score
	})

	if len(blockers) > 0 {
		result.OverallVerdict = "fail"
	} else if len(warnings) > 3 {
		result.OverallVerdict = "conditional"
	} else {
		result.OverallVerdict = "pass"
	}

	result.Recommendations = buildGateRecs(&result)
	writeJSON(w, result)
}

func buildGateRecs(r *ReleaseGateResult) []string {
	recs := []string{}
	if r.OverallVerdict == "fail" {
		recs = append(recs, fmt.Sprintf("发布门禁未通过: %d 个阻塞性问题需要先修复", len(r.Blockers)))
		for _, b := range r.Blockers {
			recs = append(recs, fmt.Sprintf("  - [%s] %s", b.Severity, b.Action))
		}
	} else if r.OverallVerdict == "conditional" {
		recs = append(recs, fmt.Sprintf("发布有条件通过: %d 个警告需要注意", len(r.Warnings)))
	} else {
		recs = append(recs, "发布门禁通过，所有关键检查项达标")
	}
	if len(r.ByCategory) > 0 {
		weakest := r.ByCategory[0]
		recs = append(recs, fmt.Sprintf("最弱维度: %s (%d/%d 通过)", weakest.Category, weakest.Passed, weakest.Total))
	}
	return recs
}

var _ = strings.Contains
