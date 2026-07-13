package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostWasteResult is the idle resource cost waste & namespace cost attribution audit.
type CostWasteResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CostWasteSummary    `json:"summary"`
	ByNamespace     []CostWasteNSEntry  `json:"byNamespace"`
	IdleResources   []IdleResourceEntry `json:"idleResources"`
	OverProvisioned []OverProvEntry     `json:"overProvisioned"`
	Recommendations []string            `json:"recommendations"`
}

// CostWasteSummary aggregates cost waste statistics.
type CostWasteSummary struct {
	TotalPods         int     `json:"totalPods"`
	IdlePods          int     `json:"idlePods"`       // pods with 0 CPU/memory usage
	OverProvPods      int     `json:"overProvPods"`   // pods requesting >> using
	IdleNamespaces    int     `json:"idleNamespaces"` // namespaces with all idle pods
	TotalCPURequested string  `json:"totalCPURequested"`
	TotalMemRequested string  `json:"totalMemRequested"`
	IdleCPURequested  string  `json:"idleCPURequested"`
	IdleMemRequested  string  `json:"idleMemRequested"`
	WastePercent      float64 `json:"wastePercent"`
	HealthScore       int     `json:"healthScore"`
}

// CostWasteNSEntry shows cost per namespace.
type CostWasteNSEntry struct {
	Namespace    string  `json:"namespace"`
	PodCount     int     `json:"podCount"`
	IdlePods     int     `json:"idlePods"`
	CPURequested string  `json:"cpuRequested"`
	MemRequested string  `json:"memRequested"`
	IdleCPU      string  `json:"idleCPU"`
	IdleMem      string  `json:"idleMem"`
	WastePercent float64 `json:"wastePercent"`
	RiskLevel    string  `json:"riskLevel"`
}

// IdleResourceEntry describes an idle resource.
type IdleResourceEntry struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"` // cpu, memory
	Requested string `json:"requested"`
	Reason    string `json:"reason"`
}

// OverProvEntry describes an over-provisioned pod.
type OverProvEntry struct {
	PodName       string  `json:"podName"`
	Namespace     string  `json:"namespace"`
	CPURequest    string  `json:"cpuRequest"`
	CPULimit      string  `json:"cpuLimit"`
	MemRequest    string  `json:"memRequest"`
	MemLimit      string  `json:"memLimit"`
	OverProvScore float64 `json:"overProvScore"` // higher = more over-provisioned
	Reason        string  `json:"reason"`
}

// costWasteAuditCore performs the cost waste audit on pods (testable).
func costWasteAuditCore(pods []corev1.Pod) CostWasteResult {
	result := CostWasteResult{
		ScannedAt: time.Now(),
	}

	totalCPUReq := resource.Quantity{}
	totalMemReq := resource.Quantity{}
	idleCPUReq := resource.Quantity{}
	idleMemReq := resource.Quantity{}
	nsStats := make(map[string]*CostWasteNSEntry)

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &CostWasteNSEntry{Namespace: ns}
		}
		nsStats[ns].PodCount++
		result.Summary.TotalPods++

		podCPUReq := resource.Quantity{}
		podMemReq := resource.Quantity{}

		for _, c := range pod.Spec.Containers {
			cpuReq := c.Resources.Requests.Cpu()
			memReq := c.Resources.Requests.Memory()
			cpuLimit := c.Resources.Limits.Cpu()
			memLimit := c.Resources.Limits.Memory()

			if cpuReq != nil && !cpuReq.IsZero() {
				podCPUReq.Add(*cpuReq)
				totalCPUReq.Add(*cpuReq)
			}
			if memReq != nil && !memReq.IsZero() {
				podMemReq.Add(*memReq)
				totalMemReq.Add(*memReq)
			}

			// Check for over-provisioning: requests much higher than typical microservice needs
			if cpuReq != nil && !cpuReq.IsZero() {
				cpuMillis := cpuReq.MilliValue()
				if cpuMillis > 4000 { // >4 CPU cores requested
					result.OverProvisioned = append(result.OverProvisioned, OverProvEntry{
						PodName:       pod.Name,
						Namespace:     ns,
						CPURequest:    cpuReq.String(),
						CPULimit:      cpuLimit.String(),
						MemRequest:    memReq.String(),
						MemLimit:      memLimit.String(),
						OverProvScore: float64(cpuMillis) / 1000.0,
						Reason:        fmt.Sprintf("high CPU request (%s) — likely over-provisioned", cpuReq.String()),
					})
					result.Summary.OverProvPods++
				}
			}

			// Check for excessive memory requests (>8Gi)
			if memReq != nil && !memReq.IsZero() {
				memMi := memReq.Value() / (1024 * 1024)
				if memMi > 8192 {
					result.OverProvisioned = append(result.OverProvisioned, OverProvEntry{
						PodName:       pod.Name,
						Namespace:     ns,
						MemRequest:    memReq.String(),
						MemLimit:      memLimit.String(),
						OverProvScore: float64(memMi) / 1024.0,
						Reason:        fmt.Sprintf("high memory request (%s) — likely over-provisioned", memReq.String()),
					})
				}
			}

			// Pod has requests and limits — real workload
		}

		// A pod is considered "idle" if it has very low total resource requests
		// (<100m CPU and <128Mi memory) or no requests at all
		podCPUMillis := podCPUReq.MilliValue()
		podMemMi := podMemReq.Value() / (1024 * 1024)
		if (podCPUMillis <= 100 && podMemMi <= 128) || (podCPUReq.IsZero() && podMemReq.IsZero()) {
			result.Summary.IdlePods++
			nsStats[ns].IdlePods++
			idleCPUReq.Add(podCPUReq)
			idleMemReq.Add(podMemReq)

			if !podCPUReq.IsZero() {
				result.IdleResources = append(result.IdleResources, IdleResourceEntry{
					PodName: pod.Name, Namespace: ns,
					Resource: "cpu", Requested: podCPUReq.String(),
					Reason: "very low CPU request — possible idle/unused pod",
				})
			}
			if !podMemReq.IsZero() {
				result.IdleResources = append(result.IdleResources, IdleResourceEntry{
					PodName: pod.Name, Namespace: ns,
					Resource: "memory", Requested: podMemReq.String(),
					Reason: "very low memory request — possible idle/unused pod",
				})
			}
		}

		// Add to namespace stats
		nsStats[ns].CPURequested = podCPUReq.String()
		nsStats[ns].MemRequested = podMemReq.String()
	}

	// Calculate waste percentage
	if !totalCPUReq.IsZero() {
		result.Summary.WastePercent = float64(idleCPUReq.MilliValue()) / float64(totalCPUReq.MilliValue()) * 100
	}

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.PodCount > 0 && stat.IdlePods == stat.PodCount {
			result.Summary.IdleNamespaces++
			stat.RiskLevel = "high"
		} else if stat.IdlePods > 0 {
			stat.RiskLevel = "medium"
		} else {
			stat.RiskLevel = "low"
		}
		if stat.PodCount > 0 {
			stat.WastePercent = float64(stat.IdlePods) / float64(stat.PodCount) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].WastePercent > result.ByNamespace[j].WastePercent
	})

	sort.Slice(result.OverProvisioned, func(i, j int) bool {
		return result.OverProvisioned[i].OverProvScore > result.OverProvisioned[j].OverProvScore
	})

	result.Summary.TotalCPURequested = totalCPUReq.String()
	result.Summary.TotalMemRequested = totalMemReq.String()
	result.Summary.IdleCPURequested = idleCPUReq.String()
	result.Summary.IdleMemRequested = idleMemReq.String()

	result.Summary.HealthScore = costWasteScore(result.Summary)
	result.Recommendations = costWasteRecommendations(result.Summary)

	return result
}

// costWasteScore calculates health score.
func costWasteScore(s CostWasteSummary) int {
	base := 100
	// Penalty for idle pods
	if s.TotalPods > 0 {
		idlePct := float64(s.IdlePods) / float64(s.TotalPods) * 100
		base -= int(idlePct / 3)
	}
	// Penalty for idle namespaces
	base -= s.IdleNamespaces * 10
	// Penalty for over-provisioned pods
	base -= s.OverProvPods * 3
	// Penalty for high waste percentage
	if s.WastePercent > 30 {
		base -= 20
	} else if s.WastePercent > 15 {
		base -= 10
	}
	if base < 0 {
		base = 0
	}
	return base
}

// costWasteRecommendations generates recommendations.
func costWasteRecommendations(s CostWasteSummary) []string {
	var recs []string
	if s.IdlePods > 0 {
		recs = append(recs, fmt.Sprintf("%d idle pods detected (very low resource requests) — review and remove unused workloads to save costs", s.IdlePods))
	}
	if s.IdleNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have all idle pods — consider deleting these namespaces to reduce wasted resources", s.IdleNamespaces))
	}
	if s.OverProvPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pods are likely over-provisioned (>4 CPU or >8Gi memory requests) — right-size to reduce costs", s.OverProvPods))
	}
	if s.WastePercent > 0 {
		recs = append(recs, fmt.Sprintf("%.1f%% of requested resources are on idle pods — optimize resource allocation", s.WastePercent))
	}
	if s.IdlePods == 0 && s.OverProvPods == 0 && s.WastePercent < 5 {
		recs = append(recs, "resource allocation is well-optimized — minimal waste detected")
	}
	return recs
}

// handleCostWaste audits idle resource cost waste and namespace cost attribution.
// GET /api/scalability/cost-waste
func (s *Server) handleCostWaste(w http.ResponseWriter, r *http.Request) {
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

	result := costWasteAuditCore(pods.Items)
	writeJSON(w, result)
}

// Suppress unused import
var _ = strings.Contains
