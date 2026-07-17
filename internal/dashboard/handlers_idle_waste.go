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

// IdleWasteResult quantifies idle resource waste: unused PVs, dangling LBs,
// stopped/terminated workloads, oversized PVCs, and orphaned storage.
type IdleWasteResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         IdleWasteSummary `json:"summary"`
	IdleWorkloads   []IdleWorkload   `json:"idleWorkloads"`
	UnusedVolumes   []UnusedVolume   `json:"unusedVolumes"`
	UnusedServices  []UnusedService  `json:"unusedServices"`
	WasteScore      int              `json:"wasteScore"`
	Grade           string           `json:"grade"`
	EstimatedWaste  CostEstimate     `json:"estimatedWaste"`
	Recommendations []string         `json:"recommendations"`
}

type IdleWasteSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	IdleWorkloadCount int     `json:"idleWorkloadCount"`
	UnusedPVs         int     `json:"unusedPVs"`
	UnusedServices    int     `json:"unusedServices"`
	UnusedLBs         int     `json:"unusedLBs"`
	EstMonthlyWaste   float64 `json:"estMonthlyWaste"`
}

type IdleWorkload struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	CPURequest  string  `json:"cpuRequest"`
	MemRequest  string  `json:"memRequest"`
	IdleReason  string  `json:"idleReason"`
	MonthlyCost float64 `json:"monthlyCost"`
}

type UnusedVolume struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Size        string  `json:"size"`
	Status      string  `json:"status"`
	MonthlyCost float64 `json:"monthlyCost"`
}

type UnusedService struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Type        string  `json:"type"`
	Reason      string  `json:"reason"`
	MonthlyCost float64 `json:"monthlyCost"`
}

type CostEstimate struct {
	IdleWorkloads  float64 `json:"idleWorkloads"`
	UnusedVolumes  float64 `json:"unusedVolumes"`
	UnusedServices float64 `json:"unusedServices"`
	TotalMonthly   float64 `json:"totalMonthly"`
}

// Cost constants (rough cloud averages)
const (
	cpuCostPerCore = 25.0 // $/month per vCPU
	memCostPerGB   = 4.0  // $/month per GB
	lbCost         = 18.0 // $/month per LoadBalancer
	pvcCostPerGB   = 0.10 // $/month per GB storage
)

// handleIdleWaste detects and quantifies idle resource waste.
// GET /api/scalability/idle-waste
func (s *Server) handleIdleWaste(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := IdleWasteResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Check deployments for zero replicas
	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(0)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas == 0 {
			result.Summary.IdleWorkloadCount++
			cpuReq, memReq := sumContainerRequests(dep.Spec.Template.Spec.Containers)
			cost := estimateResourceCost(cpuReq, memReq)
			result.IdleWorkloads = append(result.IdleWorkloads, IdleWorkload{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				CPURequest: fmt.Sprintf("%dm", cpuReq), MemRequest: fmt.Sprintf("%dMi", memReq),
				IdleReason: "scaled to 0 replicas", MonthlyCost: cost,
			})
		}
	}

	// Check statefulsets for zero replicas
	for _, sts := range statefulsets.Items {
		if systemNS[sts.Namespace] {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if replicas == 0 {
			result.Summary.IdleWorkloadCount++
			cpuReq, memReq := sumContainerRequests(sts.Spec.Template.Spec.Containers)
			cost := estimateResourceCost(cpuReq, memReq)
			result.IdleWorkloads = append(result.IdleWorkloads, IdleWorkload{
				Name: sts.Name, Namespace: sts.Namespace, Kind: "StatefulSet",
				CPURequest: fmt.Sprintf("%dm", cpuReq), MemRequest: fmt.Sprintf("%dMi", memReq),
				IdleReason: "scaled to 0 replicas", MonthlyCost: cost,
			})
		}
	}

	// Detect unused PVCs — Bound but no pod mounts them
	mountedPVCs := map[string]bool{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				mountedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	for _, pvc := range pvcs.Items {
		if systemNS[pvc.Namespace] {
			continue
		}
		key := pvc.Namespace + "/" + pvc.Name
		if pvc.Status.Phase == "Bound" && !mountedPVCs[key] {
			result.Summary.UnusedPVs++
			size := "0Gi"
			if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				size = q.String()
			}
			gb := parseStorageGBStr(size)
			cost := gb * pvcCostPerGB
			result.UnusedVolumes = append(result.UnusedVolumes, UnusedVolume{
				Name: pvc.Name, Namespace: pvc.Namespace,
				Size: size, Status: "bound-unmounted", MonthlyCost: cost,
			})
		}
	}

	// Detect LoadBalancer services — cost waste indicators
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		if svc.Spec.Type == "LoadBalancer" {
			result.Summary.UnusedLBs++
			result.Summary.UnusedServices++
			result.UnusedServices = append(result.UnusedServices, UnusedService{
				Name: svc.Name, Namespace: svc.Namespace,
				Type: "LoadBalancer", Reason: "LB service incurs cost even with low traffic",
				MonthlyCost: lbCost,
			})
		}
	}

	// Calculate totals
	for _, iw := range result.IdleWorkloads {
		result.EstimatedWaste.IdleWorkloads += iw.MonthlyCost
	}
	for _, uv := range result.UnusedVolumes {
		result.EstimatedWaste.UnusedVolumes += uv.MonthlyCost
	}
	for _, us := range result.UnusedServices {
		result.EstimatedWaste.UnusedServices += us.MonthlyCost
	}
	result.EstimatedWaste.TotalMonthly = result.EstimatedWaste.IdleWorkloads + result.EstimatedWaste.UnusedVolumes + result.EstimatedWaste.UnusedServices
	result.Summary.EstMonthlyWaste = result.EstimatedWaste.TotalMonthly

	// Score
	wasteRatio := 0.0
	if result.Summary.TotalWorkloads > 0 {
		wasteRatio = float64(result.Summary.IdleWorkloadCount) / float64(result.Summary.TotalWorkloads)
	}
	result.WasteScore = int((1.0 - wasteRatio) * 100)
	result.WasteScore -= result.Summary.UnusedPVs * 5
	if result.WasteScore < 0 {
		result.WasteScore = 0
	}
	result.WasteScore = min(100, result.WasteScore)
	result.Grade = goldenScoreToGrade(result.WasteScore)

	// Sort
	sort.Slice(result.IdleWorkloads, func(i, j int) bool {
		return result.IdleWorkloads[i].MonthlyCost > result.IdleWorkloads[j].MonthlyCost
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Resource efficiency: %d/100 (grade %s) — estimated $%.2f/month waste", result.WasteScore, result.Grade, result.EstimatedWaste.TotalMonthly))
	if result.Summary.IdleWorkloadCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads scaled to 0 — clean up or document as intentionally paused ($%.2f/month)", result.Summary.IdleWorkloadCount, result.EstimatedWaste.IdleWorkloads))
	}
	if result.Summary.UnusedPVs > 0 {
		recs = append(recs, fmt.Sprintf("%d bound PVCs not mounted by any pod — reclaim storage ($%.2f/month)", result.Summary.UnusedPVs, result.EstimatedWaste.UnusedVolumes))
	}
	if result.Summary.UnusedLBs > 0 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer services — each costs ~$%.0f/month even idle", result.Summary.UnusedLBs, lbCost))
	}
	if len(recs) == 1 {
		recs = append(recs, "No significant idle resource waste detected — maintain current efficiency")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}

// sumContainerRequests sums CPU (millicores) and memory (Mi) requests across containers.
func sumContainerRequests(containers []corev1.Container) (cpuMilli int64, memMi int64) {
	for _, c := range containers {
		if c.Resources.Requests != nil {
			if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuMilli += q.MilliValue()
			}
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memMi += q.Value() / (1024 * 1024)
			}
		}
	}
	return
}

// estimateResourceCost converts resource quantities to monthly cost.
func estimateResourceCost(cpuMilli int64, memMi int64) float64 {
	cores := float64(cpuMilli) / 1000.0
	gb := float64(memMi) / 1024.0
	return cores*cpuCostPerCore + gb*memCostPerGB
}

// parseStorageGBStr converts storage string like "10Gi" to GB float.
func parseStorageGBStr(s string) float64 {
	s = strings.TrimSuffix(s, "Gi")
	s = strings.TrimSuffix(s, "G")
	var gb float64
	fmt.Sscanf(s, "%f", &gb)
	return gb
}
