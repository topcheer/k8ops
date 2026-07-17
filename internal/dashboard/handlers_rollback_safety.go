package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RollbackSafetyResult evaluates whether rollback is safe by checking
// revision history depth, config compatibility, and data migration risks.
type RollbackSafetyResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         RollbackSafetySummary `json:"summary"`
	ByWorkload      []RollbackSafetyEntry `json:"byWorkload"`
	UnsafeWorkloads []RollbackSafetyEntry `json:"unsafeWorkloads"`
	SafetyScore     int                   `json:"safetyScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type RollbackSafetySummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	SafeToRollback  int `json:"safeToRollback"`
	UnsafeRollback  int `json:"unsafeRollback"`
	NoHistory       int `json:"noHistory"`
	LowHistoryDepth int `json:"lowHistoryDepth"`
	HasPVC          int `json:"hasPVC"`
	HasConfigDrift  int `json:"hasConfigDrift"`
}

type RollbackSafetyEntry struct {
	Workload          string   `json:"workload"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	RevisionHistory   int      `json:"revisionHistory"`
	CanRollback       bool     `json:"canRollback"`
	SafetyLevel       string   `json:"safetyLevel"`
	RiskFactors       []string `json:"riskFactors"`
	HasPVC            bool     `json:"hasPVC"`
	HasBreakingChange bool     `json:"hasBreakingChange"`
	HasConfigDrift    bool     `json:"hasConfigDrift"`
	Recommendation    string   `json:"recommendation"`
}

// handleRollbackSafety handles GET /api/deployment/rollback-safety
func (s *Server) handleRollbackSafety(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RollbackSafetyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	cmList, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Build PVC namespace map
	pvcNsMap := make(map[string]bool)
	for _, pvc := range pvcs.Items {
		pvcNsMap[pvc.Namespace] = true
	}

	// Build configmap namespace map
	cmNsMap := make(map[string]bool)
	for _, cm := range cmList.Items {
		if !isSystemNamespace(cm.Namespace) {
			cmNsMap[cm.Namespace] = true
		}
	}

	// Analyze Deployments
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := RollbackSafetyEntry{
			Workload:  d.Name,
			Namespace: d.Namespace,
			Kind:      "Deployment",
		}

		// Check revision history
		revHistory := 10
		if d.Spec.RevisionHistoryLimit != nil {
			revHistory = int(*d.Spec.RevisionHistoryLimit)
		}
		entry.RevisionHistory = revHistory

		// Check for PVC usage in pod template
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				entry.HasPVC = true
				result.Summary.HasPVC++
				break
			}
		}

		// Check for configmap references (potential drift)
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil {
				entry.HasConfigDrift = true
				result.Summary.HasConfigDrift++
				break
			}
		}

		// Determine safety
		var risks []string
		safe := true

		if revHistory < 2 {
			risks = append(risks, fmt.Sprintf("revisionHistoryLimit=%d (<2, insufficient for rollback)", revHistory))
			safe = false
			result.Summary.LowHistoryDepth++
		}
		if revHistory == 0 {
			risks = append(risks, "no revision history (rollback impossible)")
			safe = false
			result.Summary.NoHistory++
		}
		if entry.HasPVC {
			risks = append(risks, "uses PVC (data migration may not be reversible)")
		}
		if entry.HasConfigDrift {
			risks = append(risks, "references ConfigMaps (config may have drifted)")
		}

		// Check for breaking changes (env var removals, port changes)
		for _, c := range d.Spec.Template.Spec.Containers {
			if len(c.Ports) == 0 && len(c.Env) > 5 {
				entry.HasBreakingChange = true
				risks = append(risks, "potential breaking change (port/env config)")
				break
			}
		}

		entry.RiskFactors = risks
		entry.CanRollback = safe

		switch {
		case !safe:
			entry.SafetyLevel = "unsafe"
			result.Summary.UnsafeRollback++
		case entry.HasPVC || entry.HasConfigDrift:
			entry.SafetyLevel = "caution"
		default:
			entry.SafetyLevel = "safe"
			result.Summary.SafeToRollback++
		}

		if entry.SafetyLevel == "unsafe" {
			entry.Recommendation = "Increase revisionHistoryLimit to at least 3"
		} else if entry.SafetyLevel == "caution" {
			entry.Recommendation = "Verify data compatibility before rollback"
		} else {
			entry.Recommendation = "Rollback is safe"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
		if entry.SafetyLevel == "unsafe" {
			result.UnsafeWorkloads = append(result.UnsafeWorkloads, entry)
		}
	}

	// Sort by safety level (unsafe first)
	safetyRank := map[string]int{"unsafe": 0, "caution": 1, "safe": 2}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return safetyRank[result.ByWorkload[i].SafetyLevel] < safetyRank[result.ByWorkload[j].SafetyLevel]
	})

	// Safety score
	if result.Summary.TotalWorkloads > 0 {
		result.SafetyScore = result.Summary.SafeToRollback * 100 / result.Summary.TotalWorkloads
	}

	switch {
	case result.SafetyScore >= 80:
		result.Grade = "A"
	case result.SafetyScore >= 60:
		result.Grade = "B"
	case result.SafetyScore >= 40:
		result.Grade = "C"
	case result.SafetyScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildRollbackSafetyRecs(&result)
	writeJSON(w, result)
}

func buildRollbackSafetyRecs(r *RollbackSafetyResult) []string {
	recs := []string{
		fmt.Sprintf("回滚安全: %d/%d 工作负载可安全回滚", r.Summary.SafeToRollback, r.Summary.TotalWorkloads),
	}
	if r.Summary.UnsafeRollback > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个工作负载无法安全回滚", r.Summary.UnsafeRollback))
	}
	if r.Summary.NoHistory > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载完全无修订历史 (revisionHistoryLimit=0)", r.Summary.NoHistory))
	}
	if r.Summary.HasPVC > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载使用 PVC, 回滚可能需数据迁移", r.Summary.HasPVC))
	}
	if r.SafetyScore < 60 {
		recs = append(recs, "建议: 设置 revisionHistoryLimit >= 3, 验证数据兼容性")
	}
	return recs
}

// keep import
var _ appsv1.Deployment
