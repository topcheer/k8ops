package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChargebackResult produces a detailed cost chargeback report:
// per-namespace cost breakdown, resource waste cost, shared infrastructure cost,
// and team-level budget allocation recommendations.
type ChargebackResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ChargebackSummary `json:"summary"`
	NamespaceCosts  []NamespaceCost   `json:"namespaceCosts"`
	SharedCosts     []SharedCost      `json:"sharedCosts"`
	WasteCost       float64           `json:"wasteCost"`
	QualityScore    int               `json:"qualityScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type ChargebackSummary struct {
	TotalMonthlyCost float64 `json:"totalMonthlyCost"`
	ComputeCost      float64 `json:"computeCost"`
	StorageCost      float64 `json:"storageCost"`
	NetworkCost      float64 `json:"networkCost"`
	SharedInfraCost  float64 `json:"sharedInfraCost"`
	NamespaceCount   int     `json:"namespaceCount"`
	TopNS            string  `json:"topNamespace"`
	AvgCostPerNS     float64 `json:"avgCostPerNS"`
}

type NamespaceCost struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	CPURequest  float64 `json:"cpuRequestCores"`
	MemRequest  float64 `json:"memRequestGB"`
	PVCGB       float64 `json:"pvcGB"`
	LBCost      float64 `json:"lbCost"`
	MonthlyCost float64 `json:"monthlyCost"`
	Pct         float64 `json:"pctOfTotal"`
}

type SharedCost struct {
	Category    string  `json:"category"`
	MonthlyCost float64 `json:"monthlyCost"`
	Description string  `json:"description"`
}

// Cost constants
const (
	cbCPUCoreMonthly = 25.0
	cbMemGBMonthly   = 4.0
	cbPVCGBMonthly   = 0.10
	cbLBMonthly      = 18.0
	cbNodeBase       = 50.0
)

// handleChargeback produces a detailed cost chargeback report.
// GET /api/scalability/chargeback
func (s *Server) handleChargeback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChargebackResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Build per-namespace cost map
	nsCostMap := map[string]*NamespaceCost{}

	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		ns := pod.Namespace
		if nsCostMap[ns] == nil {
			nsCostMap[ns] = &NamespaceCost{Namespace: ns}
		}
		nsCostMap[ns].PodCount++

		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests["cpu"]; ok {
					nsCostMap[ns].CPURequest += float64(q.MilliValue()) / 1000.0
				}
				if q, ok := c.Resources.Requests["memory"]; ok {
					nsCostMap[ns].MemRequest += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}
	}

	// Add PVC costs
	for _, pvc := range pvcs.Items {
		if systemNS[pvc.Namespace] || pvc.Status.Phase != "Bound" {
			continue
		}
		ns := pvc.Namespace
		if nsCostMap[ns] == nil {
			nsCostMap[ns] = &NamespaceCost{Namespace: ns}
		}
		if q, ok := pvc.Spec.Resources.Requests["storage"]; ok {
			gb := float64(q.Value()) / (1024 * 1024 * 1024)
			nsCostMap[ns].PVCGB += gb
		}
	}

	// Add LB costs
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		if svc.Spec.Type == "LoadBalancer" {
			ns := svc.Namespace
			if nsCostMap[ns] == nil {
				nsCostMap[ns] = &NamespaceCost{Namespace: ns}
			}
			nsCostMap[ns].LBCost += cbLBMonthly
		}
	}

	// Calculate monthly cost per namespace
	totalCost := 0.0
	for _, nc := range nsCostMap {
		nc.MonthlyCost = nc.CPURequest*cbCPUCoreMonthly + nc.MemRequest*cbMemGBMonthly + nc.PVCGB*cbPVCGBMonthly + nc.LBCost
		totalCost += nc.MonthlyCost
	}

	// Shared infrastructure cost (node base cost)
	nodeCount := len(nodes.Items)
	sharedCost := float64(nodeCount) * cbNodeBase
	result.SharedCosts = append(result.SharedCosts, SharedCost{
		Category: "node-base", MonthlyCost: sharedCost,
		Description: fmt.Sprintf("Base node cost for %d nodes", nodeCount),
	})

	// Calculate percentages
	for _, nc := range nsCostMap {
		if totalCost > 0 {
			nc.Pct = nc.MonthlyCost / totalCost * 100
		}
		result.NamespaceCosts = append(result.NamespaceCosts, *nc)
	}

	// Summary
	result.Summary.NamespaceCount = len(result.NamespaceCosts)
	result.Summary.TotalMonthlyCost = totalCost + sharedCost
	result.Summary.ComputeCost = totalCost * 0.6
	result.Summary.StorageCost = totalCost * 0.15
	result.Summary.NetworkCost = totalCost * 0.1
	result.Summary.SharedInfraCost = sharedCost
	if result.Summary.NamespaceCount > 0 {
		result.Summary.AvgCostPerNS = totalCost / float64(result.Summary.NamespaceCount)
	}

	// Sort by cost descending
	sort.Slice(result.NamespaceCosts, func(i, j int) bool {
		return result.NamespaceCosts[i].MonthlyCost > result.NamespaceCosts[j].MonthlyCost
	})
	if len(result.NamespaceCosts) > 0 {
		result.Summary.TopNS = result.NamespaceCosts[0].Namespace
	}

	// Waste estimate: namespaces with 0 CPU request but pods running
	waste := 0.0
	for _, nc := range result.NamespaceCosts {
		if nc.CPURequest == 0 && nc.PodCount > 0 {
			waste += float64(nc.PodCount) * 5.0 // $5/month per pod without resource requests
		}
	}
	result.WasteCost = waste

	// Score: higher cost efficiency = better score
	score := 70
	if waste > 50 {
		score -= 20
	}
	if totalCost > 500 {
		score -= 10
	}
	if result.Summary.NamespaceCount > 10 {
		score += 10
	}
	result.QualityScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.QualityScore)

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Cost chargeback: %d/100 (grade %s) — $%.2f/month total", result.QualityScore, result.Grade, result.Summary.TotalMonthlyCost))
	if len(result.NamespaceCosts) > 0 {
		recs = append(recs, fmt.Sprintf("Top namespace: '%s' ($%.2f/month, %.1f%%)", result.Summary.TopNS, result.NamespaceCosts[0].MonthlyCost, result.NamespaceCosts[0].Pct))
	}
	if waste > 0 {
		recs = append(recs, fmt.Sprintf("$%.2f/month waste from pods without resource requests — add requests for cost tracking", waste))
	}
	if sharedCost > 0 {
		recs = append(recs, fmt.Sprintf("Shared infra: $%.2f/month for %d nodes — distribute across teams via quota allocation", sharedCost, nodeCount))
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
