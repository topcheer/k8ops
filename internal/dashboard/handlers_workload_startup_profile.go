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

// WorkloadStartupProfileResult profiles workload startup time and initialization patterns.
type WorkloadStartupProfileResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         StartupProfileSummary `json:"summary"`
	ByWorkload      []StartupProfileEntry `json:"byWorkload"`
	SlowStarters    []StartupProfileEntry `json:"slowStarters"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type StartupProfileSummary struct {
	TotalPods        int     `json:"totalPods"`
	AvgAge           float64 `json:"avgPodAgeHours"`
	WithInitCont     int     `json:"withInitContainers"`
	WithStartupProbe int     `json:"withStartupProbe"`
	RecentlyCreated  int     `json:"recentlyCreatedPods"` // < 1hr
	HighRestarts     int     `json:"highRestartsNearStartup"`
}

type StartupProfileEntry struct {
	PodName            string  `json:"podName"`
	Namespace          string  `json:"namespace"`
	AgeHours           float64 `json:"ageHours"`
	ContainerCount     int     `json:"containerCount"`
	HasInit            bool    `json:"hasInitContainers"`
	InitCount          int     `json:"initContainerCount"`
	HasStartup         bool    `json:"hasStartupProbe"`
	RestartNearStartup bool    `json:"restartedNearStartup"`
	RiskLevel          string  `json:"riskLevel"`
}

// handleWorkloadStartupProfile handles GET /api/product/workload-startup-profile
func (s *Server) handleWorkloadStartupProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := WorkloadStartupProfileResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build startup probe map from deployments
	startupProbeMap := make(map[string]bool) // ns/dep -> has startup probe
	initContainerMap := make(map[string]int)
	for _, dep := range deployments.Items {
		key := dep.Namespace + "/" + dep.Name
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.StartupProbe != nil {
				startupProbeMap[key] = true
			}
		}
		initContainerMap[key] = len(dep.Spec.Template.Spec.InitContainers)
	}

	totalAgeHours := 0.0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		ageHours := time.Since(pod.CreationTimestamp.Time).Hours()
		totalAgeHours += ageHours

		entry := StartupProfileEntry{
			PodName:        pod.Name,
			Namespace:      pod.Namespace,
			AgeHours:       ageHours,
			ContainerCount: len(pod.Spec.Containers),
			HasInit:        len(pod.Spec.InitContainers) > 0,
			InitCount:      len(pod.Spec.InitContainers),
		}

		// Derive deployment name for startup probe lookup
		depName := getOwnerDeployName(&pod)
		if depName != "" {
			key := pod.Namespace + "/" + depName
			entry.HasStartup = startupProbeMap[key]
		}

		// Check restart near startup
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 0 && ageHours < 24 {
				entry.RestartNearStartup = true
				result.Summary.HighRestarts++
				break
			}
		}

		// Track stats
		if entry.HasInit {
			result.Summary.WithInitCont++
		}
		if entry.HasStartup {
			result.Summary.WithStartupProbe++
		}
		if ageHours < 1 {
			result.Summary.RecentlyCreated++
		}

		// Risk
		var issues int
		if entry.HasInit && !entry.HasStartup {
			issues++ // init containers without startup probe
		}
		if entry.RestartNearStartup {
			issues++
		}
		if entry.ContainerCount > 3 {
			issues++ // complex startup
		}
		switch {
		case issues >= 2:
			entry.RiskLevel = "high"
			result.SlowStarters = append(result.SlowStarters, entry)
		case issues >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgAge = totalAgeHours / float64(result.Summary.TotalPods)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].AgeHours < result.ByWorkload[j].AgeHours
	})

	if result.Summary.TotalPods > 0 {
		stable := result.Summary.TotalPods - result.Summary.HighRestarts
		result.HealthScore = stable * 100 / result.Summary.TotalPods
		if result.Summary.WithStartupProbe > 0 {
			result.HealthScore += 10
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("工作负载启动画像: %d Pod, 平均运行 %.1f 小时, %d 有 init, %d 有 startup probe, %d 启动后重启",
			result.Summary.TotalPods, result.Summary.AvgAge,
			result.Summary.WithInitCont, result.Summary.WithStartupProbe,
			result.Summary.HighRestarts),
	}
	if result.Summary.HighRestarts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 在启动后 24h 内重启", result.Summary.HighRestarts))
	}
	if result.Summary.WithStartupProbe == 0 {
		result.Recommendations = append(result.Recommendations, "无 Pod 使用 startup probe, 慢启动容器可能被 liveness 误杀")
	}
	writeJSON(w, result)
}

var _ appsv1.DeploymentSpec
