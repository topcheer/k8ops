package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AllocEfficiencyResult is the resource request vs limit allocation efficiency analysis.
type AllocEfficiencyResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         AllocEffSummary    `json:"summary"`
	ByNamespace     []AllocEffNSStat   `json:"byNamespace"`
	ByWorkload      []AllocEffWorkStat `json:"byWorkload"`
	Overallocated   []AllocEffEntry    `json:"overallocated"`
	Underallocated  []AllocEffEntry    `json:"underallocated"`
	NoLimits        []AllocEffEntry    `json:"noLimits"`
	NoRequests      []AllocEffEntry    `json:"noRequests"`
	Issues          []AllocEffIssue    `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// AllocEffSummary aggregates allocation efficiency statistics.
type AllocEffSummary struct {
	TotalContainers int     `json:"totalContainers"`
	WithRequests    int     `json:"withRequests"`
	WithLimits      int     `json:"withLimits"`
	NoRequests      int     `json:"noRequests"`
	NoLimits        int     `json:"noLimits"`
	Overallocated   int     `json:"overallocated"`
	Underallocated  int     `json:"underallocated"`
	TotalCPURequest string  `json:"totalCPURequest"`
	TotalCPULimit   string  `json:"totalCPULimit"`
	TotalMemRequest string  `json:"totalMemRequest"`
	TotalMemLimit   string  `json:"totalMemLimit"`
	AllocEfficiency float64 `json:"allocEfficiency"` // requests/limits ratio
}

// AllocEffNSStat per-namespace allocation stats.
type AllocEffNSStat struct {
	Namespace      string `json:"namespace"`
	ContainerCount int    `json:"containerCount"`
	NoRequests     int    `json:"noRequests"`
	NoLimits       int    `json:"noLimits"`
	Overallocated  int    `json:"overallocated"`
	Underallocated int    `json:"underallocated"`
}

// AllocEffWorkStat per-workload-type allocation stats.
type AllocEffWorkStat struct {
	WorkloadType     string  `json:"workloadType"`
	ContainerCount   int     `json:"containerCount"`
	AvgReqLimitRatio float64 `json:"avgReqLimitRatio"`
	Overallocated    int     `json:"overallocated"`
}

// AllocEffEntry describes one container's allocation efficiency.
type AllocEffEntry struct {
	PodName          string  `json:"podName"`
	Namespace        string  `json:"namespace"`
	Container        string  `json:"container"`
	WorkloadType     string  `json:"workloadType"`
	CPURequest       string  `json:"cpuRequest"`
	CPULimit         string  `json:"cpuLimit"`
	MemRequest       string  `json:"memRequest"`
	MemLimit         string  `json:"memLimit"`
	CPUReqLimitRatio float64 `json:"cpuReqLimitRatio"`
	MemReqLimitRatio float64 `json:"memReqLimitRatio"`
	IssueType        string  `json:"issueType"`
	RiskLevel        string  `json:"riskLevel"`
}

// AllocEffIssue is a detected allocation problem.
type AllocEffIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleAllocEfficiency audits resource request vs limit allocation efficiency.
// GET /api/scalability/alloc-efficiency
func (s *Server) handleAllocEfficiency(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &AllocEfficiencyResult{
		ScannedAt: time.Now(),
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var overallocated []AllocEffEntry
	var underallocated []AllocEffEntry
	var noLimits []AllocEffEntry
	var noRequests []AllocEffEntry
	var issues []AllocEffIssue

	totalContainers := 0
	withRequests := 0
	withLimits := 0
	noReqCount := 0
	noLimCount := 0
	overCount := 0
	underCount := 0

	var totalCPUReq, totalCPULim, totalMemReq, totalMemLim resource.Quantity

	nsStats := make(map[string]*AllocEffNSStat)
	workStats := make(map[string]*AllocEffWorkStat)

	for i := range pods.Items {
		pod := &pods.Items[i]
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		// Determine workload type
		workloadType := "Pod"
		for _, ref := range pod.OwnerReferences {
			if ref.Kind != "" {
				workloadType = ref.Kind
				break
			}
		}

		for _, c := range pod.Spec.Containers {
			totalContainers++
			req := c.Resources.Requests
			lim := c.Resources.Limits

			hasCPUReq := req.Cpu() != nil && !req.Cpu().IsZero()
			hasMemReq := req.Memory() != nil && !req.Memory().IsZero()
			hasCPULim := lim.Cpu() != nil && !lim.Cpu().IsZero()
			hasMemLim := lim.Memory() != nil && !lim.Memory().IsZero()

			hasReq := hasCPUReq || hasMemReq
			hasLim := hasCPULim || hasMemLim

			if hasReq {
				withRequests++
			} else {
				noReqCount++
			}
			if hasLim {
				withLimits++
			} else {
				noLimCount++
			}

			// Accumulate totals
			if req.Cpu() != nil {
				totalCPUReq.Add(*req.Cpu())
			}
			if lim.Cpu() != nil {
				totalCPULim.Add(*lim.Cpu())
			}
			if req.Memory() != nil {
				totalMemReq.Add(*req.Memory())
			}
			if lim.Memory() != nil {
				totalMemLim.Add(*req.Memory())
			}

			entry := AllocEffEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				Container:    c.Name,
				WorkloadType: workloadType,
			}

			if hasCPUReq {
				entry.CPURequest = req.Cpu().String()
			}
			if hasCPULim {
				entry.CPULimit = lim.Cpu().String()
			}
			if hasMemReq {
				entry.MemRequest = req.Memory().String()
			}
			if hasMemLim {
				entry.MemLimit = lim.Memory().String()
			}

			// Calculate CPU request/limit ratio
			if hasCPUReq && hasCPULim {
				reqVal := req.Cpu()
				limVal := lim.Cpu()
				if limVal.Sign() > 0 {
					ratio := float64(reqVal.MilliValue()) / float64(limVal.MilliValue())
					entry.CPUReqLimitRatio = ratio

					if ratio > 0.9 {
						// Request is nearly equal to limit — overallocated (wasted scheduling)
						overCount++
						entry.IssueType = "cpu-overallocated"
						entry.RiskLevel = "warning"
						overallocated = append(overallocated, entry)
						issues = append(issues, AllocEffIssue{
							Severity: "warning",
							Type:     "cpu-overallocated",
							Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
							Message:  fmt.Sprintf("CPU request %s is %.0f%% of limit %s — minimal headroom for bursts", req.Cpu().String(), ratio*100, lim.Cpu().String()),
						})
					} else if ratio < 0.1 {
						// Request much less than limit — underallocated (potential throttling risk)
						underCount++
						entry.IssueType = "cpu-underallocated"
						entry.RiskLevel = "warning"
						underallocated = append(underallocated, entry)
						issues = append(issues, AllocEffIssue{
							Severity: "warning",
							Type:     "cpu-underallocated",
							Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
							Message:  fmt.Sprintf("CPU request %s is only %.0f%% of limit %s — may cause scheduling pressure or CPU throttling", req.Cpu().String(), ratio*100, lim.Cpu().String()),
						})
					}
				}
			}

			// Calculate memory request/limit ratio
			if hasMemReq && hasMemLim {
				reqVal := req.Memory()
				limVal := lim.Memory()
				if limVal.Sign() > 0 {
					ratio := float64(reqVal.Value()) / float64(limVal.Value())
					entry.MemReqLimitRatio = ratio

					if ratio > 0.95 {
						overCount++
						if entry.IssueType == "" {
							entry.IssueType = "mem-overallocated"
							entry.RiskLevel = "warning"
							overallocated = append(overallocated, entry)
						}
						issues = append(issues, AllocEffIssue{
							Severity: "warning",
							Type:     "mem-overallocated",
							Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
							Message:  fmt.Sprintf("Memory request %s is %.0f%% of limit %s — minimal headroom, OOM risk", req.Memory().String(), ratio*100, lim.Memory().String()),
						})
					} else if ratio < 0.2 {
						underCount++
						if entry.IssueType == "" {
							entry.IssueType = "mem-underallocated"
							entry.RiskLevel = "info"
							underallocated = append(underallocated, entry)
						}
					}
				}
			}

			// No limits
			if !hasLim {
				noLimCount++
				entry.IssueType = "no-limits"
				entry.RiskLevel = "warning"
				noLimits = append(noLimits, entry)
				issues = append(issues, AllocEffIssue{
					Severity: "warning",
					Type:     "no-limits",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Container %s has no resource limits — may consume unlimited node resources", c.Name),
				})
			}

			// No requests
			if !hasReq {
				noReqCount++
				entry.IssueType = "no-requests"
				entry.RiskLevel = "critical"
				noRequests = append(noRequests, entry)
				issues = append(issues, AllocEffIssue{
					Severity: "critical",
					Type:     "no-requests",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Container %s has no resource requests — scheduler cannot make informed placement decisions", c.Name),
				})
			}

			// Update namespace stats
			if _, ok := nsStats[pod.Namespace]; !ok {
				nsStats[pod.Namespace] = &AllocEffNSStat{Namespace: pod.Namespace}
			}
			ns := nsStats[pod.Namespace]
			ns.ContainerCount++
			if !hasReq {
				ns.NoRequests++
			}
			if !hasLim {
				ns.NoLimits++
			}
			if entry.IssueType == "cpu-overallocated" || entry.IssueType == "mem-overallocated" {
				ns.Overallocated++
			}
			if entry.IssueType == "cpu-underallocated" || entry.IssueType == "mem-underallocated" {
				ns.Underallocated++
			}

			// Update workload stats
			if _, ok := workStats[workloadType]; !ok {
				workStats[workloadType] = &AllocEffWorkStat{WorkloadType: workloadType}
			}
			ws := workStats[workloadType]
			ws.ContainerCount++
			if entry.IssueType == "cpu-overallocated" || entry.IssueType == "mem-overallocated" {
				ws.Overallocated++
			}
			if entry.CPUReqLimitRatio > 0 {
				ws.AvgReqLimitRatio = (ws.AvgReqLimitRatio*float64(ws.ContainerCount-1) + entry.CPUReqLimitRatio) / float64(ws.ContainerCount)
			}
		}
	}

	// Convert namespace stats to slice
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].NoRequests > result.ByNamespace[j].NoRequests
	})

	// Convert workload stats to slice
	for _, ws := range workStats {
		result.ByWorkload = append(result.ByWorkload, *ws)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].Overallocated > result.ByWorkload[j].Overallocated
	})

	// Sort entries
	sort.Slice(overallocated, func(i, j int) bool {
		return overallocated[i].CPUReqLimitRatio > overallocated[j].CPUReqLimitRatio
	})
	sort.Slice(underallocated, func(i, j int) bool {
		return underallocated[i].CPUReqLimitRatio < underallocated[j].CPUReqLimitRatio
	})
	if len(overallocated) > 20 {
		overallocated = overallocated[:20]
	}
	if len(underallocated) > 20 {
		underallocated = underallocated[:20]
	}
	if len(noLimits) > 20 {
		noLimits = noLimits[:20]
	}
	if len(noRequests) > 20 {
		noRequests = noRequests[:20]
	}

	// Calculate allocation efficiency
	allocEff := 0.0
	if totalCPULim.Sign() > 0 {
		allocEff = float64(totalCPUReq.MilliValue()) / float64(totalCPULim.MilliValue())
	}

	// Recommendations
	var recommendations []string
	if noReqCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) have no resource requests — add CPU/memory requests for proper scheduling", noReqCount))
	}
	if noLimCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) have no resource limits — add limits to prevent resource monopolization", noLimCount))
	}
	if overCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) are overallocated (request ≈ limit) — reduce requests to give burst headroom", overCount))
	}
	if underCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) are underallocated (request << limit) — increase requests to match actual usage", underCount))
	}
	if allocEff > 0.9 {
		recommendations = append(recommendations, fmt.Sprintf("Overall CPU allocation efficiency is %.0f%% — requests nearly equal limits, reduce requests to improve scheduling", allocEff*100))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Resource allocation is well-balanced — requests and limits are properly configured")
	}

	result.Overallocated = overallocated
	result.Underallocated = underallocated
	result.NoLimits = noLimits
	result.NoRequests = noRequests
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = AllocEffSummary{
		TotalContainers: totalContainers,
		WithRequests:    withRequests,
		WithLimits:      withLimits,
		NoRequests:      noReqCount,
		NoLimits:        noLimCount,
		Overallocated:   overCount,
		Underallocated:  underCount,
		TotalCPURequest: totalCPUReq.String(),
		TotalCPULimit:   totalCPULim.String(),
		TotalMemRequest: totalMemReq.String(),
		TotalMemLimit:   totalMemLim.String(),
		AllocEfficiency: allocEff,
	}
	result.HealthScore = computeAllocEffScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// computeAllocEffScore computes a 0-100 health score.
func computeAllocEffScore(s AllocEffSummary, issueCount int) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	// No requests is most critical
	score -= s.NoRequests * 5
	// No limits is warning
	score -= s.NoLimits * 3
	// Overallocated
	score -= s.Overallocated * 2
	// Underallocated
	score -= s.Underallocated * 1
	// General issues
	score -= issueCount * 1
	// Penalize extreme allocation efficiency
	if s.AllocEfficiency > 0.95 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused warning
var _ = strings.TrimSpace
