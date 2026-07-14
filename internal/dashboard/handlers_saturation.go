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

// SaturationResult is the resource saturation & CPU/memory throttling risk audit.
type SaturationResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         SaturationSummary `json:"summary"`
	ThrottlingRisks []ThrottlingRisk  `json:"throttlingRisks"`
	SaturationByNS  []NSSaturation    `json:"saturationByNamespace"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// SaturationSummary aggregates saturation statistics.
type SaturationSummary struct {
	TotalPods          int    `json:"totalPods"`
	PodsWithCPULimits  int    `json:"podsWithCPULimits"`
	PodsWithMemLimits  int    `json:"podsWithMemLimits"`
	PodsWithoutLimits  int    `json:"podsWithoutLimits"`  // no limits at all
	HighCPULimitRatio  int    `json:"highCPULimitRatio"`  // limit >> request (>5x)
	HighMemLimitRatio  int    `json:"highMemLimitRatio"`  // limit >> request (>5x)
	ThrottlingRiskPods int    `json:"throttlingRiskPods"` // CPU limit < request (impossible) or very low
	OOMRiskPods        int    `json:"oomRiskPods"`        // no memory limit or limit == request
	UnboundedPods      int    `json:"unboundedPods"`      // no limits = unbounded resource use
	TotalCPURequested  string `json:"totalCPURequested"`
	TotalCPULimited    string `json:"totalCPULimited"`
	TotalMemRequested  string `json:"totalMemRequested"`
	TotalMemLimited    string `json:"totalMemLimited"`
}

// ThrottlingRisk describes a pod at risk of CPU throttling or OOM.
type ThrottlingRisk struct {
	Namespace  string `json:"namespace"`
	PodName    string `json:"podName"`
	OwnerKind  string `json:"ownerKind"`
	OwnerName  string `json:"ownerName"`
	Container  string `json:"container"`
	Issue      string `json:"issue"`
	CPURequest string `json:"cpuRequest,omitempty"`
	CPULimit   string `json:"cpuLimit,omitempty"`
	MemRequest string `json:"memRequest,omitempty"`
	MemLimit   string `json:"memLimit,omitempty"`
	LimitRatio string `json:"limitRatio,omitempty"`
	Severity   string `json:"severity"`
}

// NSSaturation shows per-namespace resource saturation.
type NSSaturation struct {
	Namespace     string `json:"namespace"`
	PodCount      int    `json:"podCount"`
	CPURequested  string `json:"cpuRequested"`
	CPULimited    string `json:"cpuLimited"`
	MemRequested  string `json:"memRequested"`
	MemLimited    string `json:"memLimited"`
	UnboundedPods int    `json:"unboundedPods"`
	RiskLevel     string `json:"riskLevel"`
}

// handleSaturation audits resource saturation & CPU/memory throttling risk.
// GET /api/scalability/saturation
func (s *Server) handleSaturation(w http.ResponseWriter, r *http.Request) {
	result := SaturationResult{
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

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsMap := make(map[string]*NSSaturation)
		totalCPUReq := resource.Quantity{}
		totalCPULim := resource.Quantity{}
		totalMemReq := resource.Quantity{}
		totalMemLim := resource.Quantity{}

		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}
			result.Summary.TotalPods++

			nsStat, ok := nsMap[pod.Namespace]
			if !ok {
				nsStat = &NSSaturation{Namespace: pod.Namespace}
				nsMap[pod.Namespace] = nsStat
			}
			nsStat.PodCount++

			ownerKind := getOwnerKind(pod.OwnerReferences)
			ownerName := getOwnerName(pod.OwnerReferences)

			for _, c := range pod.Spec.Containers {
				req := c.Resources.Requests
				lim := c.Resources.Limits

				hasCPULimit := false
				hasMemLimit := false
				cpuReq, cpuLim, memReq, memLim := "", "", "", ""

				if req != nil {
					if cpu, ok := req[corev1.ResourceCPU]; ok {
						cpuReq = cpu.String()
						totalCPUReq.Add(cpu)
					}
					if mem, ok := req[corev1.ResourceMemory]; ok {
						memReq = mem.String()
						totalMemReq.Add(mem)
					}
				}

				if lim != nil {
					if cpu, ok := lim[corev1.ResourceCPU]; ok {
						hasCPULimit = true
						cpuLim = cpu.String()
						totalCPULim.Add(cpu)
					}
					if mem, ok := lim[corev1.ResourceMemory]; ok {
						hasMemLimit = true
						memLim = mem.String()
						totalMemLim.Add(mem)
					}
				}

				if hasCPULimit {
					result.Summary.PodsWithCPULimits++
				}
				if hasMemLimit {
					result.Summary.PodsWithMemLimits++
				}

				// Check for unbounded pods (no limits at all)
				if !hasCPULimit && !hasMemLimit {
					result.Summary.UnboundedPods++
					result.Summary.PodsWithoutLimits++
					nsStat.UnboundedPods++
					result.ThrottlingRisks = append(result.ThrottlingRisks, ThrottlingRisk{
						Namespace:  pod.Namespace,
						PodName:    pod.Name,
						OwnerKind:  ownerKind,
						OwnerName:  ownerName,
						Container:  c.Name,
						Issue:      "No resource limits set — unbounded resource consumption risk",
						CPURequest: cpuReq,
						MemRequest: memReq,
						Severity:   "medium",
					})
				}

				// Check CPU limit/request ratio
				if hasCPULimit && cpuReq != "" {
					ratio := computeRatio(cpuLim, cpuReq)
					if ratio > 5.0 {
						result.Summary.HighCPULimitRatio++
						result.ThrottlingRisks = append(result.ThrottlingRisks, ThrottlingRisk{
							Namespace:  pod.Namespace,
							PodName:    pod.Name,
							OwnerKind:  ownerKind,
							OwnerName:  ownerName,
							Container:  c.Name,
							Issue:      fmt.Sprintf("CPU limit/request ratio %.1fx — high burst potential may cause throttling", ratio),
							CPURequest: cpuReq,
							CPULimit:   cpuLim,
							LimitRatio: fmt.Sprintf("%.1fx", ratio),
							Severity:   "low",
						})
					}
					if ratio < 1.0 {
						result.Summary.ThrottlingRiskPods++
						result.ThrottlingRisks = append(result.ThrottlingRisks, ThrottlingRisk{
							Namespace:  pod.Namespace,
							PodName:    pod.Name,
							OwnerKind:  ownerKind,
							OwnerName:  ownerName,
							Container:  c.Name,
							Issue:      "CPU limit lower than request — guaranteed throttling",
							CPURequest: cpuReq,
							CPULimit:   cpuLim,
							Severity:   "critical",
						})
					}
				}

				// Check memory: OOM risk if no limit or limit == request
				if !hasMemLimit {
					result.Summary.OOMRiskPods++
					result.ThrottlingRisks = append(result.ThrottlingRisks, ThrottlingRisk{
						Namespace:  pod.Namespace,
						PodName:    pod.Name,
						OwnerKind:  ownerKind,
						OwnerName:  ownerName,
						Container:  c.Name,
						Issue:      "No memory limit — OOM kill risk if node runs out of memory",
						MemRequest: memReq,
						Severity:   "high",
					})
				} else if hasMemLimit && memReq != "" {
					ratio := computeRatio(memLim, memReq)
					if ratio > 5.0 {
						result.Summary.HighMemLimitRatio++
						result.ThrottlingRisks = append(result.ThrottlingRisks, ThrottlingRisk{
							Namespace:  pod.Namespace,
							PodName:    pod.Name,
							OwnerKind:  ownerKind,
							OwnerName:  ownerName,
							Container:  c.Name,
							Issue:      fmt.Sprintf("Memory limit/request ratio %.1fx — high limit may cause node pressure", ratio),
							MemRequest: memReq,
							MemLimit:   memLim,
							LimitRatio: fmt.Sprintf("%.1fx", ratio),
							Severity:   "low",
						})
					}
				}

				// Accumulate per-namespace
				if cpuReq != "" {
					q := resource.MustParse(cpuReq)
					if cpuLim != "" {
						q2 := resource.MustParse(cpuLim)
						nsStat.CPULimited = addQuant(nsStat.CPULimited, q2)
					}
					nsStat.CPURequested = addQuant(nsStat.CPURequested, q)
				}
				if memReq != "" {
					q := resource.MustParse(memReq)
					if memLim != "" {
						q2 := resource.MustParse(memLim)
						nsStat.MemLimited = addQuant(nsStat.MemLimited, q2)
					}
					nsStat.MemRequested = addQuant(nsStat.MemRequested, q)
				}
			}
		}

		result.Summary.TotalCPURequested = totalCPUReq.String()
		result.Summary.TotalCPULimited = totalCPULim.String()
		result.Summary.TotalMemRequested = totalMemReq.String()
		result.Summary.TotalMemLimited = totalMemLim.String()

		// Build namespace stats
		for _, ns := range nsMap {
			ns.RiskLevel = "low"
			if ns.UnboundedPods > ns.PodCount/2 {
				ns.RiskLevel = "high"
			} else if ns.UnboundedPods > 0 {
				ns.RiskLevel = "medium"
			}
			result.SaturationByNS = append(result.SaturationByNS, *ns)
		}
		sort.Slice(result.SaturationByNS, func(i, j int) bool {
			return result.SaturationByNS[i].RiskLevel > result.SaturationByNS[j].RiskLevel
		})
	}

	sort.Slice(result.ThrottlingRisks, func(i, j int) bool {
		return result.ThrottlingRisks[i].Severity > result.ThrottlingRisks[j].Severity
	})

	// Recommendations
	if result.Summary.UnboundedPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have no resource limits — set limits to prevent resource monopolization", result.Summary.UnboundedPods))
	}
	if result.Summary.ThrottlingRiskPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have CPU limit < request — guaranteed throttling, fix limit values", result.Summary.ThrottlingRiskPods))
	}
	if result.Summary.OOMRiskPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have no memory limit — set limits to prevent OOM kill cascades", result.Summary.OOMRiskPods))
	}
	if result.Summary.HighCPULimitRatio > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have high CPU limit/request ratio (>5x) — consider tighter limits", result.Summary.HighCPULimitRatio))
	}

	// Health score
	score := 100
	score -= result.Summary.UnboundedPods * 2
	score -= result.Summary.ThrottlingRiskPods * 10
	score -= result.Summary.OOMRiskPods * 3
	score -= result.Summary.HighCPULimitRatio * 1
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

func computeRatio(limit, request string) float64 {
	limQ := resource.MustParse(limit)
	reqQ := resource.MustParse(request)
	limMillicores := float64(limQ.MilliValue())
	reqMillicores := float64(reqQ.MilliValue())
	if reqMillicores == 0 {
		return 0
	}
	return limMillicores / reqMillicores
}

func addQuant(existing string, q resource.Quantity) string {
	if existing == "" {
		return q.String()
	}
	existingQ := resource.MustParse(existing)
	existingQ.Add(q)
	return existingQ.String()
}

var _ = strings.Contains
