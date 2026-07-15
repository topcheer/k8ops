package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CarbonFootprintResult is the cluster carbon footprint and sustainability analysis.
type CarbonFootprintResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CarbonSummary       `json:"summary"`
	EnergyBreakdown EnergyBreakdown     `json:"energyBreakdown"`
	ByNamespace     []CarbonNSStat      `json:"byNamespace"`
	ByWorkload      []CarbonWorkload    `json:"byWorkload"`
	Opportunities   []CarbonOpportunity `json:"opportunities"`
	GreenScore      int                 `json:"greenScore"`
	Recommendations []string            `json:"recommendations"`
}

// CarbonSummary aggregates cluster-wide carbon metrics.
type CarbonSummary struct {
	TotalPowerKW     float64 `json:"totalPowerKW"`     // estimated power consumption
	IdlePowerKW      float64 `json:"idlePowerKW"`      // idle/baseline power
	ComputePowerKW   float64 `json:"computePowerKW"`   // actual workload power
	DailyEnergyKWh   float64 `json:"dailyEnergyKWh"`   // estimated daily energy
	MonthlyEnergyKWh float64 `json:"monthlyEnergyKWh"` // estimated monthly energy
	DailyCO2Kg       float64 `json:"dailyCO2Kg"`       // daily CO2 emissions
	MonthlyCO2Kg     float64 `json:"monthlyCO2Kg"`     // monthly CO2 emissions
	CarbonIntensity  float64 `json:"carbonIntensity"`  // gCO2/kWh for the region
	Region           string  `json:"region"`           // detected cloud region
	TotalNodes       int     `json:"totalNodes"`
	TotalWorkloads   int     `json:"totalWorkloads"`
	WastedPowerKW    float64 `json:"wastedPowerKW"`    // power from idle/over-provisioned resources
	WastedCO2KgMonth float64 `json:"wastedCO2KgMonth"` // CO2 from wasted resources
	PUE              float64 `json:"pue"`              // power usage effectiveness estimate
}

// EnergyBreakdown shows power consumption by component type.
type EnergyBreakdown struct {
	CPU        ComponentEnergy `json:"cpu"`
	Memory     ComponentEnergy `json:"memory"`
	Storage    ComponentEnergy `json:"storage"`
	Networking ComponentEnergy `json:"networking"`
	Overhead   ComponentEnergy `json:"overhead"` // datacenter PUE overhead
}

// ComponentEnergy represents energy consumption for one component.
type ComponentEnergy struct {
	PowerKW  float64 `json:"powerKW"`
	PctTotal float64 `json:"pctTotal"`
}

// CarbonNSStat is per-namespace carbon attribution.
type CarbonNSStat struct {
	Namespace       string  `json:"namespace"`
	PowerKW         float64 `json:"powerKW"`
	DailyEnergyKWh  float64 `json:"dailyEnergyKWh"`
	DailyCO2Kg      float64 `json:"dailyCO2Kg"`
	MonthlyCO2Kg    float64 `json:"monthlyCO2Kg"`
	PctClusterTotal float64 `json:"pctClusterTotal"`
	PodCount        int     `json:"podCount"`
}

// CarbonWorkload is a single workload's carbon attribution.
type CarbonWorkload struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Kind         string  `json:"kind"`
	PowerKW      float64 `json:"powerKW"`
	DailyCO2Kg   float64 `json:"dailyCO2Kg"`
	MonthlyCO2Kg float64 `json:"monthlyCO2Kg"`
	Efficiency   float64 `json:"efficiency"`   // CO2 per replica
	CPURequestMC int64   `json:"cpuRequestMC"` // milli-cores requested
	MemRequestMB float64 `json:"memRequestMB"` // MB requested
	Replicas     int     `json:"replicas"`
}

// CarbonOpportunity identifies a carbon reduction opportunity.
type CarbonOpportunity struct {
	Type                 string  `json:"type"` // rightsize, schedule, consolidate, relocate
	Workload             string  `json:"workload,omitempty"`
	Namespace            string  `json:"namespace,omitempty"`
	Description          string  `json:"description"`
	PotentialSavingCO2Kg float64 `json:"potentialSavingCO2KgMonth"`
	PotentialSavingKWh   float64 `json:"potentialSavingKWhMonth"`
	Severity             string  `json:"severity"` // high, medium, low
}

// handleCarbonFootprint estimates cluster carbon footprint and identifies
// sustainability improvement opportunities.
// GET /api/scalability/carbon-footprint
func (s *Server) handleCarbonFootprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CarbonFootprintResult{ScannedAt: time.Now()}

	// 1. Detect region and carbon intensity from node labels
	region, carbonIntensity := detectRegionAndCarbon(rc, ctx)
	result.Summary.Region = region
	result.Summary.CarbonIntensity = carbonIntensity
	result.Summary.PUE = 1.6 // typical datacenter PUE; cloud is ~1.1, on-prem ~1.5-2.0

	// 2. Collect nodes and estimate power
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalNodes = len(nodes.Items)

		totalAllocatableCPU := int64(0)
		totalAllocatableMem := int64(0)
		totalCapacityCPU := int64(0)

		for _, node := range nodes.Items {
			totalAllocatableCPU += node.Status.Allocatable.Cpu().MilliValue()
			totalAllocatableMem += node.Status.Allocatable.Memory().Value()
			totalCapacityCPU += node.Status.Capacity.Cpu().MilliValue()
		}

		// Estimate idle power from node count (each server idles at ~100-200W)
		// Based on node type (VM size inferred from capacity)
		idlePowerPerNode := estimateIdlePowerPerNode(nodes.Items)
		result.Summary.IdlePowerKW = idlePowerPerNode * float64(len(nodes.Items)) / 1000.0
	}

	// 3. Collect pods and estimate workload power consumption
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 5000})
	if err == nil {
		// Aggregate CPU/memory requests per namespace and workload
		nsPowerMap := map[string]*CarbonNSStat{}
		wlPowerMap := map[string]*CarbonWorkload{}
		var allWorkloads []CarbonWorkload

		totalRequestedCPU := int64(0) // milli-cores
		totalRequestedMem := int64(0) // bytes
		totalPodCount := 0

		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			totalPodCount++

			podCPU := int64(0)
			podMem := int64(0)

			for _, c := range pod.Spec.Containers {
				podCPU += c.Resources.Requests.Cpu().MilliValue()
				podMem += c.Resources.Requests.Memory().Value()
			}

			totalRequestedCPU += podCPU
			totalRequestedMem += podMem

			// Estimate power for this pod's resource requests
			// CPU is the dominant factor: ~0.5W per milli-core at full utilization
			// Memory: ~0.025W per MB
			podCPUPower := float64(podCPU) * 0.0005         // kW
			podMemPower := float64(podMem) / 1e6 * 0.000025 // MB → kW
			podPower := (podCPUPower + podMemPower) * result.Summary.PUE

			// Namespace attribution
			ns := pod.Namespace
			if nsPowerMap[ns] == nil {
				nsPowerMap[ns] = &CarbonNSStat{Namespace: ns}
			}
			nsPowerMap[ns].PowerKW += podPower
			nsPowerMap[ns].PodCount++

			// Workload attribution (extract from pod owner)
			wlName, wlKind := extractWorkloadFromPod(pod)
			if wlName != "" {
				key := fmt.Sprintf("%s/%s", ns, wlName)
				if wlPowerMap[key] == nil {
					wlPowerMap[key] = &CarbonWorkload{
						Name:      wlName,
						Namespace: ns,
						Kind:      wlKind,
					}
				}
				wlPowerMap[key].PowerKW += podPower
				wlPowerMap[key].CPURequestMC += podCPU
				wlPowerMap[key].MemRequestMB += float64(podMem) / 1e6
				wlPowerMap[key].Replicas++
			}
		}

		result.Summary.TotalWorkloads = len(wlPowerMap)

		// Calculate daily/monthly energy and CO2
		result.Summary.ComputePowerKW = result.Summary.IdlePowerKW * 0.1 // placeholder, refined below
		totalPowerKW := result.Summary.IdlePowerKW

		// Add compute power (actual resource usage above idle)
		// Assume average utilization of 40% of requested resources
		computePower := float64(totalRequestedCPU) * 0.0005 * 0.4 * result.Summary.PUE
		result.Summary.ComputePowerKW = computePower
		totalPowerKW = result.Summary.IdlePowerKW + computePower
		result.Summary.TotalPowerKW = totalPowerKW

		result.Summary.DailyEnergyKWh = totalPowerKW * 24
		result.Summary.MonthlyEnergyKWh = totalPowerKW * 24 * 30
		result.Summary.DailyCO2Kg = result.Summary.DailyEnergyKWh * carbonIntensity / 1000.0
		result.Summary.MonthlyCO2Kg = result.Summary.MonthlyEnergyKWh * carbonIntensity / 1000.0

		// Estimate wasted power (idle capacity = allocatable - requested)
		// Node idle power covers fixed overhead; wasted = (unrequested compute fraction) of node power
		if result.Summary.IdlePowerKW > 0 {
			wasteFraction := 1.0
			if totalRequestedCPU > 0 {
				// Estimate total allocatable CPU from node count
				allocCPU := getTotalAllocatableCPU(nodes.Items)
				if allocCPU > 0 {
					utilization := float64(totalRequestedCPU) / float64(allocCPU)
					wasteFraction = 1.0 - utilization
					if wasteFraction < 0 {
						wasteFraction = 0
					}
				}
			}
			result.Summary.WastedPowerKW = result.Summary.IdlePowerKW * wasteFraction * 0.5 // partial waste
			result.Summary.WastedCO2KgMonth = result.Summary.WastedPowerKW * 24 * 30 * carbonIntensity / 1000.0
		}

		// Build namespace stats
		for _, stat := range nsPowerMap {
			stat.DailyEnergyKWh = stat.PowerKW * 24
			stat.DailyCO2Kg = stat.DailyEnergyKWh * carbonIntensity / 1000.0
			stat.MonthlyCO2Kg = stat.DailyCO2Kg * 30
			if totalPowerKW > 0 {
				stat.PctClusterTotal = stat.PowerKW / totalPowerKW * 100
			}
			result.ByNamespace = append(result.ByNamespace, *stat)
		}
		sort.Slice(result.ByNamespace, func(i, j int) bool {
			return result.ByNamespace[i].MonthlyCO2Kg > result.ByNamespace[j].MonthlyCO2Kg
		})
		if len(result.ByNamespace) > 30 {
			result.ByNamespace = result.ByNamespace[:30]
		}

		// Build workload stats
		for _, wl := range wlPowerMap {
			wl.DailyCO2Kg = wl.PowerKW * 24 * carbonIntensity / 1000.0
			wl.MonthlyCO2Kg = wl.DailyCO2Kg * 30
			if wl.Replicas > 0 {
				wl.Efficiency = wl.MonthlyCO2Kg / float64(wl.Replicas)
			}
			allWorkloads = append(allWorkloads, *wl)
		}
		sort.Slice(allWorkloads, func(i, j int) bool {
			return allWorkloads[i].MonthlyCO2Kg > allWorkloads[j].MonthlyCO2Kg
		})
		if len(allWorkloads) > 50 {
			allWorkloads = allWorkloads[:50]
		}
		result.ByWorkload = allWorkloads
	}

	// 4. Energy breakdown by component
	totalPower := result.Summary.TotalPowerKW
	if totalPower > 0 {
		// Typical distribution: CPU ~50%, Memory ~20%, Storage ~5%, Network ~5%, Overhead (PUE) ~20%
		pueOverhead := 1.0 - 1.0/result.Summary.PUE
		itPower := totalPower * (1.0 - pueOverhead)
		result.EnergyBreakdown = EnergyBreakdown{
			CPU:        ComponentEnergy{PowerKW: itPower * 0.50, PctTotal: (1 - pueOverhead) * 50},
			Memory:     ComponentEnergy{PowerKW: itPower * 0.20, PctTotal: (1 - pueOverhead) * 20},
			Storage:    ComponentEnergy{PowerKW: itPower * 0.05, PctTotal: (1 - pueOverhead) * 5},
			Networking: ComponentEnergy{PowerKW: itPower * 0.05, PctTotal: (1 - pueOverhead) * 5},
			Overhead:   ComponentEnergy{PowerKW: totalPower * pueOverhead, PctTotal: pueOverhead * 100},
		}
	}

	// 5. Generate carbon reduction opportunities
	result.Opportunities = generateCarbonOpportunities(result)

	// 6. Calculate green score (0-100)
	score := 100
	// Deduct for wasted resources
	if result.Summary.WastedCO2KgMonth > 100 {
		score -= 15
	} else if result.Summary.WastedCO2KgMonth > 50 {
		score -= 8
	}
	// Deduct for high carbon intensity region
	if carbonIntensity > 500 {
		score -= 10
	} else if carbonIntensity > 300 {
		score -= 5
	}
	// Deduct for high CO2 per workload (inefficiency)
	if len(result.ByWorkload) > 0 {
		topWl := result.ByWorkload[0]
		if topWl.MonthlyCO2Kg > 100 {
			score -= 10
		}
	}
	// Deduct for too many namespaces with tiny footprints (fragmentation)
	if len(result.ByNamespace) > 20 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.GreenScore = score

	// 7. Recommendations
	result.Recommendations = generateCarbonRecommendations(result)

	writeJSON(w, result)
}

// detectRegionAndCarbon determines cloud region and grid carbon intensity from node metadata.
func detectRegionAndCarbon(rc *requestClients, ctx context.Context) (string, float64) {
	// Default carbon intensity values (gCO2/kWh) by region
	// Source: approximate cloud provider sustainability data
	regionCarbonMap := map[string]float64{
		// AWS
		"us-east-1":      340, // Virginia (mixed)
		"us-east-2":      340, // Ohio
		"us-west-1":      120, // California (low carbon)
		"us-west-2":      120, // Oregon (hydro)
		"eu-west-1":      320, // Ireland
		"eu-central-1":   340, // Frankfurt
		"eu-north-1":     40,  // Sweden (hydro/nuclear)
		"ap-south-1":     700, // Mumbai (coal-heavy)
		"ap-northeast-1": 500, // Tokyo
		"ap-southeast-1": 490, // Singapore
		// GCP
		"us-central1":   470,
		"us-east1":      340,
		"us-west1":      80, // Oregon (low carbon)
		"europe-west1":  340,
		"europe-north1": 40, // Finland (low carbon)
		"asia-south1":   700,
		// Azure
		"eastus":      340,
		"westus2":     120,
		"northeurope": 340,
		"westeurope":  340,
	}

	// Default fallback
	region := "unknown"
	carbonIntensity := 400.0 // world average

	// Try to detect from provider ID on nodes
	clientset := rc.clientset
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			// Check providerID for cloud hints
			providerID := node.Spec.ProviderID
			if providerID != "" {
				providerLower := strings.ToLower(providerID)
				// AWS: aws:///us-east-1a/i-xxx
				if strings.Contains(providerLower, "aws") {
					region = extractRegionFromProviderID(providerID, "aws://")
					if ci, ok := regionCarbonMap[region]; ok {
						return fmt.Sprintf("AWS %s", region), ci
					}
				}
				// GCP: gce://project/zone/instance
				if strings.Contains(providerLower, "gce") {
					region = extractRegionFromProviderID(providerID, "gce://")
					if ci, ok := regionCarbonMap[region]; ok {
						return fmt.Sprintf("GCP %s", region), ci
					}
				}
				// Azure: azure:///subscriptions/.../resourceGroups/.../providers/Microsoft.Compute/virtualMachines/xxx
				if strings.Contains(providerLower, "azure") {
					// Check node labels for region
					if r, ok := node.Labels["topology.kubernetes.io/region"]; ok {
						if ci, ok := regionCarbonMap[r]; ok {
							return fmt.Sprintf("Azure %s", r), ci
						}
					}
				}
			}

			// Check node labels for region hints
			if r, ok := node.Labels["topology.kubernetes.io/region"]; ok {
				if ci, ok := regionCarbonMap[r]; ok {
					provider := "Cloud"
					if strings.Contains(strings.ToLower(providerID), "aws") {
						provider = "AWS"
					} else if strings.Contains(strings.ToLower(providerID), "gce") {
						provider = "GCP"
					} else if strings.Contains(strings.ToLower(providerID), "azure") {
						provider = "Azure"
					}
					return fmt.Sprintf("%s %s", provider, r), ci
				}
				region = r
			}

			// Check for well-known labels
			if k := node.Labels["kubernetes.io/hostname"]; k != "" && region == "unknown" {
				// Try to parse region from hostname
				for knownRegion := range regionCarbonMap {
					if strings.Contains(k, knownRegion) {
						return fmt.Sprintf("Unknown %s", knownRegion), regionCarbonMap[knownRegion]
					}
				}
			}
		}
	}

	return region, carbonIntensity
}

// extractRegionFromProviderID parses the region from a cloud provider ID.
func extractRegionFromProviderID(providerID, prefix string) string {
	rest := strings.TrimPrefix(providerID, prefix)
	rest = strings.TrimPrefix(rest, "//")
	// Format: zone/instance-id or region/zone/instance-id
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		// Check if this part looks like a region (e.g., us-east-1, europe-west1)
		if isRegionLike(part) {
			// Strip zone suffix (e.g., us-east-1a → us-east-1)
			region := part
			if len(region) > 1 {
				lastChar := region[len(region)-1]
				if lastChar >= 'a' && lastChar <= 'z' && len(region) > 2 {
					// Check if without last char it's still valid
					stripped := region[:len(region)-1]
					if isRegionLike(stripped) {
						region = stripped
					}
				}
			}
			return region
		}
	}
	return ""
}

// isRegionLike checks if a string looks like a cloud region name.
func isRegionLike(s string) bool {
	if len(s) < 8 { // regions are at least 8 chars (us-west1, eu-west1)
		return false
	}
	hasDash := strings.Contains(s, "-")
	hasDigit := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}
	return hasDash && hasDigit
}

// estimateIdlePowerPerNode estimates idle power per node in watts.
func estimateIdlePowerPerNode(nodes []corev1.Node) float64 {
	var totalIdleWatts float64
	for _, node := range nodes {
		// Estimate based on node capacity
		cpuMillis := node.Status.Capacity.Cpu().MilliValue()
		memGB := node.Status.Capacity.Memory().Value() / 1e9

		// Base power depends on node size (server class)
		var idleWatts float64
		switch {
		case cpuMillis >= 16000: // 16+ cores = large server
			idleWatts = 250 + float64(memGB)*2
		case cpuMillis >= 8000: // 8+ cores = medium server
			idleWatts = 180 + float64(memGB)*1.5
		case cpuMillis >= 4000: // 4+ cores
			idleWatts = 120 + float64(memGB)*1.0
		default: // small VM
			idleWatts = 80 + float64(memGB)*0.5
		}

		// Check for GPU nodes
		if gpuCount := node.Status.Capacity["nvidia.com/gpu"]; gpuCount.Value() > 0 {
			idleWatts += float64(gpuCount.Value()) * 50 // GPU idle ~50W each
		}

		totalIdleWatts += idleWatts
	}

	if len(nodes) == 0 {
		return 0
	}
	return totalIdleWatts / float64(len(nodes))
}

// getTotalAllocatableCPU returns total allocatable CPU across all nodes in milli-cores.
func getTotalAllocatableCPU(nodes []corev1.Node) int64 {
	var total int64
	for _, node := range nodes {
		total += node.Status.Allocatable.Cpu().MilliValue()
	}
	return total
}

// extractWorkloadFromPod tries to determine the owning workload from pod metadata.
func extractWorkloadFromPod(pod *corev1.Pod) (string, string) {
	for _, ref := range pod.OwnerReferences {
		switch ref.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
			return ref.Name, ref.Kind
		}
	}
	// Fallback: use pod name without hash
	return extractWorkloadName(pod.Name), "Pod"
}

// generateCarbonOpportunities identifies carbon reduction opportunities.
func generateCarbonOpportunities(result CarbonFootprintResult) []CarbonOpportunity {
	var opportunities []CarbonOpportunity

	// 1. Wasted resources (over-provisioned)
	if result.Summary.WastedCO2KgMonth > 50 {
		opportunities = append(opportunities, CarbonOpportunity{
			Type:                 "consolidate",
			Description:          fmt.Sprintf("%.0f kg CO2/month from idle/over-provisioned resources — consolidate workloads to fewer nodes and scale down", result.Summary.WastedCO2KgMonth),
			PotentialSavingCO2Kg: result.Summary.WastedCO2KgMonth,
			PotentialSavingKWh:   result.Summary.WastedPowerKW * 24 * 30,
			Severity:             "high",
		})
	}

	// 2. Top carbon consumers (rightsize candidates)
	for i, wl := range result.ByWorkload {
		if i >= 5 {
			break
		}
		if wl.MonthlyCO2Kg > 20 && wl.Replicas > 1 {
			// Potential 20-30% reduction from rightsizing
			saving := wl.MonthlyCO2Kg * 0.25
			opportunities = append(opportunities, CarbonOpportunity{
				Type:                 "rightsize",
				Workload:             wl.Name,
				Namespace:            wl.Namespace,
				Description:          fmt.Sprintf("Right-size %s: %d replicas consuming %.1f kg CO2/month — review CPU/memory requests for 25%% potential reduction", wl.Name, wl.Replicas, wl.MonthlyCO2Kg),
				PotentialSavingCO2Kg: saving,
				PotentialSavingKWh:   wl.PowerKW * 24 * 30 * 0.25,
				Severity:             "medium",
			})
		}
	}

	// 3. Carbon-intensive region
	if result.Summary.CarbonIntensity > 500 {
		opportunities = append(opportunities, CarbonOpportunity{
			Type:                 "relocate",
			Description:          fmt.Sprintf("Region %s has high carbon intensity (%.0f gCO2/kWh) — consider scheduling batch workloads during low-carbon hours or relocating to cleaner regions", result.Summary.Region, result.Summary.CarbonIntensity),
			PotentialSavingCO2Kg: result.Summary.MonthlyCO2Kg * 0.15,
			PotentialSavingKWh:   0,
			Severity:             "medium",
		})
	}

	// 4. Schedule workloads for green hours
	if result.Summary.CarbonIntensity > 200 {
		opportunity := CarbonOpportunity{
			Type:                 "schedule",
			Description:          "Schedule non-time-critical batch workloads during off-peak hours when grid carbon intensity is typically lower",
			PotentialSavingCO2Kg: result.Summary.MonthlyCO2Kg * 0.05,
			PotentialSavingKWh:   0,
			Severity:             "low",
		}
		opportunities = append(opportunities, opportunity)
	}

	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].PotentialSavingCO2Kg > opportunities[j].PotentialSavingCO2Kg
	})

	return opportunities
}

// generateCarbonRecommendations produces actionable recommendations.
func generateCarbonRecommendations(result CarbonFootprintResult) []string {
	var recs []string

	// Top-level summary
	recs = append(recs, fmt.Sprintf("Cluster consumes %.1f kW (%.0f kWh/month), emitting %.0f kg CO2/month in region %s",
		result.Summary.TotalPowerKW, result.Summary.MonthlyEnergyKWh, result.Summary.MonthlyCO2Kg, result.Summary.Region))

	// Wasted resources
	if result.Summary.WastedCO2KgMonth > 50 {
		recs = append(recs, fmt.Sprintf("%.0f kg CO2/month from wasted resources — consolidate to fewer nodes for immediate carbon savings", result.Summary.WastedCO2KgMonth))
	}

	// Top consumer
	if len(result.ByWorkload) > 0 {
		top := result.ByWorkload[0]
		recs = append(recs, fmt.Sprintf("Workload %q in %s is the top carbon consumer (%.1f kg CO2/month) — review resource requests", top.Name, top.Namespace, top.MonthlyCO2Kg))
	}

	// Top namespace
	if len(result.ByNamespace) > 0 {
		topNS := result.ByNamespace[0]
		recs = append(recs, fmt.Sprintf("Namespace %q accounts for %.0f%% of cluster carbon footprint — focus optimization efforts here", topNS.Namespace, topNS.PctClusterTotal))
	}

	// Region awareness
	if result.Summary.CarbonIntensity > 500 {
		recs = append(recs, fmt.Sprintf("Carbon intensity in %s is %.0f gCO2/kWh (high) — consider cleaner regions for new workloads", result.Summary.Region, result.Summary.CarbonIntensity))
	} else if result.Summary.CarbonIntensity < 100 {
		recs = append(recs, fmt.Sprintf("Carbon intensity in %s is %.0f gCO2/kWh (excellent) — region is already very green", result.Summary.Region, result.Summary.CarbonIntensity))
	}

	// Right-sizing
	rightSizeCount := 0
	for _, wl := range result.ByWorkload {
		if wl.MonthlyCO2Kg > 10 && wl.Replicas > 1 {
			rightSizeCount++
		}
	}
	if rightSizeCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are candidates for right-sizing — reducing resource requests can cut carbon by 20-30%%", rightSizeCount))
	}

	// Green score context
	if result.GreenScore < 50 {
		recs = append(recs, fmt.Sprintf("Green score is %d/100 — significant carbon optimization opportunities available", result.GreenScore))
	} else if result.GreenScore >= 80 {
		recs = append(recs, fmt.Sprintf("Green score is %d/100 — cluster is operating efficiently", result.GreenScore))
	}

	return recs
}
