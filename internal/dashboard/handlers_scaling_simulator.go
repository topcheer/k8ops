package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScalingSimResult simulates cluster behavior under different load scenarios.
// It answers: "What would happen if traffic doubled? Do we have enough
// capacity? Which workloads would break first? How much would it cost?"
type ScalingSimResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ScalingSimSummary   `json:"summary"`
	Scenarios       []ScalingScenario   `json:"scenarios"`
	Bottlenecks     []SimBottleneck     `json:"bottlenecks"`
	CostProjection  []CostProjection    `json:"costProjection"`
	HealthScore     int                 `json:"healthScaleScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type ScalingSimSummary struct {
	CurrentPods      int     `json:"currentPods"`
	CurrentCPUReq    float64 `json:"currentCPUReqCores"`
	CurrentMemReq    float64 `json:"currentMemReqGB"`
	NodeCapacityCPU  float64 `json:"nodeCapacityCPU"`
	NodeCapacityMem  float64 `json:"nodeCapacityMemGB"`
	HeadroomCPU      float64 `json:"headroomCPUPct"`
	HeadroomMem      float64 `json:"headroomMemPct"`
	MaxScaleUpPods   int     `json:"maxScaleUpPods"`
}

type ScalingScenario struct {
	Name         string  `json:"name"`
	Multiplier   float64 `json:"multiplier"`
	NeededCPU    float64 `json:"neededCPU"`
	NeededMem    float64 `json:"neededMemGB"`
	NeededPods   int     `json:"neededPods"`
	Feasible     bool    `json:"feasible"`
	ExcessCPU    float64 `json:"excessCPU"`   // negative = shortage
	ExcessMem    float64 `json:"excessMem"`
	AdditionalNodes int  `json:"additionalNodesNeeded"`
	AdditionalCost float64 `json:"additionalMonthlyCost"`
}

type SimBottleneck struct {
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
	Scenario  string `json:"scenario"`
}

type CostProjection struct {
	Scenario     string  `json:"scenario"`
	MonthlyCost  float64 `json:"monthlyCost"`
	Delta        float64 `json:"deltaFromCurrent"`
}

// handleScalingSimulator handles GET /api/scalability/scaling-simulator
func (s *Server) handleScalingSimulator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ScalingSimResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Calculate current requests
	totalCPUReq := 0.0
	totalMemReq := 0.0
	appPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					totalCPUReq += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					totalMemReq += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}
		appPods++
	}

	// Node capacity
	nodeCPU := 0.0
	nodeMem := 0.0
	for _, node := range nodes.Items {
		if q, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			nodeCPU += float64(q.MilliValue()) / 1000
		}
		if q, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			nodeMem += float64(q.Value()) / (1024 * 1024 * 1024)
		}
	}

	result.Summary = ScalingSimSummary{
		CurrentPods: appPods, CurrentCPUReq: totalCPUReq, CurrentMemReq: totalMemReq,
		NodeCapacityCPU: nodeCPU, NodeCapacityMem: nodeMem,
	}
	if nodeCPU > 0 {
		result.Summary.HeadroomCPU = (nodeCPU - totalCPUReq) / nodeCPU * 100
	}
	if nodeMem > 0 {
		result.Summary.HeadroomMem = (nodeMem - totalMemReq) / nodeMem * 100
	}
	if totalCPUReq > 0 && nodeCPU > 0 {
		result.Summary.MaxScaleUpPods = int(nodeCPU/totalCPUReq*float64(appPods)) - appPods
	}

	// Per-node cost estimate
	perNodeCPUCost := 100.0 // $100/month per node (rough)
	if len(nodes.Items) > 0 {
		perNodeCPUCost = 100.0 // simplified
	}
	nodeCPUEach := 0.0
	if len(nodes.Items) > 0 {
		nodeCPUEach = nodeCPU / float64(len(nodes.Items))
	}

	// Run scenarios
	multipliers := []struct {
		name string
		mult float64
	}{
		{"current", 1.0}, {"1.5x", 1.5}, {"2x", 2.0}, {"3x", 3.0}, {"5x", 5.0},
	}
	currentCost := totalCPUReq*28.0 + totalMemReq*3.8

	for _, m := range multipliers {
		neededCPU := totalCPUReq * m.mult
		neededMem := totalMemReq * m.mult
		neededPods := int(float64(appPods) * m.mult)

		excessCPU := nodeCPU - neededCPU
		excessMem := nodeMem - neededMem
		feasible := excessCPU >= 0 && excessMem >= 0

		additionalNodes := 0
		if !feasible && nodeCPUEach > 0 {
			shortfall := 0.0
			if excessCPU < 0 {
				shortfall = -excessCPU
			}
			memShortfall := 0.0
			if excessMem < 0 {
				memShortfall = -excessMem
			}
			// Use the larger shortfall
			cpuNodesNeeded := int(shortfall/nodeCPUEach) + 1
			memPerNode := 0.0
			if len(nodes.Items) > 0 {
				memPerNode = nodeMem / float64(len(nodes.Items))
			}
			memNodesNeeded := 0
			if memPerNode > 0 {
				memNodesNeeded = int(memShortfall/memPerNode) + 1
			}
			additionalNodes = maxInt(cpuNodesNeeded, memNodesNeeded)
		}

		scenarioCost := neededCPU*28.0 + neededMem*3.8 + float64(additionalNodes)*perNodeCPUCost

		scenario := ScalingScenario{
			Name: m.name, Multiplier: m.mult,
			NeededCPU: neededCPU, NeededMem: neededMem, NeededPods: neededPods,
			Feasible: feasible, ExcessCPU: excessCPU, ExcessMem: excessMem,
			AdditionalNodes: additionalNodes, AdditionalCost: scenarioCost,
		}
		result.Scenarios = append(result.Scenarios, scenario)

		result.CostProjection = append(result.CostProjection, CostProjection{
			Scenario: m.name, MonthlyCost: scenarioCost, Delta: scenarioCost - currentCost,
		})

		// Identify bottlenecks
		if !feasible {
			if excessCPU < 0 {
				result.Bottlenecks = append(result.Bottlenecks, SimBottleneck{
					Resource: "CPU", Scenario: m.name, Severity: "high",
					Detail: fmt.Sprintf("CPU shortage: %.1f cores needed, %.1f available (short: %.1f)", neededCPU, nodeCPU, -excessCPU),
				})
			}
			if excessMem < 0 {
				result.Bottlenecks = append(result.Bottlenecks, SimBottleneck{
					Resource: "Memory", Scenario: m.name, Severity: "high",
					Detail: fmt.Sprintf("Memory shortage: %.1f GB needed, %.1f available (short: %.1f)", neededMem, nodeMem, -excessMem),
				})
			}
		}
	}

	result.HealthScore = computeScalingSimScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generateScalingSimRecs(result)

	writeJSON(w, result)
}

func computeScalingSimScore(s ScalingSimSummary) int {
	score := 100
	if s.HeadroomCPU < 20 {
		score -= 30
	} else if s.HeadroomCPU < 40 {
		score -= 15
	}
	if s.HeadroomMem < 20 {
		score -= 25
	} else if s.HeadroomMem < 40 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

func generateScalingSimRecs(r ScalingSimResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Scaling simulation: %.0f%% CPU headroom, %.0f%% memory headroom, max %d additional pods — score %d/100",
		r.Summary.HeadroomCPU, r.Summary.HeadroomMem, r.Summary.MaxScaleUpPods, r.HealthScore))
	for _, s := range r.Scenarios {
		if !s.Feasible {
			recs = append(recs, fmt.Sprintf("%s scenario: NOT feasible — need %d additional nodes (+$%.0f/month)", s.Name, s.AdditionalNodes, s.AdditionalCost))
		}
	}
	for _, b := range r.Bottlenecks {
		if b.Severity == "high" {
			recs = append(recs, fmt.Sprintf("BOTTLENECK [%s]: %s — %s", b.Scenario, b.Resource, b.Detail))
		}
	}
	return recs
}
