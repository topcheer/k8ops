package cost

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Resources represents CPU and RAM quantities.
type Resources struct {
	CPUCores float64 `json:"cpuCores"`
	RAMGB    float64 `json:"ramGB"`
}

// NamespaceCost holds the aggregated cost for a single namespace.
type NamespaceCost struct {
	Namespace    string   `json:"namespace"`
	Pods         int      `json:"pods"`
	CPURequested float64  `json:"cpuRequestedCores"`
	RAMRequested float64  `json:"ramRequestedGB"`
	MonthlyCost  float64  `json:"monthlyCostUSD"`
	Percentage   float64  `json:"percentage"`
	TopWorkloads []string `json:"topWorkloads,omitempty"`
}

// CostSummary is the response for /api/cost/summary.
type CostSummary struct {
	TotalMonthlyCost float64         `json:"totalMonthlyCostUSD"`
	TotalPods        int             `json:"totalPods"`
	TotalCPU         float64         `json:"totalCPURequestedCores"`
	TotalRAM         float64         `json:"totalRAMRequestedGB"`
	Namespaces       []NamespaceCost `json:"namespaces"`
	Pricing          Pricing         `json:"pricing"`
	GeneratedAt      time.Time       `json:"generatedAt"`
}

// Recommendation is a single right-sizing suggestion.
type Recommendation struct {
	Workload       string    `json:"workload"` // "namespace/name"
	Kind           string    `json:"kind"`
	Namespace      string    `json:"namespace"`
	ContainerName  string    `json:"container"`
	Current        Resources `json:"current"`
	Limit          Resources `json:"limit"`
	Recommended    Resources `json:"recommended"`
	MonthlySavings float64   `json:"monthlySavingsUSD"`
	SavingsPct     float64   `json:"savingsPercent"`
	Reason         string    `json:"reason"`
}

// RecommendationSummary is the response for /api/cost/recommendations.
type RecommendationSummary struct {
	TotalPotentialSavings float64          `json:"totalPotentialSavingsUSD"`
	Count                 int              `json:"count"`
	Recommendations       []Recommendation `json:"recommendations"`
	Pricing               Pricing          `json:"pricing"`
	GeneratedAt           time.Time        `json:"generatedAt"`
}

// Estimator calculates cluster resource costs and right-sizing recommendations.
type Estimator struct {
	clientset kubernetes.Interface
	pricing   Pricing
}

// NewEstimator creates a new cost estimator with the given pricing.
func NewEstimator(clientset kubernetes.Interface, pricing Pricing) *Estimator {
	return &Estimator{
		clientset: clientset,
		pricing:   pricing,
	}
}

// Summary calculates the cluster-wide cost breakdown by namespace.
func (e *Estimator) Summary(ctx context.Context) (*CostSummary, error) {
	pods, err := e.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	type nsAgg struct {
		pods      int
		cpuCores  float64
		ramGB     float64
		workloads map[string]struct{}
	}
	byNS := make(map[string]*nsAgg)

	for _, pod := range pods.Items {
		ns := pod.Namespace
		if ns == "" {
			continue
		}
		agg, ok := byNS[ns]
		if !ok {
			agg = &nsAgg{workloads: make(map[string]struct{})}
			byNS[ns] = agg
		}
		agg.pods++

		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" || ownerRef.Kind == "Deployment" ||
				ownerRef.Kind == "StatefulSet" || ownerRef.Kind == "DaemonSet" ||
				ownerRef.Kind == "Job" {
				agg.workloads[ownerRef.Name] = struct{}{}
			}
		}

		for _, c := range pod.Spec.Containers {
			req := c.Resources.Requests
			agg.cpuCores += resourceToFloat(req, corev1.ResourceCPU)
			agg.ramGB += resourceToFloat(req, corev1.ResourceMemory) / 1e9
		}
	}

	var totalCost float64
	nsCosts := make([]NamespaceCost, 0, len(byNS))
	for ns, agg := range byNS {
		c := agg.cpuCores*e.pricing.CPUPricePerCore + agg.ramGB*e.pricing.RAMPricePerGB
		totalCost += c

		top := make([]string, 0, len(agg.workloads))
		for w := range agg.workloads {
			top = append(top, w)
		}
		sort.Strings(top)
		if len(top) > 5 {
			top = top[:5]
		}

		nsCosts = append(nsCosts, NamespaceCost{
			Namespace:    ns,
			Pods:         agg.pods,
			CPURequested: roundTo(agg.cpuCores, 3),
			RAMRequested: roundTo(agg.ramGB, 3),
			MonthlyCost:  roundTo(c, 2),
			TopWorkloads: top,
		})
	}

	sort.Slice(nsCosts, func(i, j int) bool {
		return nsCosts[i].MonthlyCost > nsCosts[j].MonthlyCost
	})

	totalPods := 0
	totalCPU := 0.0
	totalRAM := 0.0
	for i := range nsCosts {
		if totalCost > 0 {
			nsCosts[i].Percentage = roundTo(nsCosts[i].MonthlyCost/totalCost*100, 1)
		}
		totalPods += nsCosts[i].Pods
		totalCPU += nsCosts[i].CPURequested
		totalRAM += nsCosts[i].RAMRequested
	}

	return &CostSummary{
		TotalMonthlyCost: roundTo(totalCost, 2),
		TotalPods:        totalPods,
		TotalCPU:         roundTo(totalCPU, 3),
		TotalRAM:         roundTo(totalRAM, 3),
		Namespaces:       nsCosts,
		Pricing:          e.pricing,
		GeneratedAt:      time.Now().UTC(),
	}, nil
}

// Recommendations generates right-sizing suggestions by comparing resource
// requests to limits. When limit/request ratio exceeds 3x,
// the pod is flagged as over-provisioned.
func (e *Estimator) Recommendations(ctx context.Context) (*RecommendationSummary, error) {
	const ratioThreshold = 3.0

	pods, err := e.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var recs []Recommendation
	var totalSavings float64

	for _, pod := range pods.Items {
		ns := pod.Namespace
		if ns == "" {
			continue
		}

		for _, c := range pod.Spec.Containers {
			req := c.Resources.Requests
			lim := c.Resources.Limits

			reqCPU := resourceToFloat(req, corev1.ResourceCPU)
			reqRAM := resourceToFloat(req, corev1.ResourceMemory) / 1e9
			limCPU := resourceToFloat(lim, corev1.ResourceCPU)
			limRAM := resourceToFloat(lim, corev1.ResourceMemory) / 1e9

			if reqCPU == 0 && reqRAM == 0 {
				continue
			}

			var reason string
			overProvisioned := false

			if reqCPU > 0 && limCPU > 0 && limCPU/reqCPU > ratioThreshold {
				overProvisioned = true
				reason = fmt.Sprintf("CPU limit (%.2f cores) is %.1fx the request (%.2f cores)", limCPU, limCPU/reqCPU, reqCPU)
			}
			if reqRAM > 0 && limRAM > 0 && limRAM/reqRAM > ratioThreshold {
				overProvisioned = true
				if reason != "" {
					reason += "; "
				}
				reason += fmt.Sprintf("RAM limit (%.2f GB) is %.1fx the request (%.2f GB)", limRAM, limRAM/reqRAM, reqRAM)
			}

			if !overProvisioned {
				continue
			}

			recCPU := reqCPU * 2
			if recCPU == 0 {
				recCPU = limCPU
			}
			recRAM := reqRAM * 2
			if recRAM == 0 {
				recRAM = limRAM
			}

			cpuSavings := (limCPU - recCPU) * e.pricing.CPUPricePerCore
			ramSavings := (limRAM - recRAM) * e.pricing.RAMPricePerGB
			savings := cpuSavings + ramSavings

			currentCost := limCPU*e.pricing.CPUPricePerCore + limRAM*e.pricing.RAMPricePerGB
			var savingsPct float64
			if currentCost > 0 {
				savingsPct = savings / currentCost * 100
			}

			if savings <= 0 {
				continue
			}

			totalSavings += savings

			recs = append(recs, Recommendation{
				Workload:       fmt.Sprintf("%s/%s", ns, pod.Name),
				Kind:           "Pod",
				Namespace:      ns,
				ContainerName:  c.Name,
				Current:        Resources{CPUCores: roundTo(reqCPU, 3), RAMGB: roundTo(reqRAM, 3)},
				Limit:          Resources{CPUCores: roundTo(limCPU, 3), RAMGB: roundTo(limRAM, 3)},
				Recommended:    Resources{CPUCores: roundTo(recCPU, 3), RAMGB: roundTo(recRAM, 3)},
				MonthlySavings: roundTo(savings, 2),
				SavingsPct:     roundTo(savingsPct, 1),
				Reason:         reason,
			})
		}
	}

	sort.Slice(recs, func(i, j int) bool {
		return recs[i].MonthlySavings > recs[j].MonthlySavings
	})

	return &RecommendationSummary{
		TotalPotentialSavings: roundTo(totalSavings, 2),
		Count:                 len(recs),
		Recommendations:       recs,
		Pricing:               e.pricing,
		GeneratedAt:           time.Now().UTC(),
	}, nil
}

// resourceToFloat extracts a resource quantity as a float64.
func resourceToFloat(rl corev1.ResourceList, name corev1.ResourceName) float64 {
	q, ok := rl[name]
	if !ok {
		return 0
	}
	return q.AsApproximateFloat64()
}

// roundTo rounds to the given number of decimal places.
func roundTo(val float64, decimals int) float64 {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int64(val*mult+0.5)) / mult
}
