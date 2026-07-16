package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SLOComplianceResult analyzes service SLO compliance:
// availability estimation, error budget burn rate,
// and per-namespace SLO target tracking.
type SLOComplianceResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SLOSummary          `json:"summary"`
	NamespaceSLOs   []NamespaceSLO      `json:"namespaceSLOs"`
	ComplianceScore int                  `json:"complianceScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type SLOSummary struct {
	TotalServices    int     `json:"totalServices"`
	HealthyServices  int     `json:"healthyServices"`
	DegradedServices int     `json:"degradedServices"`
	DownServices     int     `json:"downServices"`
	EstAvailability  float64 `json:"estAvailability"`
	ErrorBudgetPct   float64 `json:"errorBudgetPct"`
}

type NamespaceSLO struct {
	Namespace       string  `json:"namespace"`
	ServiceCount    int     `json:"serviceCount"`
	HealthyCount    int     `json:"healthyCount"`
	Availability    float64 `json:"availability"`
	Status          string  `json:"status"`
}

// handleSLOCompliance analyzes service SLO compliance and error budget.
// GET /api/product/slo-compliance
func (s *Server) handleSLOCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SLOComplianceResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build endpoint and pod health maps
	epReady := map[string]int{}  // ns/name -> ready addresses
	epTotal := map[string]int{}  // ns/name -> total addresses
	for _, ep := range endpoints.Items {
		key := ep.Namespace + "/" + ep.Name
		ready := len(ep.Subsets)
		for _, sub := range ep.Subsets {
			epReady[key] += len(sub.Addresses)
			epTotal[key] += len(sub.Addresses) + len(sub.NotReadyAddresses)
		}
		_ = ready
	}

	// Pod health per namespace
	nsPodReady := map[string]int{}
	nsPodTotal := map[string]int{}
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] { continue }
		nsPodTotal[pod.Namespace]++
		if pod.Status.Phase == "Running" {
			ready := true
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready { ready = false; break }
			}
			if ready { nsPodReady[pod.Namespace]++ }
		}
	}

	// Per-namespace SLO
	nsSvcCount := map[string]int{}
	nsSvcHealthy := map[string]int{}
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] { continue }
		result.Summary.TotalServices++
		key := svc.Namespace + "/" + svc.Name
		nsSvcCount[svc.Namespace]++

		healthy := epReady[key] > 0
		if healthy {
			result.Summary.HealthyServices++
			nsSvcHealthy[svc.Namespace]++
		} else if epTotal[key] > 0 {
			result.Summary.DegradedServices++
		} else {
			// No endpoints — may be headless or unused
			result.Summary.DownServices++
		}
	}

	// Build per-namespace SLO
	for ns, svcCount := range nsSvcCount {
		readyPods := nsPodReady[ns]
		totalPods := nsPodTotal[ns]
		avail := 100.0
		if totalPods > 0 {
			avail = float64(readyPods) / float64(totalPods) * 100
		}
		status := "healthy"
		if avail < 95 { status = "degraded" }
		if avail < 80 { status = "critical" }

		result.NamespaceSLOs = append(result.NamespaceSLOs, NamespaceSLO{
			Namespace: ns, ServiceCount: svcCount,
			HealthyCount: nsSvcHealthy[ns], Availability: avail, Status: status,
		})
	}

	// Overall availability
	totalReady := 0
	totalPods := 0
	for ns, ready := range nsPodReady {
		totalReady += ready
		totalPods += nsPodTotal[ns]
	}
	if totalPods > 0 {
		result.Summary.EstAvailability = float64(totalReady) / float64(totalPods) * 100
	}
	result.Summary.ErrorBudgetPct = 100 - result.Summary.EstAvailability

	// Score
	result.ComplianceScore = int(result.Summary.EstAvailability)
	result.Grade = goldenScoreToGrade(result.ComplianceScore)

	sort.Slice(result.NamespaceSLOs, func(i, j int) bool {
		return result.NamespaceSLOs[i].Availability < result.NamespaceSLOs[j].Availability
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("SLO compliance: %d/100 (grade %s) — %.1f%% availability", result.ComplianceScore, result.Grade, result.Summary.EstAvailability))
	if result.Summary.DegradedServices > 0 {
		recs = append(recs, fmt.Sprintf("%d degraded services — partial endpoint availability", result.Summary.DegradedServices))
	}
	if result.Summary.ErrorBudgetPct > 1 {
		recs = append(recs, fmt.Sprintf("Error budget burn: %.2f%% — exceeding 99%% SLO target", result.Summary.ErrorBudgetPct))
	}
	if len(recs) == 1 {
		recs = append(recs, "All services meeting SLO targets — maintain current monitoring")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
