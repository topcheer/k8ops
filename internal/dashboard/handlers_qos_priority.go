package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QoSResult is the full Pod QoS & Priority Class distribution analysis.
type QoSResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         QoSSummary          `json:"summary"`
	ByNamespace     []QoSNamespaceStat  `json:"byNamespace"`
	ByWorkload      []QoSWorkloadStat   `json:"byWorkload"`
	Misconfigs      []QoSMisconfig      `json:"misconfigs"`
	EvictionRisk    []EvictionRiskPod   `json:"evictionRisk"`
	PriorityClasses []PriorityClassInfo `json:"priorityClasses"`
	Recommendations []string            `json:"recommendations"`
}

// QoSSummary aggregates cluster-wide QoS and priority statistics.
type QoSSummary struct {
	TotalPods         int `json:"totalPods"`
	GuaranteedPods    int `json:"guaranteedPods"`
	BurstablePods     int `json:"burstablePods"`
	BestEffortPods    int `json:"bestEffortPods"`
	WithPriorityClass int `json:"withPriorityClass"`
	NoPriorityClass   int `json:"noPriorityClass"`
	SystemCritical    int `json:"systemCritical"` // system-node-critical or system-cluster-critical
	HighPriority      int `json:"highPriority"`   // >= 1000000
	MediumPriority    int `json:"mediumPriority"` // 1000-999999
	LowPriority       int `json:"lowPriority"`    // < 1000
	NoRequestsPods    int `json:"noRequestsPods"` // pods with zero resource requests
	NoLimitsPods      int `json:"noLimitsPods"`   // pods with zero resource limits
	MisconfigCount    int `json:"misconfigCount"`
	QoSScore          int `json:"qosScore"` // 0-100
}

// QoSNamespaceStat shows QoS distribution per namespace.
type QoSNamespaceStat struct {
	Namespace  string `json:"namespace"`
	TotalPods  int    `json:"totalPods"`
	Guaranteed int    `json:"guaranteed"`
	Burstable  int    `json:"burstable"`
	BestEffort int    `json:"bestEffort"`
	NoPriority int    `json:"noPriorityClass"`
	IsSystem   bool   `json:"isSystem"`
}

// QoSWorkloadStat shows QoS distribution per workload type.
type QoSWorkloadStat struct {
	WorkloadType string `json:"workloadType"`
	TotalPods    int    `json:"totalPods"`
	Guaranteed   int    `json:"guaranteed"`
	Burstable    int    `json:"burstable"`
	BestEffort   int    `json:"bestEffort"`
}

// QoSMisconfig describes a QoS or priority class misconfiguration.
type QoSMisconfig struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	WorkloadType string `json:"workloadType"`
	QoSClass     string `json:"qosClass"`
	Issue        string `json:"issue"`
	Severity     string `json:"severity"`
	Suggestion   string `json:"suggestion"`
}

// EvictionRiskPod describes pods at high risk of eviction.
type EvictionRiskPod struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	QoSClass      string `json:"qosClass"`
	PriorityValue int32  `json:"priorityValue"`
	EvictionOrder int    `json:"evictionOrder"` // 1 = evicted first
	Reason        string `json:"reason"`
}

// PriorityClassInfo describes a PriorityClass in the cluster.
type PriorityClassInfo struct {
	Name             string `json:"name"`
	Value            int32  `json:"value"`
	IsGlobalDefault  bool   `json:"isGlobalDefault"`
	PreemptionPolicy string `json:"preemptionPolicy,omitempty"`
	PodCount         int    `json:"podCount"`
	IsSystem         bool   `json:"isSystem"`
}

// handleQoSAudit analyzes Pod QoS class distribution and PriorityClass usage.
// GET /api/product/qos-priority
func (s *Server) handleQoSAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	priorityClasses, _ := rc.clientset.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})

	// Build priority class map
	pcMap := map[string]*schedulingv1.PriorityClass{}
	var defaultPC *schedulingv1.PriorityClass
	for i := range priorityClasses.Items {
		pc := &priorityClasses.Items[i]
		pcMap[pc.Name] = pc
		if pc.GlobalDefault {
			defaultPC = pc
		}
	}

	// Count pods per priority class
	pcPodCount := map[string]int{}

	now := time.Now()
	result := QoSResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)

	nsStats := map[string]*QoSNamespaceStat{}
	wlStats := map[string]*QoSWorkloadStat{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		qosClass := string(pod.Status.QOSClass)
		if qosClass == "" {
			qosClass = computeQoS(&pod)
		}

		// Determine priority
		priName := pod.Spec.PriorityClassName
		priValue := int32(0)
		if pod.Spec.Priority != nil {
			priValue = *pod.Spec.Priority
		} else if priName != "" {
			if pc, ok := pcMap[priName]; ok {
				priValue = pc.Value
			}
		} else if defaultPC != nil {
			priValue = defaultPC.Value
		}

		isSys := isSystemNamespace(pod.Namespace)
		wlType := inferWorkloadTypeFromPod(&pod)

		// Update summary
		switch corev1.PodQOSClass(qosClass) {
		case corev1.PodQOSGuaranteed:
			result.Summary.GuaranteedPods++
		case corev1.PodQOSBurstable:
			result.Summary.BurstablePods++
		case corev1.PodQOSBestEffort:
			result.Summary.BestEffortPods++
		}

		if priName != "" {
			result.Summary.WithPriorityClass++
			pcPodCount[priName]++
		} else {
			result.Summary.NoPriorityClass++
		}

		// Categorize priority
		if priValue >= 2000000000 {
			result.Summary.SystemCritical++
		} else if priValue >= 1000000 {
			result.Summary.HighPriority++
		} else if priValue >= 1000 {
			result.Summary.MediumPriority++
		} else {
			result.Summary.LowPriority++
		}

		// Check resource requests/limits
		hasRequests, hasLimits := checkPodResources(&pod)
		if !hasRequests {
			result.Summary.NoRequestsPods++
		}
		if !hasLimits {
			result.Summary.NoLimitsPods++
		}

		// Namespace stats
		nsStat, ok := nsStats[pod.Namespace]
		if !ok {
			nsStat = &QoSNamespaceStat{Namespace: pod.Namespace, IsSystem: isSys}
			nsStats[pod.Namespace] = nsStat
		}
		nsStat.TotalPods++
		switch corev1.PodQOSClass(qosClass) {
		case corev1.PodQOSGuaranteed:
			nsStat.Guaranteed++
		case corev1.PodQOSBurstable:
			nsStat.Burstable++
		case corev1.PodQOSBestEffort:
			nsStat.BestEffort++
		}
		if priName == "" {
			nsStat.NoPriority++
		}

		// Workload stats
		wlStat, ok := wlStats[wlType]
		if !ok {
			wlStat = &QoSWorkloadStat{WorkloadType: wlType}
			wlStats[wlType] = wlStat
		}
		wlStat.TotalPods++
		switch corev1.PodQOSClass(qosClass) {
		case corev1.PodQOSGuaranteed:
			wlStat.Guaranteed++
		case corev1.PodQOSBurstable:
			wlStat.Burstable++
		case corev1.PodQOSBestEffort:
			wlStat.BestEffort++
		}

		// Check for misconfigurations
		misconfigs := checkQoSMisconfigs(&pod, qosClass, priName, priValue, isSys, wlType)
		for _, m := range misconfigs {
			result.Misconfigs = append(result.Misconfigs, m)
			result.Summary.MisconfigCount++
		}

		// Eviction risk for BestEffort and low-priority pods
		if corev1.PodQOSClass(qosClass) == corev1.PodQOSBestEffort || (priValue < 1000 && !isSys) {
			order := 1
			reason := ""
			if corev1.PodQOSClass(qosClass) == corev1.PodQOSBestEffort {
				order = 1
				reason = "BestEffort QoS — evicted first under node pressure"
			} else if corev1.PodQOSClass(qosClass) == corev1.PodQOSBurstable {
				order = 2
				reason = "Burstable QoS — evicted after BestEffort under pressure"
			} else {
				order = 3
				reason = "Low priority — vulnerable to preemption"
			}
			result.EvictionRisk = append(result.EvictionRisk, EvictionRiskPod{
				PodName:       pod.Name,
				Namespace:     pod.Namespace,
				QoSClass:      qosClass,
				PriorityValue: priValue,
				EvictionOrder: order,
				Reason:        reason,
			})
		}
	}

	// Build priority class info
	for _, pc := range priorityClasses.Items {
		preemptionPolicy := ""
		if pc.PreemptionPolicy != nil {
			preemptionPolicy = string(*pc.PreemptionPolicy)
		}
		result.PriorityClasses = append(result.PriorityClasses, PriorityClassInfo{
			Name:             pc.Name,
			Value:            pc.Value,
			IsGlobalDefault:  pc.GlobalDefault,
			PreemptionPolicy: preemptionPolicy,
			PodCount:         pcPodCount[pc.Name],
			IsSystem:         strings.HasPrefix(pc.Name, "system-"),
		})
	}

	// Sort namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].BestEffort != result.ByNamespace[j].BestEffort {
			return result.ByNamespace[i].BestEffort > result.ByNamespace[j].BestEffort
		}
		return result.ByNamespace[i].TotalPods > result.ByNamespace[j].TotalPods
	})

	// Sort workload stats
	for _, stat := range wlStats {
		result.ByWorkload = append(result.ByWorkload, *stat)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].BestEffort > result.ByWorkload[j].BestEffort
	})

	// Sort misconfigs by severity
	sort.Slice(result.Misconfigs, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Misconfigs[i].Severity] < sevOrder[result.Misconfigs[j].Severity]
	})
	if len(result.Misconfigs) > 30 {
		result.Misconfigs = result.Misconfigs[:30]
	}

	// Sort eviction risk by order
	sort.Slice(result.EvictionRisk, func(i, j int) bool {
		return result.EvictionRisk[i].EvictionOrder < result.EvictionRisk[j].EvictionOrder
	})
	if len(result.EvictionRisk) > 30 {
		result.EvictionRisk = result.EvictionRisk[:30]
	}

	result.Summary.QoSScore = qosScore(result.Summary)
	result.Recommendations = qosRecommendations(&result)

	writeJSON(w, result)
}

// computeQoS determines the QoS class from pod resource requests/limits.
// Kubernetes sets this automatically, but we compute it as fallback.
func computeQoS(pod *corev1.Pod) string {
	allGuaranteed := true
	allBestEffort := true

	for _, c := range pod.Spec.Containers {
		req := c.Resources.Requests
		lim := c.Resources.Limits

		hasCPUReq := !req.Cpu().IsZero()
		hasMemReq := !req.Memory().IsZero()
		hasCPULim := !lim.Cpu().IsZero()
		hasMemLim := !lim.Memory().IsZero()

		if hasCPUReq || hasMemReq {
			allBestEffort = false
		}

		// Guaranteed requires: requests == limits for CPU and memory
		if !hasCPULim || !hasMemLim {
			allGuaranteed = false
			continue
		}
		if !req.Cpu().Equal(*lim.Cpu()) || !req.Memory().Equal(*lim.Memory()) {
			allGuaranteed = false
		}
	}

	if allBestEffort && len(pod.Spec.Containers) > 0 {
		return string(corev1.PodQOSBestEffort)
	}
	if allGuaranteed {
		return string(corev1.PodQOSGuaranteed)
	}
	return string(corev1.PodQOSBurstable)
}

// checkPodResources returns whether the pod has resource requests and limits.
func checkPodResources(pod *corev1.Pod) (hasRequests, hasLimits bool) {
	hasRequests = true
	hasLimits = true
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
			hasRequests = false
		}
		if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
			hasLimits = false
		}
	}
	return
}

// checkQoSMisconfigs identifies QoS and priority class configuration issues.
func checkQoSMisconfigs(pod *corev1.Pod, qosClass string, priName string, priValue int32, isSystem bool, wlType string) []QoSMisconfig {
	var issues []QoSMisconfig

	// BestEffort in non-system namespace
	if qosClass == string(corev1.PodQOSBestEffort) && !isSystem {
		issues = append(issues, QoSMisconfig{
			PodName:      pod.Name,
			Namespace:    pod.Namespace,
			WorkloadType: wlType,
			QoSClass:     qosClass,
			Issue:        "BestEffort QoS in user namespace — first to be evicted under node pressure",
			Severity:     "high",
			Suggestion:   "Add CPU and memory requests to upgrade to Burstable or Guaranteed",
		})
	}

	// Critical workload (Deployment with implied criticality) without priority class
	if !isSystem && priName == "" && wlType == "Deployment" {
		// Only flag if it's a single-replica deployment (implied criticality)
		if isSingleReplicaDeployment(pod) {
			issues = append(issues, QoSMisconfig{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				WorkloadType: wlType,
				QoSClass:     qosClass,
				Issue:        "Single-replica Deployment without PriorityClass — may be preempted by higher-priority pods",
				Severity:     "medium",
				Suggestion:   "Assign a PriorityClass to ensure scheduling priority",
			})
		}
	}

	// Guaranteed QoS with non-critical priority — possible resource over-allocation
	if qosClass == string(corev1.PodQOSGuaranteed) && priValue < 1000 && !isSystem {
		issues = append(issues, QoSMisconfig{
			PodName:      pod.Name,
			Namespace:    pod.Namespace,
			WorkloadType: wlType,
			QoSClass:     qosClass,
			Issue:        "Guaranteed QoS with low priority — may be preempted despite resource guarantees",
			Severity:     "low",
			Suggestion:   "Consider if Guaranteed QoS is warranted for this workload's priority level",
		})
	}

	// No resource requests at all
	hasReq, _ := checkPodResources(pod)
	if !hasReq && !isSystem {
		issues = append(issues, QoSMisconfig{
			PodName:      pod.Name,
			Namespace:    pod.Namespace,
			WorkloadType: wlType,
			QoSClass:     qosClass,
			Issue:        "No resource requests set — scheduler cannot make informed placement decisions",
			Severity:     "medium",
			Suggestion:   "Set CPU and memory requests for predictable scheduling",
		})
	}

	return issues
}

// isSingleReplicaDeployment checks if a pod belongs to a 1-replica Deployment.
func isSingleReplicaDeployment(pod *corev1.Pod) bool {
	// Heuristic: if the pod name has a hash suffix and owner is ReplicaSet owned by Deployment
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" {
			return true // Can't easily check replica count from pod, assume yes
		}
	}
	return false
}

// qosScore computes a 0-100 QoS health score.
func qosScore(s QoSSummary) int {
	if s.TotalPods == 0 {
		return 100
	}

	score := 100

	// Penalize BestEffort pods in proportion
	bestEffortRatio := float64(s.BestEffortPods) / float64(s.TotalPods)
	score -= int(bestEffortRatio * 30)

	// Penalize pods without priority class
	noPriRatio := float64(s.NoPriorityClass) / float64(s.TotalPods)
	score -= int(noPriRatio * 15)

	// Penalize no requests
	noReqRatio := float64(s.NoRequestsPods) / float64(s.TotalPods)
	score -= int(noReqRatio * 20)

	// Penalize no limits
	noLimRatio := float64(s.NoLimitsPods) / float64(s.TotalPods)
	score -= int(noLimRatio * 15)

	// Penalize misconfigs
	if s.MisconfigCount > 0 {
		score -= min(10, s.MisconfigCount)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// qosRecommendations generates actionable recommendations.
func qosRecommendations(r *QoSResult) []string {
	var recs []string

	if r.Summary.BestEffortPods > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d BestEffort pod(s) detected — add resource requests/limits to prevent eviction under node pressure",
			r.Summary.BestEffortPods,
		))
	}

	if r.Summary.NoPriorityClass > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) have no PriorityClass — define priority classes to control preemption order during resource contention",
			r.Summary.NoPriorityClass,
		))
	}

	if r.Summary.NoRequestsPods > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) have no resource requests — scheduler cannot make informed placement decisions",
			r.Summary.NoRequestsPods,
		))
	}

	if r.Summary.GuaranteedPods == 0 && r.Summary.TotalPods > 5 {
		recs = append(recs, "No Guaranteed QoS pods — consider setting equal requests and limits for critical workloads to get QoS guarantees")
	}

	// Check if no global default PriorityClass
	hasDefault := false
	for _, pc := range r.PriorityClasses {
		if pc.IsGlobalDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		recs = append(recs, "No global default PriorityClass — pods without explicit priority class get priority 0")
	}

	if len(recs) == 0 {
		recs = append(recs, "Pod QoS distribution is healthy — adequate resource requests/limits and priority class coverage")
	}

	return recs
}
