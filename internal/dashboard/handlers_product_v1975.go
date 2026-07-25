package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.75 — Product Dimension (Round 15)
// 1. Pod Uptime Tracker — per-pod & per-NS uptime stats
// 2. Namespace Cost Summary — estimated cost per namespace
// 3. Replica Health Summary — deployment replica readiness overview
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Uptime Tracker
// ---------------------------------------------------------------

type PodUptimeResult1975 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PodUptimeSummary1975   `json:"summary"`
	PerNS           []PodUptimeNSEntry1975 `json:"perNamespace"`
	Recommendations []string               `json:"recommendations"`
}

type PodUptimeSummary1975 struct {
	TotalPods          int     `json:"totalPods"`
	AvgUptimeHours     float64 `json:"avgUptimeHours"`
	MaxUptimeDays      float64 `json:"maxUptimeDays"`
	TotalUptimeSeconds int64   `json:"totalUptimeSeconds"`
}

type PodUptimeNSEntry1975 struct {
	Namespace string  `json:"namespace"`
	PodCount  int     `json:"podCount"`
	AvgHours  float64 `json:"avgUptimeHours"`
}

func (s *Server) handlePodUptimeTracker(w http.ResponseWriter, r *http.Request) {
	result := PodUptimeResult1975{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*PodUptimeNSEntry1975)
	var totalUptime float64
	var maxUptime time.Duration

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		var uptime time.Duration
		// Use container status ready time if available
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Running != nil && !cs.State.Running.StartedAt.IsZero() {
				uptime = time.Since(cs.State.Running.StartedAt.Time)
				break
			}
		}
		// Fallback to creation timestamp
		if uptime == 0 && !pod.CreationTimestamp.IsZero() {
			uptime = time.Since(pod.CreationTimestamp.Time)
		}

		totalUptime += uptime.Hours()
		if uptime > maxUptime {
			maxUptime = uptime
		}

		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &PodUptimeNSEntry1975{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.PodCount++
		ns.AvgHours += uptime.Hours()
	}

	for _, ns := range nsStats {
		if ns.PodCount > 0 {
			ns.AvgHours /= float64(ns.PodCount)
		}
		result.PerNS = append(result.PerNS, *ns)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].AvgHours > result.PerNS[j].AvgHours
	})

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgUptimeHours = totalUptime / float64(result.Summary.TotalPods)
	}
	result.Summary.MaxUptimeDays = maxUptime.Hours() / 24

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg uptime %.0fh, max %.0fd", result.Summary.TotalPods, result.Summary.AvgUptimeHours, result.Summary.MaxUptimeDays))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Namespace Cost Summary
// ---------------------------------------------------------------

type NSCostResult1975 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         NSCostSummary1975 `json:"summary"`
	PerNS           []NSCostEntry1975 `json:"perNamespace"`
	Recommendations []string          `json:"recommendations"`
}

type NSCostSummary1975 struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	TotalCPUReq     float64 `json:"totalCPURequested"`
	TotalMemReq     float64 `json:"totalMemRequestedGB"`
	TotalPods       int     `json:"totalPods"`
	EstMonthlyCost  float64 `json:"estimatedMonthlyCostUSD"`
}

type NSCostEntry1975 struct {
	Namespace   string  `json:"namespace"`
	CPUReq      float64 `json:"cpuRequest"`
	MemReq      float64 `json:"memRequestGB"`
	PodCount    int     `json:"podCount"`
	MonthlyCost float64 `json:"estimatedMonthlyCostUSD"`
}

func (s *Server) handleNamespaceCostSummary(w http.ResponseWriter, r *http.Request) {
	result := NSCostResult1975{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	// Cost model: CPU $0.03/core/hour, Memory $0.004/GB/hour
	const cpuCostPerCoreHour = 0.03
	const memCostPerGBHour = 0.004
	const hoursPerMonth = 730.0

	nsStats := make(map[string]*NSCostEntry1975)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &NSCostEntry1975{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.PodCount++
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			cpu := c.Resources.Requests.Cpu().AsApproximateFloat64()
			mem := float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
			ns.CPUReq += cpu
			ns.MemReq += mem
			result.Summary.TotalCPUReq += cpu
			result.Summary.TotalMemReq += mem
		}
	}

	var totalCost float64
	for _, ns := range nsStats {
		ns.MonthlyCost = (ns.CPUReq*cpuCostPerCoreHour + ns.MemReq*memCostPerGBHour) * hoursPerMonth
		totalCost += ns.MonthlyCost
		result.PerNS = append(result.PerNS, *ns)
	}
	result.Summary.EstMonthlyCost = totalCost
	result.Summary.TotalNamespaces = len(nsList.Items)

	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].MonthlyCost > result.PerNS[j].MonthlyCost
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces, %.1f CPU, %.1f GB mem, est $%.2f/mo", result.Summary.TotalNamespaces, result.Summary.TotalCPUReq, result.Summary.TotalMemReq, result.Summary.EstMonthlyCost))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Replica Health Summary
// ---------------------------------------------------------------

type ReplicaHealthResult1975 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ReplicaHealthSummary1975 `json:"summary"`
	Unhealthy       []ReplicaHealthEntry1975 `json:"unhealthyDeployments"`
	Healthy         []ReplicaHealthEntry1975 `json:"healthyDeployments"`
	Recommendations []string                 `json:"recommendations"`
}

type ReplicaHealthSummary1975 struct {
	TotalDeployments int `json:"totalDeployments"`
	FullyReady       int `json:"fullyReady"`
	PartiallyReady   int `json:"partiallyReady"`
	NotReady         int `json:"notReady"`
	ZeroReplicas     int `json:"zeroReplicas"`
}

type ReplicaHealthEntry1975 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Desired   int    `json:"desiredReplicas"`
	Ready     int    `json:"readyReplicas"`
	Status    string `json:"status"`
}

func (s *Server) handleReplicaHealthSummary(w http.ResponseWriter, r *http.Request) {
	result := ReplicaHealthResult1975{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		desired := 0
		if dep.Spec.Replicas != nil {
			desired = int(*dep.Spec.Replicas)
		}
		ready := int(dep.Status.ReadyReplicas)

		entry := ReplicaHealthEntry1975{
			Name: dep.Name, Namespace: dep.Namespace,
			Desired: desired, Ready: ready,
		}

		if desired == 0 {
			entry.Status = "zero"
			result.Summary.ZeroReplicas++
		} else if ready == desired {
			entry.Status = "ready"
			result.Summary.FullyReady++
			result.Healthy = append(result.Healthy, entry)
		} else if ready > 0 {
			entry.Status = "partial"
			result.Summary.PartiallyReady++
			result.Unhealthy = append(result.Unhealthy, entry)
			score -= 3
		} else {
			entry.Status = "not-ready"
			result.Summary.NotReady++
			result.Unhealthy = append(result.Unhealthy, entry)
			score -= 5
		}
	}

	sort.Slice(result.Unhealthy, func(i, j int) bool {
		return result.Unhealthy[i].Ready < result.Unhealthy[j].Ready
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d ready, %d partial, %d not-ready, %d zero", result.Summary.TotalDeployments, result.Summary.FullyReady, result.Summary.PartiallyReady, result.Summary.NotReady, result.Summary.ZeroReplicas))
	if len(result.Unhealthy) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unhealthy deployments need attention", len(result.Unhealthy)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
