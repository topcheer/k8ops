package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadMaturityMatrixResult evaluates workload maturity across multiple dimensions.
type WorkloadMaturityMatrixResult struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	Summary         MaturityMatrixSummary     `json:"summary"`
	Dimensions      []MaturityMatrixDimension `json:"dimensions"`
	ByNamespace     []MaturityNsEntry         `json:"byNamespace"`
	OverallScore    int                       `json:"overallScore"`
	Grade           string                    `json:"grade"`
	Recommendations []string                  `json:"recommendations"`
}

type MaturityMatrixSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	AvgMaturity       float64 `json:"avgMaturity"`
	EliteWorkloads    int     `json:"eliteWorkloads"`    // all 6 dimensions pass
	BasicWorkloads    int     `json:"basicWorkloads"`    // 0-2 dimensions
	MidWorkloads      int     `json:"midWorkloads"`      // 3-4 dimensions
	AdvancedWorkloads int     `json:"advancedWorkloads"` // 5 dimensions
}

type MaturityMatrixDimension struct {
	Name      string  `json:"name"`
	PassCount int     `json:"passCount"`
	FailCount int     `json:"failCount"`
	PassRate  float64 `json:"passRate"`
}

type MaturityNsEntry struct {
	Namespace   string `json:"namespace"`
	Workloads   int    `json:"workloads"`
	AvgMaturity int    `json:"avgMaturity"`
	PassDims    int    `json:"passDimensions"`
}

// handleWorkloadMaturityMatrix handles GET /api/docs/workload-maturity-matrix
func (s *Server) handleWorkloadMaturityMatrix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := WorkloadMaturityMatrixResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build lookup maps
	hpaMap := make(map[string]bool) // namespace/name
	for _, hpa := range hpas.Items {
		hpaMap[hpa.Namespace+"/"+hpa.Spec.ScaleTargetRef.Name] = true
	}
	pdbMap := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil {
			for k, v := range pdb.Spec.Selector.MatchLabels {
				pdbMap[pdb.Namespace+"/"+k+"="+v] = true
			}
		}
	}

	// Init dimension counters
	dimNames := []string{"Resource Limits", "Health Probes", "HPA Autoscaling", "PDB Protection", "Multiple Replicas", "Security Context"}
	dimPass := make(map[string]int)
	dimFail := make(map[string]int)
	for _, d := range dimNames {
		dimPass[d] = 0
		dimFail[d] = 0
	}

	nsData := make(map[string]*MaturityNsEntry)

	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		passCount := 0

		// Dim 1: Resource limits
		hasLimits := true
		for _, c := range dep.Spec.Template.Spec.Containers {
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
				hasLimits = false
				break
			}
		}
		if hasLimits {
			dimPass["Resource Limits"]++
			passCount++
		} else {
			dimFail["Resource Limits"]++
		}

		// Dim 2: Health probes
		hasProbes := dep.Spec.Template.Spec.Containers[0].ReadinessProbe != nil && dep.Spec.Template.Spec.Containers[0].LivenessProbe != nil
		if hasProbes {
			dimPass["Health Probes"]++
			passCount++
		} else {
			dimFail["Health Probes"]++
		}

		// Dim 3: HPA
		hasHPA := hpaMap[dep.Namespace+"/"+dep.Name]
		if hasHPA {
			dimPass["HPA Autoscaling"]++
			passCount++
		} else {
			dimFail["HPA Autoscaling"]++
		}

		// Dim 4: PDB
		hasPDB := false
		for k, v := range dep.Spec.Selector.MatchLabels {
			if pdbMap[dep.Namespace+"/"+k+"="+v] {
				hasPDB = true
				break
			}
		}
		if hasPDB {
			dimPass["PDB Protection"]++
			passCount++
		} else {
			dimFail["PDB Protection"]++
		}

		// Dim 5: Multiple replicas
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas >= 2 {
			dimPass["Multiple Replicas"]++
			passCount++
		} else {
			dimFail["Multiple Replicas"]++
		}

		// Dim 6: Security context
		hasSecCtx := dep.Spec.Template.Spec.SecurityContext != nil &&
			dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot != nil &&
			*dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot
		if hasSecCtx {
			dimPass["Security Context"]++
			passCount++
		} else {
			dimFail["Security Context"]++
		}

		switch passCount {
		case 6:
			result.Summary.EliteWorkloads++
		case 5:
			result.Summary.AdvancedWorkloads++
		case 3, 4:
			result.Summary.MidWorkloads++
		default:
			result.Summary.BasicWorkloads++
		}

		if _, ok := nsData[dep.Namespace]; !ok {
			nsData[dep.Namespace] = &MaturityNsEntry{Namespace: dep.Namespace}
		}
		nsData[dep.Namespace].Workloads++
		nsData[dep.Namespace].AvgMaturity += passCount
		nsData[dep.Namespace].PassDims += passCount
	}

	// Build dimension stats
	for _, d := range dimNames {
		total := dimPass[d] + dimFail[d]
		var rate float64
		if total > 0 {
			rate = float64(dimPass[d]) / float64(total) * 100
		}
		result.Dimensions = append(result.Dimensions, MaturityMatrixDimension{
			Name:      d,
			PassCount: dimPass[d],
			FailCount: dimFail[d],
			PassRate:  rate,
		})
	}

	// Build namespace entries
	for _, e := range nsData {
		if e.Workloads > 0 {
			e.AvgMaturity = e.PassDims * 100 / (e.Workloads * 6)
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}

	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgMaturity = float64(result.Summary.EliteWorkloads*6+result.Summary.AdvancedWorkloads*5+result.Summary.MidWorkloads*3) / float64(result.Summary.TotalWorkloads) / 6 * 100
		result.OverallScore = int(result.Summary.AvgMaturity)
	}
	gradeFromScore(&result.Grade, result.OverallScore)

	result.Recommendations = []string{
		fmt.Sprintf("工作负载成熟度: %d 工作负载, %d Elite(6/6), %d Advanced(5/6), %d Mid(3-4/6), %d Basic(0-2/6)",
			result.Summary.TotalWorkloads, result.Summary.EliteWorkloads, result.Summary.AdvancedWorkloads,
			result.Summary.MidWorkloads, result.Summary.BasicWorkloads),
	}
	for _, d := range result.Dimensions {
		if d.PassRate < 50 {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("%s 仅 %.0f%% 通过率", d.Name, d.PassRate))
		}
	}
	writeJSON(w, result)
}
