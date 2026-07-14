package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BudgetAlertResult is the cost budget alert & namespace spending limit audit.
type BudgetAlertResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         BudgetSummary     `json:"summary"`
	Namespaces      []BudgetNamespace `json:"namespaces"`
	Alerts          []BudgetAlert     `json:"alerts"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// BudgetSummary aggregates budget statistics.
type BudgetSummary struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	NSWithBudget    int     `json:"nsWithBudget"`
	NSWithoutBudget int     `json:"nsWithoutBudget"`
	NSOverBudget    int     `json:"nsOverBudget"` // spending > budget
	NSNearBudget    int     `json:"nsNearBudget"` // spending > 80% of budget
	TotalEstCost    float64 `json:"totalEstCost"`
	TotalBudget     float64 `json:"totalBudget"`
}

// BudgetNamespace describes a namespace's budget status.
type BudgetNamespace struct {
	Namespace string  `json:"namespace"`
	HasBudget bool    `json:"hasBudget"`
	EstCost   float64 `json:"estCost"` // estimated monthly cost in USD
	Budget    float64 `json:"budget,omitempty"`
	UsagePct  float64 `json:"usagePct,omitempty"`
	Status    string  `json:"status"` // safe, warning, over-budget, no-budget
}

// BudgetAlert describes a budget alert.
type BudgetAlert struct {
	Namespace string  `json:"namespace"`
	EstCost   float64 `json:"estCost"`
	Budget    float64 `json:"budget"`
	UsagePct  float64 `json:"usagePct"`
	Issue     string  `json:"issue"`
	Severity  string  `json:"severity"`
}

// handleBudgetAlert audits cost budget alerts & namespace spending limits.
// GET /api/scalability/budget-alert
func (s *Server) handleBudgetAlert(w http.ResponseWriter, r *http.Request) {
	result := BudgetAlertResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// Cost estimation: $0.034/vCPU-hour, $0.004/GB-hour (approximate cloud rates)
	cpuCostPerHour := 0.034
	memCostPerGBHour := 0.004

	// Check for budget annotations on namespaces
	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsBudgetMap := make(map[string]float64)
		for _, ns := range namespaces.Items {
			if systemNamespaces[ns.Name] {
				continue
			}
			result.Summary.TotalNamespaces++

			budget := 0.0
			if ns.Annotations != nil {
				if b, ok := ns.Annotations["k8ops.io/monthly-budget"]; ok {
					fmt.Sscanf(b, "%f", &budget)
				}
				if b, ok := ns.Annotations["budget.io/monthly"]; ok {
					fmt.Sscanf(b, "%f", &budget)
				}
			}
			if budget > 0 {
				nsBudgetMap[ns.Name] = budget
				result.Summary.NSWithBudget++
			} else {
				result.Summary.NSWithoutBudget++
			}
		}

		// Calculate per-namespace resource usage and estimated cost
		pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
		if err == nil {
			nsCPU := make(map[string]*resource.Quantity)
			nsMem := make(map[string]*resource.Quantity)

			for _, pod := range pods.Items {
				if systemNamespaces[pod.Namespace] {
					continue
				}
				for _, c := range pod.Spec.Containers {
					if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						if nsCPU[pod.Namespace] == nil {
							q := resource.Quantity{}
							nsCPU[pod.Namespace] = &q
						}
						nsCPU[pod.Namespace].Add(req)
					}
					if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						if nsMem[pod.Namespace] == nil {
							q := resource.Quantity{}
							nsMem[pod.Namespace] = &q
						}
						nsMem[pod.Namespace].Add(req)
					}
				}
			}

			// Calculate cost per namespace
			for _, ns := range namespaces.Items {
				if systemNamespaces[ns.Name] {
					continue
				}

				cpuMillicores := int64(0)
				memBytes := int64(0)
				if nsCPU[ns.Name] != nil {
					cpuMillicores = nsCPU[ns.Name].MilliValue()
				}
				if nsMem[ns.Name] != nil {
					memBytes = nsMem[ns.Name].Value()
				}

				// Monthly cost = (CPU cores * cost/hour + Memory GB * cost/hour) * 730 hours
				cpuCores := float64(cpuMillicores) / 1000.0
				memGB := float64(memBytes) / (1024 * 1024 * 1024)
				estCost := (cpuCores*cpuCostPerHour + memGB*memCostPerGBHour) * 730

				budget := nsBudgetMap[ns.Name]
				usagePct := 0.0
				if budget > 0 {
					usagePct = estCost / budget * 100
				}

				status := "no-budget"
				if budget > 0 {
					status = "safe"
					if usagePct > 100 {
						status = "over-budget"
						result.Summary.NSOverBudget++
						result.Alerts = append(result.Alerts, BudgetAlert{
							Namespace: ns.Name,
							EstCost:   estCost,
							Budget:    budget,
							UsagePct:  usagePct,
							Issue:     fmt.Sprintf("Namespace over budget: $%.2f vs $%.2f (%.0f%%)", estCost, budget, usagePct),
							Severity:  "critical",
						})
					} else if usagePct > 80 {
						status = "warning"
						result.Summary.NSNearBudget++
						result.Alerts = append(result.Alerts, BudgetAlert{
							Namespace: ns.Name,
							EstCost:   estCost,
							Budget:    budget,
							UsagePct:  usagePct,
							Issue:     fmt.Sprintf("Namespace near budget limit: $%.2f vs $%.2f (%.0f%%)", estCost, budget, usagePct),
							Severity:  "high",
						})
					}
				}

				result.Summary.TotalEstCost += estCost
				result.Summary.TotalBudget += budget

				result.Namespaces = append(result.Namespaces, BudgetNamespace{
					Namespace: ns.Name,
					HasBudget: budget > 0,
					EstCost:   estCost,
					Budget:    budget,
					UsagePct:  usagePct,
					Status:    status,
				})
			}
		}
	}

	sort.Slice(result.Namespaces, func(i, j int) bool {
		return result.Namespaces[i].Status > result.Namespaces[j].Status
	})
	sort.Slice(result.Alerts, func(i, j int) bool {
		return result.Alerts[i].Severity > result.Alerts[j].Severity
	})

	// Recommendations
	if result.Summary.NSWithoutBudget > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have no budget annotation — add k8ops.io/monthly-budget annotation for cost tracking", result.Summary.NSWithoutBudget))
	}
	if result.Summary.NSOverBudget > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces are over budget — reduce resource requests or increase budget", result.Summary.NSOverBudget))
	}
	if result.Summary.NSNearBudget > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces are near budget limit (>80%%) — monitor spending", result.Summary.NSNearBudget))
	}

	// Health score
	score := 100
	score -= result.Summary.NSOverBudget * 10
	score -= result.Summary.NSNearBudget * 5
	score -= result.Summary.NSWithoutBudget * 2
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
