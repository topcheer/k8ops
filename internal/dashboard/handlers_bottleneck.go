package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BottleneckType describes a category of scaling limitation.
type BottleneckType string

const (
	BottleneckNodeSchedulable BottleneckType = "node-schedulable" // Not enough schedulable nodes/capacity
	BottleneckResourceQuota   BottleneckType = "resource-quota"   // Namespace quota near limit
	BottleneckHPAStuck        BottleneckType = "hpa-stuck"        // HPA at max replicas
	BottleneckPDBBlocking     BottleneckType = "pdb-blocking"     // PDB may block voluntary disruption
	BottleneckNodePressure    BottleneckType = "node-pressure"    // Node has disk/memory/CPU pressure
	BottleneckImagePullLimit  BottleneckType = "image-pull-limit" // Large images slow scaling
	BottleneckStorageExhaust  BottleneckType = "storage-exhaust"  // PVC storage class near capacity
)

// BottleneckImpact rates how severely the bottleneck limits scaling.
type BottleneckImpact string

const (
	ImpactCritical BottleneckImpact = "critical" // blocks scaling entirely
	ImpactHigh     BottleneckImpact = "high"     // significantly limits scaling
	ImpactModerate BottleneckImpact = "moderate" // may slow scaling
	ImpactLow      BottleneckImpact = "low"      // minor concern
)

// BottleneckItem describes a single scaling bottleneck.
type BottleneckItem struct {
	Type       BottleneckType   `json:"type"`
	Impact     BottleneckImpact `json:"impact"`
	Resource   string           `json:"resource"` // e.g., node name, namespace, hpa name
	Namespace  string           `json:"namespace,omitempty"`
	Detail     string           `json:"detail"`
	Metric     string           `json:"metric,omitempty"`
	Suggestion string           `json:"suggestion"`
}

// ScalingBottleneckResult is the full scan output.
type ScalingBottleneckResult struct {
	ScannedAt      time.Time             `json:"scannedAt"`
	ClusterSummary ScalingClusterSummary `json:"clusterSummary"`
	Summary        BottleneckSummary     `json:"summary"`
	Bottlenecks    []BottleneckItem      `json:"bottlenecks"`
}

// ScalingClusterSummary provides cluster-level scaling capacity overview.
type ScalingClusterSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	SchedulableNodes    int     `json:"schedulableNodes"`
	UnschedulableNodes  int     `json:"unschedulableNodes"`
	TotalCPUCapacity    string  `json:"totalCPUCapacity"` // in cores
	AllocatableCPU      string  `json:"allocatableCPU"`   // available for new pods
	TotalMemCapacity    string  `json:"totalMemCapacity"` // in Gi
	AllocatableMem      string  `json:"allocatableMem"`   // available for new pods
	PodCapacity         int     `json:"podCapacity"`      // total pod slots
	PodsAllocated       int     `json:"podsAllocated"`
	PodCapacityUsedPct  float64 `json:"podCapacityUsedPct"`
	ScalingHeadroomPods int     `json:"scalingHeadroomPods"` // estimated pods that can still be scheduled
}

// BottleneckSummary aggregates bottleneck statistics.
type BottleneckSummary struct {
	Total    int            `json:"total"`
	ByType   map[string]int `json:"byType"`
	ByImpact map[string]int `json:"byImpact"`
	Blocking int            `json:"blocking"` // critical+high count
}

// handleScalingBottlenecks scans for factors that limit horizontal scaling.
// GET /api/scaling/bottlenecks?namespace=xxx
func (s *Server) handleScalingBottlenecks(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	result := ScalingBottleneckResult{
		ScannedAt: time.Now(),
		Summary: BottleneckSummary{
			ByType:   make(map[string]int),
			ByImpact: make(map[string]int),
		},
	}

	// Gather resources
	nodeList, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	podList, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	quotaList, err := rc.clientset.CoreV1().ResourceQuotas(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	pdbList, err := rc.clientset.PolicyV1().PodDisruptionBudgets(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	hpaList, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	pvcList, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// --- Build cluster capacity summary ---
	var totalCPU, allocatableCPU resource.Quantity
	var totalMem, allocatableMem resource.Quantity
	totalPodCap := 0
	podsAllocated := 0
	schedulable := 0
	unschedulable := 0

	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable {
			unschedulable++
		} else {
			schedulable++
		}
		totalCPU.Add(*node.Status.Capacity.Cpu())
		allocatableCPU.Add(*node.Status.Allocatable.Cpu())
		totalMem.Add(*node.Status.Capacity.Memory())
		allocatableMem.Add(*node.Status.Allocatable.Memory())
		if pods, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			totalPodCap += int(pods.Value())
		}
	}
	podsAllocated = len(podList.Items)
	podUsedPct := 0.0
	if totalPodCap > 0 {
		podUsedPct = float64(podsAllocated) / float64(totalPodCap) * 100
	}

	result.ClusterSummary = ScalingClusterSummary{
		TotalNodes:          len(nodeList.Items),
		SchedulableNodes:    schedulable,
		UnschedulableNodes:  unschedulable,
		TotalCPUCapacity:    fmt.Sprintf("%.1f", float64(totalCPU.MilliValue())/1000),
		AllocatableCPU:      fmt.Sprintf("%.1f", float64(allocatableCPU.MilliValue())/1000),
		TotalMemCapacity:    fmt.Sprintf("%.1f", float64(totalMem.Value())/1024/1024/1024),
		AllocatableMem:      fmt.Sprintf("%.1f", float64(allocatableMem.Value())/1024/1024/1024),
		PodCapacity:         totalPodCap,
		PodsAllocated:       podsAllocated,
		PodCapacityUsedPct:  podUsedPct,
		ScalingHeadroomPods: totalPodCap - podsAllocated,
	}

	// --- Detect node scheduling bottlenecks ---
	for _, node := range nodeList.Items {
		// Unschedulable nodes
		if node.Spec.Unschedulable {
			result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
				Type:       BottleneckNodeSchedulable,
				Impact:     ImpactModerate,
				Resource:   node.Name,
				Detail:     "Node is cordoned (unschedulable). New pods will not be placed here.",
				Metric:     "Unschedulable=true",
				Suggestion: "Run 'kubectl uncordon " + node.Name + "' if the node is healthy and ready for workloads.",
			})
		}
		// Node pressure conditions
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckNodePressure, Impact: ImpactHigh, Resource: node.Name,
					Detail:     fmt.Sprintf("Memory pressure on node %s: %s", node.Name, cond.Message),
					Suggestion: "Investigate memory-hungry pods on this node. Consider adding memory or rebalancing pods.",
				})
			case corev1.NodeDiskPressure:
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckNodePressure, Impact: ImpactHigh, Resource: node.Name,
					Detail:     fmt.Sprintf("Disk pressure on node %s: %s", node.Name, cond.Message),
					Suggestion: "Clean up unused images, logs, or large volumes on this node.",
				})
			case corev1.NodePIDPressure:
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckNodePressure, Impact: ImpactHigh, Resource: node.Name,
					Detail:     fmt.Sprintf("PID pressure on node %s: %s", node.Name, cond.Message),
					Suggestion: "Check for runaway processes or excessive container creation.",
				})
			}
		}
	}

	// Overall cluster pod capacity check
	if totalPodCap > 0 {
		usedPct := float64(podsAllocated) / float64(totalPodCap) * 100
		if usedPct > 90 {
			result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
				Type: BottleneckNodeSchedulable, Impact: ImpactCritical,
				Resource:   "cluster",
				Detail:     fmt.Sprintf("Pod capacity %.0f%% used (%d/%d pods). Cluster cannot scale much further.", usedPct, podsAllocated, totalPodCap),
				Metric:     fmt.Sprintf("%.1f%% pod capacity used", usedPct),
				Suggestion: "Add more nodes or reduce pod count. Consider increasing node instance size or adding nodes.",
			})
		} else if usedPct > 75 {
			result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
				Type: BottleneckNodeSchedulable, Impact: ImpactHigh,
				Resource:   "cluster",
				Detail:     fmt.Sprintf("Pod capacity %.0f%% used (%d/%d pods). Approaching limit.", usedPct, podsAllocated, totalPodCap),
				Metric:     fmt.Sprintf("%.1f%% pod capacity used", usedPct),
				Suggestion: "Monitor pod growth. Plan to add nodes before reaching 90%%.",
			})
		}
	}

	// --- Detect resource quota pressure ---
	for _, quota := range quotaList.Items {
		for resourceName, hard := range quota.Status.Hard {
			used, hasUsed := quota.Status.Used[resourceName]
			if !hasUsed {
				continue
			}
			hardVal := float64(hard.MilliValue())
			usedVal := float64(used.MilliValue())
			if hardVal == 0 {
				continue
			}
			pct := usedVal / hardVal * 100
			if pct >= 90 {
				impact := ImpactCritical
				if pct >= 100 {
					impact = ImpactCritical
				}
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckResourceQuota, Impact: impact,
					Resource: quota.Name, Namespace: quota.Namespace,
					Detail:     fmt.Sprintf("Quota %s in namespace %s: %s at %.0f%% (%s / %s)", quota.Name, quota.Namespace, resourceName, pct, used.String(), hard.String()),
					Metric:     fmt.Sprintf("%s: %.0f%%", resourceName, pct),
					Suggestion: "Increase the quota limit or reduce resource consumption in this namespace. New pods may fail to schedule.",
				})
			} else if pct >= 75 {
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckResourceQuota, Impact: ImpactModerate,
					Resource: quota.Name, Namespace: quota.Namespace,
					Detail:     fmt.Sprintf("Quota %s: %s at %.0f%% (%s / %s)", quota.Name, resourceName, pct, used.String(), hard.String()),
					Metric:     fmt.Sprintf("%s: %.0f%%", resourceName, pct),
					Suggestion: "Monitor quota usage. Consider increasing limits before hitting 90%%.",
				})
			}
		}
	}

	// --- Detect HPA at max replicas (can't scale further) ---
	for _, hpa := range hpaList.Items {
		atMax := hpa.Spec.MaxReplicas > 0 && hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas
		if atMax {
			result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
				Type: BottleneckHPAStuck, Impact: ImpactHigh,
				Resource: hpa.Name, Namespace: hpa.Namespace,
				Detail:     fmt.Sprintf("HPA %s has hit max replicas (%d). Workload cannot auto-scale further.", hpa.Name, hpa.Status.DesiredReplicas),
				Metric:     fmt.Sprintf("desired=%d, max=%d", hpa.Status.DesiredReplicas, hpa.Spec.MaxReplicas),
				Suggestion: fmt.Sprintf("Increase maxReplicas for HPA %s or add more nodes to handle the load.", hpa.Name),
			})
		}
		// HPA with no current metrics (stuck, can't scale) — skip if already at max
		if !atMax && (hpa.Status.CurrentMetrics == nil || len(hpa.Status.CurrentMetrics) == 0) {
			if hpa.Status.DesiredReplicas > 0 {
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckHPAStuck, Impact: ImpactModerate,
					Resource: hpa.Name, Namespace: hpa.Namespace,
					Detail:     fmt.Sprintf("HPA %s has no current metrics. Auto-scaling is not functioning.", hpa.Name),
					Suggestion: "Check that the metrics server is running and the HPA target is correctly configured.",
				})
			}
		}
	}

	// --- Detect PDBs that may block voluntary disruptions ---
	for _, pdb := range pdbList.Items {
		allowed := pdb.Status.DisruptionsAllowed
		if allowed == 0 {
			impact := ImpactHigh
			result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
				Type: BottleneckPDBBlocking, Impact: impact,
				Resource: pdb.Name, Namespace: pdb.Namespace,
				Detail:     fmt.Sprintf("PDB %s allows 0 voluntary disruptions. Node drains and voluntary evictions are blocked.", pdb.Name),
				Metric:     fmt.Sprintf("allowed=%d, current=%d, desired=%d", allowed, pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy),
				Suggestion: "Ensure enough healthy replicas exist. Consider temporarily adjusting the PDB during maintenance.",
			})
		}
	}

	// --- Detect storage capacity pressure ---
	storageByNS := make(map[string]resource.Quantity)
	storageTotalByNS := make(map[string]resource.Quantity)
	for _, pvc := range pvcList.Items {
		if pvc.Spec.Resources.Requests.Storage() != nil {
			req := *pvc.Spec.Resources.Requests.Storage()
			if pvc.Status.Phase == corev1.ClaimBound || pvc.Status.Phase == corev1.ClaimPending {
				cur := storageByNS[pvc.Namespace]
				cur.Add(req)
				storageByNS[pvc.Namespace] = cur
			}
		}
	}
	for _, pvc := range pvcList.Items {
		if pvc.Spec.Resources.Requests.Storage() != nil {
			if pvc.Status.Phase == corev1.ClaimBound || pvc.Status.Phase == corev1.ClaimPending {
				req := *pvc.Spec.Resources.Requests.Storage()
				t := storageTotalByNS[pvc.Namespace]
				t.Add(req)
				storageTotalByNS[pvc.Namespace] = t
			}
		}
	}
	// Skip storage analysis if namespace filter is set and we don't have data
	if nsFilter != "" && len(storageByNS) == 0 {
		// no PVCs in filtered namespace
	} else {
		// We don't have storage class capacity info, so just note high PVC usage per namespace
		for nsName, totalReq := range storageTotalByNS {
			gb := float64(totalReq.Value()) / 1024 / 1024 / 1024
			if gb > 500 {
				result.Bottlenecks = append(result.Bottlenecks, BottleneckItem{
					Type: BottleneckStorageExhaust, Impact: ImpactModerate,
					Resource:   nsName,
					Detail:     fmt.Sprintf("Namespace %s has %.0f Gi of PVC requests. Verify storage backend capacity.", nsName, gb),
					Metric:     fmt.Sprintf("%.0f Gi", gb),
					Suggestion: "Check storage backend capacity. Large PVC requests may exhaust available storage.",
				})
			}
		}
	}

	// --- Sort: impact severity first, then type ---
	sort.Slice(result.Bottlenecks, func(i, j int) bool {
		si := bottleneckImpactRank(result.Bottlenecks[i].Impact)
		sj := bottleneckImpactRank(result.Bottlenecks[j].Impact)
		if si != sj {
			return si < sj
		}
		return result.Bottlenecks[i].Resource < result.Bottlenecks[j].Resource
	})

	// --- Build summary ---
	result.Summary.Total = len(result.Bottlenecks)
	for _, b := range result.Bottlenecks {
		result.Summary.ByType[string(b.Type)]++
		result.Summary.ByImpact[string(b.Impact)]++
		if b.Impact == ImpactCritical || b.Impact == ImpactHigh {
			result.Summary.Blocking++
		}
	}

	writeJSON(w, result)
}

func bottleneckImpactRank(i BottleneckImpact) int {
	switch i {
	case ImpactCritical:
		return 0
	case ImpactHigh:
		return 1
	case ImpactModerate:
		return 2
	case ImpactLow:
		return 3
	}
	return 9
}

// policyv1 import guard
var _ policyv1.PodDisruptionBudget
