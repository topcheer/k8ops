package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostAllocationResult is the namespace cost allocation & chargeback report.
type CostAllocationResult struct {
	ScannedAt            time.Time           `json:"scannedAt"`
	Summary              CostAllocSummary    `json:"summary"`
	ByNamespace          []CostAllocNS       `json:"byNamespace"`
	ByWorkload           []CostAllocWorkload `json:"byWorkload"`
	IdleResources        []IdleResource      `json:"idleResources,omitempty"`
	SavingsOpportunities []SavingsOpp        `json:"savingsOpportunities"`
	Recommendations      []string            `json:"recommendations"`
	TotalMonthlyCost     float64             `json:"totalMonthlyCost"`
}

// CostAllocSummary aggregates cost statistics.
type CostAllocSummary struct {
	TotalNamespaces   int     `json:"totalNamespaces"`
	TotalWorkloads    int     `json:"totalWorkloads"`
	TotalNodes        int     `json:"totalNodes"`
	ClusterCPUMonthly float64 `json:"clusterCPUMonthlyCost"` // cost of all CPU
	ClusterMemMonthly float64 `json:"clusterMemMonthlyCost"` // cost of all memory
	AllocatedMonthly  float64 `json:"allocatedMonthlyCost"`  // cost of allocated resources
	IdleMonthly       float64 `json:"idleMonthlyCost"`       // cost of idle/unallocated
	IdlePercent       float64 `json:"idlePercent"`
	AvgCostPerPod     float64 `json:"avgCostPerPod"`
	AvgCostPerNS      float64 `json:"avgCostPerNamespace"`
	CPUHourlyRate     float64 `json:"cpuHourlyRate"` // $ per vCPU-hour
	MemHourlyRate     float64 `json:"memHourlyRate"` // $ per GB-hour
}

// CostAllocNS shows per-namespace cost breakdown.
type CostAllocNS struct {
	Namespace     string  `json:"namespace"`
	PodCount      int     `json:"podCount"`
	CPUAllocated  int64   `json:"cpuAllocatedMC"` // milli-cores
	MemAllocated  float64 `json:"memAllocatedGB"`
	MonthlyCost   float64 `json:"monthlyCost"`
	CPUCost       float64 `json:"cpuCost"`
	MemCost       float64 `json:"memCost"`
	PctOfTotal    float64 `json:"pctOfTotal"`
	WorkloadCount int     `json:"workloadCount"`
	CostPerPod    float64 `json:"costPerPod"`
	Efficiency    float64 `json:"efficiency"` // allocated vs requested (higher = better utilization)
}

// CostAllocWorkload shows per-workload cost.
type CostAllocWorkload struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	Replicas    int     `json:"replicas"`
	CPUCost     float64 `json:"cpuCost"`
	MemCost     float64 `json:"memCost"`
	MonthlyCost float64 `json:"monthlyCost"`
}

// IdleResource identifies wasted resources with cost impact.
type IdleResource struct {
	Type        string  `json:"type"` // idle-pod, over-provisioned, pending-pv, unused-service
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Description string  `json:"description"`
	WastedCPU   int64   `json:"wastedCPUmc"` // milli-cores
	WastedMem   float64 `json:"wastedMemGB"`
	MonthlyCost float64 `json:"monthlyCost"`
}

// SavingsOpp identifies a cost savings opportunity.
type SavingsOpp struct {
	Type           string  `json:"type"` // right-size, terminate, consolidate, spot
	Description    string  `json:"description"`
	Target         string  `json:"target,omitempty"`
	MonthlySavings float64 `json:"monthlySavings"`
	Severity       string  `json:"severity"`
}

// handleCostAllocation generates a namespace cost allocation & chargeback report.
// GET /api/scalability/cost-allocation
func (s *Server) handleCostAllocation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Pricing model (approximate cloud rates)
	cpuHourlyRate := 0.034 // $/vCPU-hour (approximate cloud rate)
	memHourlyRate := 0.004 // $/GB-hour
	hoursPerMonth := 730.0

	result := CostAllocationResult{
		ScannedAt: time.Now(),
	}
	result.Summary.CPUHourlyRate = cpuHourlyRate
	result.Summary.MemHourlyRate = memHourlyRate

	// 1. Collect nodes for total capacity
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalNodes = len(nodes.Items)
		totalCPU := int64(0)
		totalMemGB := 0.0
		for _, node := range nodes.Items {
			totalCPU += node.Status.Allocatable.Cpu().MilliValue()
			totalMemGB += float64(node.Status.Allocatable.Memory().Value()) / 1e9
		}
		result.Summary.ClusterCPUMonthly = float64(totalCPU) / 1000.0 * cpuHourlyRate * hoursPerMonth
		result.Summary.ClusterMemMonthly = totalMemGB * memHourlyRate * hoursPerMonth
	}

	// 2. Collect pods for allocation
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// Aggregate per namespace
	nsMap := map[string]*CostAllocNS{}
	wlMap := map[string]*CostAllocWorkload{}
	totalAllocCPU := int64(0)
	totalAllocMem := 0.0
	totalPodCount := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		totalPodCount++

		podCPU := int64(0)
		podMem := 0.0
		for _, c := range pod.Spec.Containers {
			podCPU += c.Resources.Requests.Cpu().MilliValue()
			podMem += float64(c.Resources.Requests.Memory().Value()) / 1e9
		}
		totalAllocCPU += podCPU
		totalAllocMem += podMem

		ns := pod.Namespace
		if nsMap[ns] == nil {
			nsMap[ns] = &CostAllocNS{Namespace: ns}
		}
		nsMap[ns].PodCount++
		nsMap[ns].CPUAllocated += podCPU
		nsMap[ns].MemAllocated += podMem

		// Workload attribution
		wlName, wlKind := extractWorkloadFromPod(pod)
		if wlName != "" {
			wlKey := fmt.Sprintf("%s/%s", ns, wlName)
			if wlMap[wlKey] == nil {
				wlMap[wlKey] = &CostAllocWorkload{
					Name: wlName, Namespace: ns, Kind: wlKind,
				}
			}
			wlMap[wlKey].Replicas++
		}
	}

	// 3. Calculate costs
	for _, nsStat := range nsMap {
		nsStat.CPUCost = float64(nsStat.CPUAllocated) / 1000.0 * cpuHourlyRate * hoursPerMonth
		nsStat.MemCost = nsStat.MemAllocated * memHourlyRate * hoursPerMonth
		nsStat.MonthlyCost = nsStat.CPUCost + nsStat.MemCost
		if nsStat.PodCount > 0 {
			nsStat.CostPerPod = nsStat.MonthlyCost / float64(nsStat.PodCount)
		}
	}

	// Workload costs
	for _, wl := range wlMap {
		nsStat := nsMap[wl.Namespace]
		if nsStat != nil && nsStat.PodCount > 0 {
			// Proportional allocation
			perPodCPU := nsStat.CPUCost / float64(nsStat.PodCount)
			perPodMem := nsStat.MemCost / float64(nsStat.PodCount)
			wl.CPUCost = perPodCPU * float64(wl.Replicas)
			wl.MemCost = perPodMem * float64(wl.Replicas)
			wl.MonthlyCost = wl.CPUCost + wl.MemCost
		}
		nsStat.WorkloadCount++
	}

	// Totals
	totalCost := 0.0
	for _, nsStat := range nsMap {
		totalCost += nsStat.MonthlyCost
	}
	result.TotalMonthlyCost = totalCost

	for _, nsStat := range nsMap {
		if totalCost > 0 {
			nsStat.PctOfTotal = nsStat.MonthlyCost / totalCost * 100
		}
		// Efficiency: utilization percentage
		if result.Summary.ClusterCPUMonthly > 0 {
			nsStat.Efficiency = nsStat.CPUCost / result.Summary.ClusterCPUMonthly * 100
		}
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MonthlyCost > result.ByNamespace[j].MonthlyCost
	})
	if len(result.ByNamespace) > 30 {
		result.ByNamespace = result.ByNamespace[:30]
	}

	// Workload list
	for _, wl := range wlMap {
		result.ByWorkload = append(result.ByWorkload, *wl)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].MonthlyCost > result.ByWorkload[j].MonthlyCost
	})
	if len(result.ByWorkload) > 50 {
		result.ByWorkload = result.ByWorkload[:50]
	}

	// 4. Idle resource detection
	var idleResources []IdleResource

	// Check for pending PVs (provisioned but unclaimed)
	pvs, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pv := range pvs.Items {
			if pv.Status.Phase == corev1.VolumeAvailable {
				sizeGB := float64(pv.Spec.Capacity.Storage().Value()) / 1e9
				monthlyCost := sizeGB * 0.10 * hoursPerMonth / 730 // ~$0.10/GB-month
				idleResources = append(idleResources, IdleResource{
					Type:        "unused-pv",
					Name:        pv.Name,
					Description: fmt.Sprintf("PV %s is Available (unclaimed), %.1f GB", pv.Name, sizeGB),
					WastedMem:   sizeGB,
					MonthlyCost: monthlyCost,
				})
			}
		}
	}

	// Check for services with no endpoints (unused)
	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err == nil {
		endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
		epMap := map[string]bool{}
		for _, ep := range endpoints.Items {
			if len(ep.Subsets) > 0 {
				epMap[fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)] = true
			}
		}
		for _, svc := range services.Items {
			if svc.Spec.Type == corev1.ServiceTypeExternalName || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				continue
			}
			if len(svc.Spec.Selector) > 0 {
				key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
				if !epMap[key] {
					idleResources = append(idleResources, IdleResource{
						Type:        "unused-service",
						Name:        svc.Name,
						Namespace:   svc.Namespace,
						Description: fmt.Sprintf("Service %s has selector but no endpoints", svc.Name),
						MonthlyCost: 2.0, // nominal cost
					})
				}
			}
		}
	}

	sort.Slice(idleResources, func(i, j int) bool {
		return idleResources[i].MonthlyCost > idleResources[j].MonthlyCost
	})
	if len(idleResources) > 50 {
		idleResources = idleResources[:50]
	}
	result.IdleResources = idleResources

	// 5. Savings opportunities
	result.SavingsOpportunities = generateSavingsOpps(result, cpuHourlyRate, memHourlyRate, hoursPerMonth)

	// 6. Summary
	result.Summary.TotalNamespaces = len(nsMap)
	result.Summary.TotalWorkloads = len(wlMap)
	result.Summary.AllocatedMonthly = totalCost
	result.Summary.IdleMonthly = result.Summary.ClusterCPUMonthly + result.Summary.ClusterMemMonthly - totalCost
	if result.Summary.IdleMonthly < 0 {
		result.Summary.IdleMonthly = 0
	}
	totalClusterCost := result.Summary.ClusterCPUMonthly + result.Summary.ClusterMemMonthly
	if totalClusterCost > 0 {
		result.Summary.IdlePercent = result.Summary.IdleMonthly / totalClusterCost * 100
	}
	if totalPodCount > 0 {
		result.Summary.AvgCostPerPod = totalCost / float64(totalPodCount)
	}
	if len(nsMap) > 0 {
		result.Summary.AvgCostPerNS = totalCost / float64(len(nsMap))
	}

	// 7. Recommendations
	result.Recommendations = generateCostAllocRecs(result)

	writeJSON(w, result)
}

// generateSavingsOpps identifies cost savings opportunities.
func generateSavingsOpps(result CostAllocationResult, cpuRate, memRate, hours float64) []SavingsOpp {
	var opps []SavingsOpp

	// Idle cluster resources
	if result.Summary.IdlePercent > 30 {
		opps = append(opps, SavingsOpp{
			Type:           "consolidate",
			Description:    fmt.Sprintf("%.0f%% of cluster resources are unallocated — consolidate workloads to fewer nodes", result.Summary.IdlePercent),
			MonthlySavings: result.Summary.IdleMonthly * 0.5,
			Severity:       "high",
		})
	}

	// Top cost namespaces
	for i, ns := range result.ByNamespace {
		if i >= 3 {
			break
		}
		if ns.MonthlyCost > 50 {
			opps = append(opps, SavingsOpp{
				Type:           "right-size",
				Target:         ns.Namespace,
				Description:    fmt.Sprintf("Namespace %q costs $%.0f/month — review resource requests for 20%% potential savings", ns.Namespace, ns.MonthlyCost),
				MonthlySavings: ns.MonthlyCost * 0.2,
				Severity:       "medium",
			})
		}
	}

	// Idle resources
	totalIdleCost := 0.0
	for _, ir := range result.IdleResources {
		totalIdleCost += ir.MonthlyCost
	}
	if totalIdleCost > 10 {
		opps = append(opps, SavingsOpp{
			Type:           "terminate",
			Description:    fmt.Sprintf("$%.0f/month from %d idle resources (unused PVs, services with no endpoints)", totalIdleCost, len(result.IdleResources)),
			MonthlySavings: totalIdleCost,
			Severity:       "medium",
		})
	}

	sort.Slice(opps, func(i, j int) bool {
		return opps[i].MonthlySavings > opps[j].MonthlySavings
	})

	return opps
}

// generateCostAllocRecs produces recommendations.
func generateCostAllocRecs(result CostAllocationResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Total cluster cost: $%.0f/month (%.0f%% allocated, $%.0f idle)",
		result.TotalMonthlyCost, 100-result.Summary.IdlePercent, result.Summary.IdleMonthly))

	if result.Summary.IdlePercent > 30 {
		recs = append(recs, fmt.Sprintf("%.0f%% resources are idle ($%.0f/month) — right-size workloads or scale down nodes", result.Summary.IdlePercent, result.Summary.IdleMonthly))
	}

	if len(result.ByNamespace) > 0 {
		top := result.ByNamespace[0]
		recs = append(recs, fmt.Sprintf("Top namespace %q costs $%.0f/month (%.1f%% of total) — focus optimization here", top.Namespace, top.MonthlyCost, top.PctOfTotal))
	}

	if len(result.IdleResources) > 0 {
		recs = append(recs, fmt.Sprintf("%d idle resource(s) costing $%.0f/month — clean up unused PVs and services", len(result.IdleResources), result.Summary.IdleMonthly))
	}

	if result.Summary.AvgCostPerPod > 0 {
		recs = append(recs, fmt.Sprintf("Average cost per pod: $%.2f/month — identify expensive pods for optimization", result.Summary.AvgCostPerPod))
	}

	return recs
}
