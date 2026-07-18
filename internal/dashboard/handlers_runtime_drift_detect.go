package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeDriftDetectResult detects configuration drift between deployed
// pod specs and their controller templates (Deployment/StatefulSet).
type RuntimeDriftDetectResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         RuntimeDriftSummary `json:"summary"`
	Drifts          []RuntimeDriftEntry `json:"drifts"`
	DriftScore      int                 `json:"driftScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type RuntimeDriftSummary struct {
	TotalPods      int `json:"totalPods"`
	DriftedPods    int `json:"driftedPods"`
	ImageDrifts    int `json:"imageDrifts"`
	EnvDrifts      int `json:"envDrifts"`
	ResourceDrifts int `json:"resourceDrifts"`
	VolumeDrifts   int `json:"volumeDrifts"`
}

type RuntimeDriftEntry struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	DriftType string `json:"driftType"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
}

// handleRuntimeDriftDetect handles GET /api/security/runtime-drift-detect
func (s *Server) handleRuntimeDriftDetect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RuntimeDriftDetectResult{ScannedAt: time.Now()}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build deployment template lookup: ns/name -> pod spec template
	type depTemplate struct {
		containers map[string]corev1.Container
	}
	depMap := make(map[string]depTemplate)
	for _, d := range deployments.Items {
		key := d.Namespace + "/" + d.Name
		ct := make(map[string]corev1.Container)
		for _, c := range d.Spec.Template.Spec.Containers {
			ct[c.Name] = c
		}
		depMap[key] = depTemplate{containers: ct}
	}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		tmpl, ok := depMap[key]
		if !ok {
			continue
		}

		for _, podContainer := range pod.Spec.Containers {
			depContainer, exists := tmpl.containers[podContainer.Name]
			if !exists {
				continue
			}

			// Image drift
			if podContainer.Image != depContainer.Image {
				result.Summary.ImageDrifts++
				result.Summary.DriftedPods++
				result.Drifts = append(result.Drifts, RuntimeDriftEntry{
					PodName: pod.Name, Namespace: pod.Namespace, Workload: wlName,
					DriftType: "image", Severity: "high",
					Detail: fmt.Sprintf("pod=%s vs template=%s", podContainer.Image, depContainer.Image),
				})
			}

			// Env drift (count difference)
			if len(podContainer.Env) != len(depContainer.Env) {
				result.Summary.EnvDrifts++
				if !containsPodDrift(result.Drifts, pod.Name, "env") {
					result.Summary.DriftedPods++
					result.Drifts = append(result.Drifts, RuntimeDriftEntry{
						PodName: pod.Name, Namespace: pod.Namespace, Workload: wlName,
						DriftType: "env", Severity: "medium",
						Detail: fmt.Sprintf("pod has %d env vars, template has %d", len(podContainer.Env), len(depContainer.Env)),
					})
				}
			}

			// Resource drift
			podCPU := getContainerCPU(podContainer)
			depCPU := getContainerCPU(depContainer)
			if podCPU != depCPU {
				result.Summary.ResourceDrifts++
				if !containsPodDrift(result.Drifts, pod.Name, "resource") {
					result.Summary.DriftedPods++
					result.Drifts = append(result.Drifts, RuntimeDriftEntry{
						PodName: pod.Name, Namespace: pod.Namespace, Workload: wlName,
						DriftType: "resource", Severity: "medium",
						Detail: fmt.Sprintf("CPU request differs: pod=%.3f template=%.3f", podCPU, depCPU),
					})
				}
			}
		}
	}

	sort.Slice(result.Drifts, func(i, j int) bool {
		sevRank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevRank[result.Drifts[i].Severity] < sevRank[result.Drifts[j].Severity]
	})

	if result.Summary.TotalPods > 0 {
		cleanRatio := float64(result.Summary.TotalPods-result.Summary.DriftedPods) / float64(result.Summary.TotalPods)
		result.DriftScore = int(cleanRatio * 100)
	}
	gradeFromScore(&result.Grade, result.DriftScore)

	result.Recommendations = []string{
		fmt.Sprintf("运行时漂移: %d/%d Pod 存在漂移 (%d 镜像, %d 环境变量, %d 资源)", result.Summary.DriftedPods, result.Summary.TotalPods, result.Summary.ImageDrifts, result.Summary.EnvDrifts, result.Summary.ResourceDrifts),
	}
	if result.Summary.ImageDrifts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("警告: %d 个 Pod 镜像与模板不一致", result.Summary.ImageDrifts))
	}
	if result.DriftScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 检查是否有手动 kubectl edit 修改, 确保所有变更通过 GitOps")
	}
	writeJSON(w, result)
}

func getContainerCPU(c corev1.Container) float64 {
	if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
		return req.AsApproximateFloat64()
	}
	return 0
}

func containsPodDrift(drifts []RuntimeDriftEntry, podName, driftType string) bool {
	for _, d := range drifts {
		if d.PodName == podName && d.DriftType == driftType {
			return true
		}
	}
	return false
}

func gradeFromScore(grade *string, score int) {
	switch {
	case score >= 80:
		*grade = "A"
	case score >= 60:
		*grade = "B"
	case score >= 40:
		*grade = "C"
	case score >= 20:
		*grade = "D"
	default:
		*grade = "F"
	}
}

// avoid unused import warning
var _ = strings.Contains
