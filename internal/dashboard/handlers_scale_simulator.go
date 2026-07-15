package dashboard

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScaleSimResult is the workload scaling impact simulation.
type ScaleSimResult struct {
	ScannedAt      time.Time         `json:"scannedAt"`
	Input          ScaleSimInput     `json:"input"`
	CurrentState   ScaleSimState     `json:"currentState"`
	SimulatedState ScaleSimState     `json:"simulatedState"`
	Delta          ScaleSimDelta     `json:"delta"`
	Checks         []ScaleSimCheck   `json:"checks"`
	Blockers       []ScaleSimBlocker `json:"blockers,omitempty"`
	Verdict        string            `json:"verdict"` // can-scale, risky, cannot-scale
	Suggestions    []string          `json:"suggestions"`
}

// ScaleSimInput holds the simulation parameters.
type ScaleSimInput struct {
	Workload       string `json:"workload"`
	Namespace      string `json:"namespace"`
	TargetReplicas int    `json:"targetReplicas"`
}

// ScaleSimState captures resource state at a replica count.
type ScaleSimState struct {
	Replicas      int     `json:"replicas"`
	TotalCPU_mCPU int64   `json:"totalCPU_mCPU"` // milli-cores
	TotalMem_MB   float64 `json:"totalMem_MB"`
	TotalPods     int     `json:"totalPods"`
}

// ScaleSimDelta shows the resource change.
type ScaleSimDelta struct {
	ReplicaDelta int     `json:"replicaDelta"`
	CPUDelta     int64   `json:"cpuDelta_mCPU"`
	MemDelta     float64 `json:"memDelta_MB"`
	PodDelta     int     `json:"podDelta"`
	PctIncrease  float64 `json:"pctIncrease"`
}

// ScaleSimCheck is a single feasibility check result.
type ScaleSimCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, warn, fail
	Detail string `json:"detail"`
}

// ScaleSimBlocker describes a scaling blocker.
type ScaleSimBlocker struct {
	Type     string `json:"type"` // quota, node-capacity, pdb, affinity, image-pull
	Detail   string `json:"detail"`
	Resource string `json:"resource,omitempty"`
}

// handleScaleSimulator simulates the impact of scaling a workload.
// GET /api/scalability/scale-simulator?workload=xxx&namespace=xxx&replicas=N
func (s *Server) handleScaleSimulator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	wlName := r.URL.Query().Get("workload")
	wlNamespace := r.URL.Query().Get("namespace")
	replicasStr := r.URL.Query().Get("replicas")

	if wlName == "" {
		writeError(w, http.StatusBadRequest, "workload parameter is required")
		return
	}
	if wlNamespace == "" {
		wlNamespace = "default"
	}
	targetReplicas, err := strconv.Atoi(replicasStr)
	if err != nil || targetReplicas < 0 {
		writeError(w, http.StatusBadRequest, "replicas parameter must be a non-negative integer")
		return
	}

	result := ScaleSimResult{
		ScannedAt: time.Now(),
		Input: ScaleSimInput{
			Workload:       wlName,
			Namespace:      wlNamespace,
			TargetReplicas: targetReplicas,
		},
	}

	// 1. Find the workload and get per-pod resource requests
	var currentReplicas int
	var perPodCPU int64   // milli-cores
	var perPodMem float64 // MB
	var foundWorkload bool

	deployments, err := rc.clientset.AppsV1().Deployments(wlNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range deployments.Items {
			if d.Name != wlName {
				continue
			}
			foundWorkload = true
			currentReplicas = 1
			if d.Spec.Replicas != nil {
				currentReplicas = int(*d.Spec.Replicas)
			}
			for _, c := range d.Spec.Template.Spec.Containers {
				perPodCPU += c.Resources.Requests.Cpu().MilliValue()
				perPodMem += float64(c.Resources.Requests.Memory().Value()) / 1e6
			}
			break
		}
	}

	if !foundWorkload {
		stss, err := rc.clientset.AppsV1().StatefulSets(wlNamespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, sts := range stss.Items {
				if sts.Name != wlName {
					continue
				}
				foundWorkload = true
				currentReplicas = 1
				if sts.Spec.Replicas != nil {
					currentReplicas = int(*sts.Spec.Replicas)
				}
				for _, c := range sts.Spec.Template.Spec.Containers {
					perPodCPU += c.Resources.Requests.Cpu().MilliValue()
					perPodMem += float64(c.Resources.Requests.Memory().Value()) / 1e6
				}
				break
			}
		}
	}

	if !foundWorkload {
		writeError(w, http.StatusNotFound, fmt.Sprintf("workload %s/%s not found", wlNamespace, wlName))
		return
	}

	// 2. Calculate current and simulated states
	result.CurrentState = ScaleSimState{
		Replicas:      currentReplicas,
		TotalCPU_mCPU: int64(currentReplicas) * perPodCPU,
		TotalMem_MB:   float64(currentReplicas) * perPodMem,
		TotalPods:     currentReplicas,
	}
	result.SimulatedState = ScaleSimState{
		Replicas:      targetReplicas,
		TotalCPU_mCPU: int64(targetReplicas) * perPodCPU,
		TotalMem_MB:   float64(targetReplicas) * perPodMem,
		TotalPods:     targetReplicas,
	}
	result.Delta = ScaleSimDelta{
		ReplicaDelta: targetReplicas - currentReplicas,
		CPUDelta:     int64(targetReplicas-currentReplicas) * perPodCPU,
		MemDelta:     float64(targetReplicas-currentReplicas) * perPodMem,
		PodDelta:     targetReplicas - currentReplicas,
	}
	if currentReplicas > 0 {
		result.Delta.PctIncrease = float64(targetReplicas-currentReplicas) / float64(currentReplicas) * 100
	}

	// 3. Run feasibility checks
	var checks []ScaleSimCheck
	var blockers []ScaleSimBlocker

	// Check A: Node capacity
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		totalAllocatableCPU := int64(0)
		totalAllocatableMem := float64(0)
		for _, node := range nodes.Items {
			totalAllocatableCPU += node.Status.Allocatable.Cpu().MilliValue()
			totalAllocatableMem += float64(node.Status.Allocatable.Memory().Value()) / 1e6
		}

		// Get current cluster-wide usage from pods
		allPods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err == nil {
			usedCPU := int64(0)
			usedMem := float64(0)
			for _, pod := range allPods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					continue
				}
				for _, c := range pod.Spec.Containers {
					usedCPU += c.Resources.Requests.Cpu().MilliValue()
					usedMem += float64(c.Resources.Requests.Memory().Value()) / 1e6
				}
			}

			// Simulated usage = current usage + delta
			simUsedCPU := usedCPU + result.Delta.CPUDelta
			simUsedMem := usedMem + result.Delta.MemDelta

			if simUsedCPU > totalAllocatableCPU {
				blockers = append(blockers, ScaleSimBlocker{
					Type:     "node-capacity",
					Detail:   fmt.Sprintf("Insufficient CPU: need %d mCPU but only %d mCPU allocatable (%d mCPU used)", simUsedCPU, totalAllocatableCPU, usedCPU),
					Resource: "cpu",
				})
				checks = append(checks, ScaleSimCheck{
					Name:   "Node CPU Capacity",
					Status: "fail",
					Detail: fmt.Sprintf("Simulated CPU usage %d/%d mCPU (%.1f%%) exceeds cluster capacity", simUsedCPU, totalAllocatableCPU, float64(simUsedCPU)/float64(totalAllocatableCPU)*100),
				})
			} else {
				pct := float64(simUsedCPU) / float64(totalAllocatableCPU) * 100
				status := "pass"
				if pct > 80 {
					status = "warn"
				}
				checks = append(checks, ScaleSimCheck{
					Name:   "Node CPU Capacity",
					Status: status,
					Detail: fmt.Sprintf("Simulated CPU usage %d/%d mCPU (%.1f%%)", simUsedCPU, totalAllocatableCPU, pct),
				})
			}

			if simUsedMem > totalAllocatableMem {
				blockers = append(blockers, ScaleSimBlocker{
					Type:     "node-capacity",
					Detail:   fmt.Sprintf("Insufficient memory: need %.0f MB but only %.0f MB allocatable (%.0f MB used)", simUsedMem, totalAllocatableMem, usedMem),
					Resource: "memory",
				})
				checks = append(checks, ScaleSimCheck{
					Name:   "Node Memory Capacity",
					Status: "fail",
					Detail: fmt.Sprintf("Simulated memory usage %.0f/%.0f MB (%.1f%%) exceeds capacity", simUsedMem, totalAllocatableMem, simUsedMem/totalAllocatableMem*100),
				})
			} else {
				pct := simUsedMem / totalAllocatableMem * 100
				status := "pass"
				if pct > 80 {
					status = "warn"
				}
				checks = append(checks, ScaleSimCheck{
					Name:   "Node Memory Capacity",
					Status: status,
					Detail: fmt.Sprintf("Simulated memory usage %.0f/%.0f MB (%.1f%%)", simUsedMem, totalAllocatableMem, pct),
				})
			}
		}
	}

	// Check B: Namespace ResourceQuota
	quotas, err := rc.clientset.CoreV1().ResourceQuotas(wlNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, rq := range quotas.Items {
			// Check CPU quota
			cpuHard := rq.Status.Hard[corev1.ResourceRequestsCPU]
			if cpuHard.MilliValue() > 0 {
				cpuUsed := rq.Status.Used[corev1.ResourceRequestsCPU]
				usedCPU := cpuUsed.MilliValue()
				simUsedCPU := usedCPU + result.Delta.CPUDelta
				hardCPU := cpuHard.MilliValue()
				if simUsedCPU > hardCPU {
					blockers = append(blockers, ScaleSimBlocker{
						Type:     "quota",
						Detail:   fmt.Sprintf("ResourceQuota %s: CPU requests %d/%d mCPU would exceed quota", rq.Name, simUsedCPU, hardCPU),
						Resource: "cpu-quota",
					})
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s CPU", rq.Name),
						Status: "fail",
						Detail: fmt.Sprintf("CPU requests %d/%d mCPU — exceeds quota", simUsedCPU, hardCPU),
					})
				} else {
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s CPU", rq.Name),
						Status: "pass",
						Detail: fmt.Sprintf("CPU requests %d/%d mCPU (%.1f%%)", simUsedCPU, hardCPU, float64(simUsedCPU)/float64(hardCPU)*100),
					})
				}
			}

			// Check memory quota
			memHard := rq.Status.Hard[corev1.ResourceRequestsMemory]
			if memHard.Value() > 0 {
				memUsed := rq.Status.Used[corev1.ResourceRequestsMemory]
				usedMem := float64(memUsed.Value()) / 1e6
				simUsedMem := usedMem + result.Delta.MemDelta
				hardMem := float64(memHard.Value()) / 1e6
				if simUsedMem > hardMem {
					blockers = append(blockers, ScaleSimBlocker{
						Type:     "quota",
						Detail:   fmt.Sprintf("ResourceQuota %s: memory requests %.0f/%.0f MB would exceed quota", rq.Name, simUsedMem, hardMem),
						Resource: "memory-quota",
					})
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s Memory", rq.Name),
						Status: "fail",
						Detail: fmt.Sprintf("Memory requests %.0f/%.0f MB — exceeds quota", simUsedMem, hardMem),
					})
				} else {
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s Memory", rq.Name),
						Status: "pass",
						Detail: fmt.Sprintf("Memory requests %.0f/%.0f MB", simUsedMem, hardMem),
					})
				}
			}

			// Check pod count quota
			podHardQ := rq.Status.Hard[corev1.ResourcePods]
			if podHardQ.Value() > 0 {
				podUsedQ := rq.Status.Used[corev1.ResourcePods]
				usedPods := podUsedQ.Value()
				simPods := usedPods + int64(result.Delta.PodDelta)
				hardPods := podHardQ.Value()
				if simPods > hardPods {
					blockers = append(blockers, ScaleSimBlocker{
						Type:     "quota",
						Detail:   fmt.Sprintf("ResourceQuota %s: pod count %d/%d would exceed quota", rq.Name, simPods, hardPods),
						Resource: "pod-quota",
					})
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s Pods", rq.Name),
						Status: "fail",
						Detail: fmt.Sprintf("Pod count %d/%d — exceeds quota", simPods, hardPods),
					})
				} else {
					checks = append(checks, ScaleSimCheck{
						Name:   fmt.Sprintf("Quota: %s Pods", rq.Name),
						Status: "pass",
						Detail: fmt.Sprintf("Pod count %d/%d", simPods, hardPods),
					})
				}
			}
		}
	}

	// Check C: PDB (voluntary disruption constraint)
	pdbs, err := rc.clientset.PolicyV1().PodDisruptionBudgets(wlNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pdb := range pdbs.Items {
			if pdb.Spec.Selector == nil {
				continue
			}
			// Simplified: just note that PDB exists for this namespace
			checks = append(checks, ScaleSimCheck{
				Name:   fmt.Sprintf("PDB: %s", pdb.Name),
				Status: "pass",
				Detail: fmt.Sprintf("PDB exists — MinAvailable/MaxUnavailable may affect future scale-down"),
			})
		}
	}

	// Check D: Scale-down risk
	if targetReplicas < currentReplicas {
		checks = append(checks, ScaleSimCheck{
			Name:   "Scale-Down Safety",
			Status: "pass",
			Detail: fmt.Sprintf("Scaling down from %d to %d — ensure PDB and graceful shutdown", currentReplicas, targetReplicas),
		})
	}

	// Check E: HPA conflict
	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(wlNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			if hpa.Spec.ScaleTargetRef.Name == wlName {
				maxReplicas := int(hpa.Spec.MaxReplicas)
				if targetReplicas > maxReplicas {
					checks = append(checks, ScaleSimCheck{
						Name:   "HPA Conflict",
						Status: "warn",
						Detail: fmt.Sprintf("HPA %s has maxReplicas=%d — HPA may scale down after manual scale-up", hpa.Name, maxReplicas),
					})
				} else {
					checks = append(checks, ScaleSimCheck{
						Name:   "HPA Alignment",
						Status: "pass",
						Detail: fmt.Sprintf("Target %d is within HPA range (min=%d, max=%d)", targetReplicas, int(*hpa.Spec.MinReplicas), maxReplicas),
					})
				}
				break
			}
		}
	}

	result.Checks = checks
	result.Blockers = blockers

	// 4. Verdict
	if len(blockers) > 0 {
		result.Verdict = "cannot-scale"
	} else {
		hasWarn := false
		for _, c := range checks {
			if c.Status == "warn" {
				hasWarn = true
			}
		}
		if hasWarn {
			result.Verdict = "risky"
		} else {
			result.Verdict = "can-scale"
		}
	}

	// 5. Suggestions
	result.Suggestions = generateScaleSimSuggestions(result)

	writeJSON(w, result)
}

// generateScaleSimSuggestions produces actionable suggestions.
func generateScaleSimSuggestions(result ScaleSimResult) []string {
	var suggestions []string

	switch result.Verdict {
	case "can-scale":
		suggestions = append(suggestions, fmt.Sprintf("Scaling %s to %d replicas is safe — all checks passed", result.Input.Workload, result.Input.TargetReplicas))
	case "risky":
		suggestions = append(suggestions, fmt.Sprintf("Scaling %s to %d replicas has warnings — review before proceeding", result.Input.Workload, result.Input.TargetReplicas))
	case "cannot-scale":
		suggestions = append(suggestions, fmt.Sprintf("Scaling %s to %d replicas is blocked by %d issue(s):", result.Input.Workload, result.Input.TargetReplicas, len(result.Blockers)))
		for _, b := range result.Blockers {
			suggestions = append(suggestions, fmt.Sprintf("  - [%s] %s", b.Type, b.Detail))
		}
	}

	// Resource impact
	if result.Delta.CPUDelta > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Additional resources needed: +%d mCPU, +%.0f MB memory", result.Delta.CPUDelta, result.Delta.MemDelta))
	} else if result.Delta.CPUDelta < 0 {
		suggestions = append(suggestions, fmt.Sprintf("Resources freed: %d mCPU, %.0f MB memory", -result.Delta.CPUDelta, -result.Delta.MemDelta))
	}

	return suggestions
}
