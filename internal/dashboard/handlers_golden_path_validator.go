package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GoldenPathValidatorResult validates workloads against golden path
// standards: best-practice configuration templates for production readiness.
type GoldenPathValidatorResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         GoldenPathSummary `json:"summary"`
	ByWorkload      []GoldenPathEntry `json:"byWorkload"`
	NonCompliant    []GoldenPathEntry `json:"nonCompliant"`
	ComplianceScore int               `json:"complianceScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type GoldenPathSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	FullyCompliant    int `json:"fullyCompliant"`
	PartialCompliance int `json:"partialCompliance"`
	NonCompliantCount int `json:"nonCompliantCount"`
	MissingProbes     int `json:"missingProbes"`
	MissingLimits     int `json:"missingLimits"`
	MissingAffinity   int `json:"missingAffinity"`
	MissingPDB        int `json:"missingPDB"`
}

type GoldenPathEntry struct {
	Workload        string            `json:"workload"`
	Namespace       string            `json:"namespace"`
	Kind            string            `json:"kind"`
	Score           int               `json:"score"`
	Checks          []GoldenPathCheck `json:"checks"`
	ComplianceLevel string            `json:"complianceLevel"`
	MissingItems    []string          `json:"missingItems"`
}

type GoldenPathCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// handleGoldenPathValidator handles GET /api/product/golden-path-validator
func (s *Server) handleGoldenPathValidator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GoldenPathValidatorResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbMap := make(map[string]bool)
	for _, p := range pdbs.Items {
		pdbMap[p.Namespace+"/"+p.Name] = true
	}

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := GoldenPathEntry{Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment"}
		replicas := 1
		if d.Spec.Replicas != nil {
			replicas = int(*d.Spec.Replicas)
		}

		// Check 1: Readiness probe
		hasReady := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				hasReady = true
				break
			}
		}
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "readiness-probe", Passed: hasReady})
		if !hasReady {
			result.Summary.MissingProbes++
			entry.MissingItems = append(entry.MissingItems, "readinessProbe")
		}

		// Check 2: Liveness probe
		hasLive := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe != nil {
				hasLive = true
				break
			}
		}
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "liveness-probe", Passed: hasLive})
		if !hasLive {
			entry.MissingItems = append(entry.MissingItems, "livenessProbe")
		}

		// Check 3: Resource limits
		hasLimits := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if len(c.Resources.Limits) == 0 {
				hasLimits = false
				break
			}
		}
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "resource-limits", Passed: hasLimits})
		if !hasLimits {
			result.Summary.MissingLimits++
			entry.MissingItems = append(entry.MissingItems, "resourceLimits")
		}

		// Check 4: Multi-replica
		multiReplica := replicas >= 2
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "multi-replica", Passed: multiReplica})
		if !multiReplica {
			entry.MissingItems = append(entry.MissingItems, "multiReplica")
		}

		// Check 5: PDB
		hasPDB := pdbMap[d.Namespace+"/"+d.Name]
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "pdb", Passed: hasPDB})
		if !hasPDB {
			result.Summary.MissingPDB++
			entry.MissingItems = append(entry.MissingItems, "PDB")
		}

		// Check 6: Affinity/anti-affinity
		hasAffinity := d.Spec.Template.Spec.Affinity != nil
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "affinity", Passed: hasAffinity})
		if !hasAffinity {
			result.Summary.MissingAffinity++
			entry.MissingItems = append(entry.MissingItems, "affinity")
		}

		// Check 7: RollingUpdate strategy
		isRolling := d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType
		entry.Checks = append(entry.Checks, GoldenPathCheck{Name: "rolling-strategy", Passed: isRolling})
		if !isRolling {
			entry.MissingItems = append(entry.MissingItems, "rollingStrategy")
		}

		// Score
		passed := 0
		for _, c := range entry.Checks {
			if c.Passed {
				passed++
			}
		}
		entry.Score = passed * 100 / len(entry.Checks)

		switch {
		case entry.Score == 100:
			entry.ComplianceLevel = "golden"
			result.Summary.FullyCompliant++
		case entry.Score >= 50:
			entry.ComplianceLevel = "partial"
			result.Summary.PartialCompliance++
		default:
			entry.ComplianceLevel = "non-compliant"
			result.Summary.NonCompliantCount++
			result.NonCompliant = append(result.NonCompliant, entry)
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort by score ascending (worst first)
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].Score < result.ByWorkload[j].Score
	})

	if result.Summary.TotalWorkloads > 0 {
		result.ComplianceScore = result.Summary.FullyCompliant * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.ComplianceScore)

	result.Recommendations = []string{
		fmt.Sprintf("Golden Path 合规: %d/%d 完全合规 (%d%%)", result.Summary.FullyCompliant, result.Summary.TotalWorkloads, result.ComplianceScore),
		fmt.Sprintf("缺口: %d 缺探针, %d 缺限制, %d 缺 PDB, %d 缺亲和性", result.Summary.MissingProbes, result.Summary.MissingLimits, result.Summary.MissingPDB, result.Summary.MissingAffinity),
	}
	if result.Summary.NonCompliantCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作负载严重不合规", result.Summary.NonCompliantCount))
	}
	if result.ComplianceScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 创建 golden path 模板, 新工作负载必须满足全部检查项")
	}
	writeJSON(w, result)
	_ = corev1.Pod{}
}
