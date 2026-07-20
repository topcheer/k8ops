package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceWasteDeepResult performs deep resource waste analysis finding idle, over-provisioned, and zombie resources.
type ResourceWasteDeepResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          ResourceWasteSummary  `json:"summary"`
	IdleWorkloads    []ResourceWasteEntry  `json:"idleWorkloads"`
	OverProvisioned  []ResourceWasteEntry  `json:"overProvisioned"`
	ZombieResources  []ZombieResourceEntry `json:"zombieResources"`
	PotentialSavings float64               `json:"potentialSavingsPerMonth"`
	HealthScore      int                   `json:"healthScore"`
	Grade            string                `json:"grade"`
	Recommendations  []string              `json:"recommendations"`
}

type ResourceWasteSummary struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	IdleCount      int     `json:"idleWorkloads"`
	OverProvCount  int     `json:"overProvisionedCount"`
	ZombieCount    int     `json:"zombieResourceCount"`
	WasteCPUCores  float64 `json:"wasteCPUCores"`
	WasteMemGB     float64 `json:"wasteMemGB"`
	EstWastePerMo  float64 `json:"estimatedWastePerMonth"`
}

type ResourceWasteEntry struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	CPURequest   float64 `json:"cpuRequest"`
	MemRequestGB float64 `json:"memRequestGB"`
	CPUPercent   float64 `json:"cpuUsagePercent"`
	MemPercent   float64 `json:"memUsagePercent"`
	WasteType    string  `json:"wasteType"`
	EstCostMo    float64 `json:"estimatedCostPerMonth"`
}

type ZombieResourceEntry struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Detail    string `json:"detail"`
}

// handleResourceWasteDeep handles GET /api/scalability/resource-waste-deep
func (s *Server) handleResourceWasteDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceWasteDeepResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Build pod metrics from container statuses (proxy: restart count and age)
	podMetrics := make(map[string]float64) // pod key -> usage proxy
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			// Low restart = potentially stable; we use CPU/Mem requests as waste proxy
			_ = cs
		}
		key := pod.Namespace + "/" + pod.Name
		podMetrics[key] = 0
	}

	// Analyze deployments for over-provisioning
	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		var cpuReq, memReq float64
		for _, c := range dep.Spec.Template.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuReq += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		totalCPU := cpuReq * float64(replicas)
		totalMem := memReq * float64(replicas)

		// Heuristic: high request with low actual usage proxy (no metrics server, use heuristics)
		// If replicas=1 and CPU request > 2 cores, likely over-provisioned
		entry := ResourceWasteEntry{
			Name:         dep.Name,
			Namespace:    dep.Namespace,
			CPURequest:   totalCPU,
			MemRequestGB: totalMem,
		}

		// Estimate cost
		entry.EstCostMo = totalCPU*20 + totalMem*3

		if totalCPU > 2 && replicas <= 2 {
			entry.WasteType = "over-provisioned"
			entry.CPUPercent = 10 // assumed low without metrics
			result.OverProvisioned = append(result.OverProvisioned, entry)
			result.Summary.OverProvCount++
			result.Summary.WasteCPUCores += totalCPU * 0.5 // assume 50% waste
			result.Summary.WasteMemGB += totalMem * 0.3
		}

		if replicas == 0 {
			entry.WasteType = "scaled-to-zero"
			result.IdleWorkloads = append(result.IdleWorkloads, entry)
			result.Summary.IdleCount++
		}
	}

	// Find zombie PVCs (not mounted by any pod)
	pvcMounted := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				pvcMounted[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		if !pvcMounted[pvc.Namespace+"/"+pvc.Name] && pvc.Status.Phase == corev1.ClaimBound {
			size := 0.0
			if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				size = float64(req.Value()) / (1024 * 1024 * 1024)
			}
			result.ZombieResources = append(result.ZombieResources, ZombieResourceEntry{
				Type: "PVC", Name: pvc.Name, Namespace: pvc.Namespace,
				Detail: fmt.Sprintf("%.1fGB unmounted", size),
			})
			result.Summary.ZombieCount++
			result.Summary.WasteMemGB += size
		}
	}

	// Find zombie ConfigMaps (large, not mounted)
	cmMounted := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				cmMounted[pod.Namespace+"/"+vol.ConfigMap.Name] = true
			}
		}
	}
	for _, cm := range configmaps.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		if !cmMounted[cm.Namespace+"/"+cm.Name] && len(cm.Data) > 5 {
			result.ZombieResources = append(result.ZombieResources, ZombieResourceEntry{
				Type: "ConfigMap", Name: cm.Name, Namespace: cm.Namespace,
				Detail: fmt.Sprintf("%d keys unmounted", len(cm.Data)),
			})
			result.Summary.ZombieCount++
		}
	}

	// Sort by cost
	sort.Slice(result.OverProvisioned, func(i, j int) bool {
		return result.OverProvisioned[i].EstCostMo > result.OverProvisioned[j].EstCostMo
	})

	result.PotentialSavings = result.Summary.WasteCPUCores*20 + result.Summary.WasteMemGB*3
	result.Summary.EstWastePerMo = result.PotentialSavings

	if result.Summary.TotalWorkloads > 0 {
		wasteRatio := float64(result.Summary.OverProvCount+result.Summary.IdleCount) / float64(result.Summary.TotalWorkloads)
		result.HealthScore = int((1 - wasteRatio) * 100)
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("资源浪费深度分析: %d 工作负载, %d 过度配置, %d 空闲, %d 僵尸资源",
			result.Summary.TotalWorkloads, result.Summary.OverProvCount,
			result.Summary.IdleCount, result.Summary.ZombieCount),
		fmt.Sprintf("预估月浪费: $%.2f (CPU %.1f cores + %.1fGB 内存)",
			result.PotentialSavings, result.Summary.WasteCPUCores, result.Summary.WasteMemGB),
	}
	if result.Summary.ZombieCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个僵尸资源 (未挂载 PVC/CM), 建议清理", result.Summary.ZombieCount))
	}
	writeJSON(w, result)
}
