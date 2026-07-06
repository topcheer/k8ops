package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RLEResult is the resource limit & enforcement gap analysis.
type RLEResult struct {
	ScannedAt        time.Time    `json:"scannedAt"`
	Summary          RLESummary   `json:"summary"`
	ByWorkload       []RLEEntry   `json:"byWorkload"`
	Unbounded        []RLEEntry   `json:"unbounded"`        // no limits at all
	OverProvisioned  []RLEEntry   `json:"overProvisioned"`  // limits >> requests
	UnderProvisioned []RLEEntry   `json:"underProvisioned"` // requests close to limits
	ByNamespace      []RLENSEntry `json:"byNamespace"`
	Issues           []RLEIssue   `json:"issues"`
	Recommendations  []string     `json:"recommendations"`
}

// RLESummary aggregates resource limit statistics.
type RLESummary struct {
	TotalContainers     int `json:"totalContainers"`
	NoRequests          int `json:"noRequests"`
	NoLimits            int `json:"noLimits"`
	NoCPULimit          int `json:"noCPULimit"`
	NoMemLimit          int `json:"noMemLimit"`
	OverProvisioned     int `json:"overProvisioned"`     // limit/request > 4x
	UnderProvisioned    int `json:"underProvisioned"`    // limit/request < 1.2x
	GoodRatio           int `json:"goodRatio"`           // 1.2x-4x
	ExcessiveCPURequest int `json:"excessiveCPURequest"` // >2000m
	ExcessiveMemRequest int `json:"excessiveMemRequest"` // >4Gi
	ComplianceScore     int `json:"complianceScore"`     // 0-100
}

// RLEEntry describes one container's resource configuration.
type RLEEntry struct {
	Workload    string  `json:"workload"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	Container   string  `json:"container"`
	CPURequest  string  `json:"cpuRequest,omitempty"`
	CPULimit    string  `json:"cpuLimit,omitempty"`
	MemRequest  string  `json:"memRequest,omitempty"`
	MemLimit    string  `json:"memLimit,omitempty"`
	CPUReqMilli int64   `json:"cpuReqMilli"`
	CPULimMilli int64   `json:"cpuLimMilli"`
	MemReqMB    float64 `json:"memReqMB"`
	MemLimMB    float64 `json:"memLimMB"`
	CPURatio    float64 `json:"cpuRatio"` // limit/request (0 = no limit)
	MemRatio    float64 `json:"memRatio"`
	HasCPUReq   bool    `json:"hasCPUReq"`
	HasCPULim   bool    `json:"hasCPULim"`
	HasMemReq   bool    `json:"hasMemReq"`
	HasMemLim   bool    `json:"hasMemLim"`
	RiskLevel   string  `json:"riskLevel"`
	Issue       string  `json:"issue,omitempty"`
}

// RLENSEntry per-namespace stats.
type RLENSEntry struct {
	Namespace      string  `json:"namespace"`
	ContainerCount int     `json:"containerCount"`
	NoLimitCount   int     `json:"noLimitCount"`
	OverProvCount  int     `json:"overProvCount"`
	UnderProvCount int     `json:"underProvCount"`
	TotalCPUReqMi  int64   `json:"totalCPUReqMi"`
	TotalMemReqMB  float64 `json:"totalMemReqMB"`
	RiskLevel      string  `json:"riskLevel"`
}

// RLEIssue is a detected problem.
type RLEIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleResourceLimitsAudit audits resource limits and enforcement gaps.
// GET /api/deployment/resource-limits
func (s *Server) handleResourceLimitsAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := RLEResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*RLENSEntry)

	for _, dep := range deployments.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			entry := RLEEntry{
				Workload:  dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Container: c.Name,
			}

			// CPU request
			if req := c.Resources.Requests.Cpu(); req != nil && !req.IsZero() {
				entry.HasCPUReq = true
				entry.CPUReqMilli = req.MilliValue()
				entry.CPURequest = req.String()
			}
			// CPU limit
			if lim := c.Resources.Limits.Cpu(); lim != nil && !lim.IsZero() {
				entry.HasCPULim = true
				entry.CPULimMilli = lim.MilliValue()
				entry.CPULimit = lim.String()
			}
			// Memory request
			if req := c.Resources.Requests.Memory(); req != nil && !req.IsZero() {
				entry.HasMemReq = true
				entry.MemReqMB = float64(req.Value()) / (1024 * 1024)
				entry.MemRequest = req.String()
			}
			// Memory limit
			if lim := c.Resources.Limits.Memory(); lim != nil && !lim.IsZero() {
				entry.HasMemLim = true
				entry.MemLimMB = float64(lim.Value()) / (1024 * 1024)
				entry.MemLimit = lim.String()
			}

			// Calculate ratios
			if entry.HasCPUReq && entry.HasCPULim && entry.CPUReqMilli > 0 {
				entry.CPURatio = float64(entry.CPULimMilli) / float64(entry.CPUReqMilli)
			}
			if entry.HasMemReq && entry.HasMemLim && entry.MemReqMB > 0 {
				entry.MemRatio = entry.MemLimMB / entry.MemReqMB
			}

			// Classify
			entry.RiskLevel, entry.Issue = rleClassify(entry, &result.Summary)

			// Track stats
			if !entry.HasCPUReq {
				result.Summary.NoRequests++
			}
			if !entry.HasCPULim {
				result.Summary.NoCPULimit++
			}
			if !entry.HasMemLim {
				result.Summary.NoMemLimit++
			}
			if !entry.HasCPULim && !entry.HasMemLim {
				result.Summary.NoLimits++
			}

			// Excessive requests
			if entry.CPUReqMilli > 2000 {
				result.Summary.ExcessiveCPURequest++
			}
			if entry.MemReqMB > 4096 {
				result.Summary.ExcessiveMemRequest++
			}

			// Categorize
			if !entry.HasCPULim || !entry.HasMemLim {
				result.Unbounded = append(result.Unbounded, entry)
				if !entry.HasCPULim {
					result.Issues = append(result.Issues, RLEIssue{
						Severity: "warning", Type: "no-cpu-limit",
						Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
						Message:  fmt.Sprintf("Container %s/%s has no CPU limit — can consume entire node CPU", dep.Name, c.Name),
					})
				}
				if !entry.HasMemLim {
					result.Issues = append(result.Issues, RLEIssue{
						Severity: "critical", Type: "no-mem-limit",
						Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
						Message:  fmt.Sprintf("Container %s/%s has no memory limit — OOM kill risk and noisy neighbor", dep.Name, c.Name),
					})
				}
			}

			if entry.Issue == "over-provisioned" {
				result.OverProvisioned = append(result.OverProvisioned, entry)
			} else if entry.Issue == "under-provisioned" {
				result.UnderProvisioned = append(result.UnderProvisioned, entry)
			}

			// Namespace tracking
			nsStat := rleGetOrCreateNS(nsMap, dep.Namespace)
			nsStat.ContainerCount++
			nsStat.TotalCPUReqMi += entry.CPUReqMilli
			nsStat.TotalMemReqMB += entry.MemReqMB
			if !entry.HasCPULim || !entry.HasMemLim {
				nsStat.NoLimitCount++
			}
			if entry.Issue == "over-provisioned" {
				nsStat.OverProvCount++
			}
			if entry.Issue == "under-provisioned" {
				nsStat.UnderProvCount++
			}

			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		nsStat.RiskLevel = rleNSRisk(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return rleRiskRank(result.ByWorkload[i].RiskLevel) < rleRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Unbounded, func(i, j int) bool {
		return result.Unbounded[i].Namespace < result.Unbounded[j].Namespace
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].NoLimitCount > result.ByNamespace[j].NoLimitCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return rleIssueRank(result.Issues[i].Severity) < rleIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ComplianceScore = rleScore(result.Summary)
	result.Recommendations = rleGenRecs(result.Summary, result.Unbounded, result.OverProvisioned)

	writeJSON(w, result)
}

// rleClassify determines risk level and issue type.
func rleClassify(entry RLEEntry, summary *RLESummary) (riskLevel, issue string) {
	// No limits = unbounded
	if !entry.HasCPULim && !entry.HasMemLim {
		return "critical", "unbounded"
	}
	if !entry.HasMemLim {
		return "critical", "no-mem-limit"
	}
	if !entry.HasCPULim {
		return "high", "no-cpu-limit"
	}

	// Check ratios
	maxRatio := entry.CPURatio
	if entry.MemRatio > maxRatio {
		maxRatio = entry.MemRatio
	}
	minRatio := entry.CPURatio
	if entry.MemRatio > 0 && entry.MemRatio < minRatio {
		minRatio = entry.MemRatio
	}

	if maxRatio > 4 {
		summary.OverProvisioned++
		summary.GoodRatio++
		return "medium", "over-provisioned"
	}
	if minRatio > 0 && minRatio < 1.2 {
		summary.UnderProvisioned++
		return "high", "under-provisioned"
	}

	summary.GoodRatio++
	return "low", ""
}

// rleNSRisk determines namespace risk.
func rleNSRisk(ns RLENSEntry) string {
	if ns.NoLimitCount > 0 {
		return "critical"
	}
	if ns.OverProvCount > 2 {
		return "medium"
	}
	return "low"
}

// rleScore computes 0-100.
func rleScore(s RLESummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.NoLimits * 15
	score -= s.NoMemLimit * 8
	score -= s.NoCPULimit * 4
	score -= s.NoRequests * 5
	score -= s.UnderProvisioned * 3
	if score < 0 {
		score = 0
	}
	return score
}

// rleGenRecs produces actionable advice.
func rleGenRecs(s RLESummary, unbounded []RLEEntry, overProv []RLEEntry) []string {
	var recs []string

	if s.NoLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have NO resource limits at all — risk of resource starvation and node instability", s.NoLimits))
	}
	if s.NoMemLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing memory limits — OOM kills can crash the entire node", s.NoMemLimit))
	}
	if s.NoCPULimit > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing CPU limits — CPU throttling cannot protect other workloads", s.NoCPULimit))
	}
	if s.NoRequests > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing resource requests — scheduler cannot make informed placement decisions", s.NoRequests))
	}
	if s.UnderProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) are under-provisioned (limit/request < 1.2x) — tight burst headroom, frequent throttling risk", s.UnderProvisioned))
	}
	if s.OverProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) are over-provisioned (limit/request > 4x) — wasted capacity, consider tightening limits", s.OverProvisioned))
	}
	if s.ExcessiveCPURequest > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) request >2000m CPU — verify actual usage, potential waste", s.ExcessiveCPURequest))
	}
	if s.ExcessiveMemRequest > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) request >4Gi memory — verify actual usage, potential waste", s.ExcessiveMemRequest))
	}
	if s.ComplianceScore < 60 {
		recs = append(recs, fmt.Sprintf("Resource compliance score is %d/100 — multiple containers lack proper limits", s.ComplianceScore))
	}
	if s.NoLimits == 0 && s.NoMemLimit == 0 && s.NoCPULimit == 0 {
		recs = append(recs, "All containers have CPU and memory limits — good resource governance")
	}

	return recs
}

func rleGetOrCreateNS(m map[string]*RLENSEntry, ns string) *RLENSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &RLENSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func rleRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func rleIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// Ensure imports used
var _ = appsv1.DeploymentSpec{}
var _ = corev1.PodSpec{}
var _ = resource.Quantity{}
