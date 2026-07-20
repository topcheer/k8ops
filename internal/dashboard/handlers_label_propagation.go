package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelPropagationResult audits label consistency and propagation across workloads and services.
type LabelPropagationResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         LabelPropSummary   `json:"summary"`
	ByNamespace     []LabelPropNSEntry `json:"byNamespace"`
	Inconsistencies []LabelPropEntry   `json:"inconsistencies"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type LabelPropSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithLabels        int `json:"withLabels"`
	WithoutLabels     int `json:"withoutLabels"`
	StandardLabels    int `json:"withStandardLabels"`
	InconsistentSvc   int `json:"inconsistentServices"`
	OrphanedSelectors int `json:"orphanedSelectors"`
}

type LabelPropNSEntry struct {
	Namespace     string  `json:"namespace"`
	WorkloadCount int     `json:"workloadCount"`
	WithLabels    int     `json:"withLabels"`
	CoveragePct   float64 `json:"coveragePct"`
}

type LabelPropEntry struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// Standard labels per Kubernetes recommendations
var standardLabelKeys = []string{"app", "app.kubernetes.io/name", "app.kubernetes.io/instance", "app.kubernetes.io/version", "app.kubernetes.io/managed-by"}

// handleLabelPropagation handles GET /api/product/label-propagation
func (s *Server) handleLabelPropagation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := LabelPropagationResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Build deployment label map per namespace
	depLabels := make(map[string]map[string]map[string]string) // ns -> depName -> labels
	nsMap := make(map[string]*LabelPropNSEntry)

	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		labels := dep.Spec.Template.Labels // pod template labels
		if len(labels) == 0 {
			labels = dep.Labels
		}

		if nsMap[dep.Namespace] == nil {
			nsMap[dep.Namespace] = &LabelPropNSEntry{Namespace: dep.Namespace}
		}
		nsMap[dep.Namespace].WorkloadCount++

		if len(labels) > 0 {
			result.Summary.WithLabels++
			nsMap[dep.Namespace].WithLabels++
		} else {
			result.Summary.WithoutLabels++
			result.Inconsistencies = append(result.Inconsistencies, LabelPropEntry{
				Type: "Deployment", Name: dep.Name, Namespace: dep.Namespace,
				Issue: "no labels on pod template", Severity: "high",
			})
			continue
		}

		// Check for standard labels
		hasStandard := false
		for _, sl := range standardLabelKeys {
			if _, ok := labels[sl]; ok {
				hasStandard = true
				break
			}
		}
		if hasStandard {
			result.Summary.StandardLabels++
		}

		if depLabels[dep.Namespace] == nil {
			depLabels[dep.Namespace] = make(map[string]map[string]string)
		}
		depLabels[dep.Namespace][dep.Name] = labels
	}

	// Check service selector consistency
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) || svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
			continue
		}
		// Check if any deployment matches this selector
		matched := false
		deps := depLabels[svc.Namespace]
		for _, labels := range deps {
			matchCount := 0
			for k, v := range svc.Spec.Selector {
				if labels[k] == v {
					matchCount++
				}
			}
			if matchCount == len(svc.Spec.Selector) {
				matched = true
				break
			}
		}
		if !matched {
			result.Summary.OrphanedSelectors++
			result.Inconsistencies = append(result.Inconsistencies, LabelPropEntry{
				Type: "Service", Name: svc.Name, Namespace: svc.Namespace,
				Issue: fmt.Sprintf("selector %v matches no deployment", svc.Spec.Selector), Severity: "medium",
			})
		}

		// Check label naming consistency (app vs app.kubernetes.io/name)
		hasOldStyle := false
		hasNewStyle := false
		for k := range svc.Spec.Selector {
			if k == "app" || k == "app.kubernetes.io/name" {
				if strings.HasPrefix(k, "app.kubernetes.io") {
					hasNewStyle = true
				} else {
					hasOldStyle = true
				}
			}
		}
		if hasOldStyle && !hasNewStyle {
			result.Summary.InconsistentSvc++
		}
	}

	for _, e := range nsMap {
		if e.WorkloadCount > 0 {
			e.CoveragePct = float64(e.WithLabels) / float64(e.WorkloadCount) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.WithLabels * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("标签传播审计: %d 工作负载, %d 有标签, %d 标准标签, %d 孤立选择器",
			result.Summary.TotalWorkloads, result.Summary.WithLabels,
			result.Summary.StandardLabels, result.Summary.OrphanedSelectors),
	}
	if result.Summary.WithoutLabels > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作负载无标签, 无法被 Service/监控选择", result.Summary.WithoutLabels))
	}
	if result.Summary.OrphanedSelectors > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Service 选择器无匹配工作负载", result.Summary.OrphanedSelectors))
	}
	writeJSON(w, result)
}
