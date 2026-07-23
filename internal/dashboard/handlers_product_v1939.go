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
// v19.39 — Product Dimension (Round 9)
// 1. Container Resource Wastage — overcommitted limits vs requests
// 2. Service Account Usage Tracker — used vs orphaned SAs
// 3. Endpoint Slice Health — slice distribution & readiness
// ============================================================

// ---------------------------------------------------------------
// 1. Container Resource Wastage — overcommitted limits vs requests
// ---------------------------------------------------------------

type ResWastageResult1939 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ResWastageSummary1939 `json:"summary"`
	Containers      []ResWastageEntry1939 `json:"containers"`
	TopWaste        []ResWastageEntry1939 `json:"topWaste"`
	Recommendations []string              `json:"recommendations"`
}

type ResWastageSummary1939 struct {
	TotalContainers  int     `json:"totalContainers"`
	WithLimits       int     `json:"withLimits"`
	WithoutLimits    int     `json:"withoutLimits"`
	OvercommCPUCores float64 `json:"overcommittedCPUCores"`
	OvercommMemGB    float64 `json:"overcommittedMemGB"`
	EstWasteUSD      float64 `json:"estWasteUSD"`
	HighRatioCount   int     `json:"highRatioCount"`
}

type ResWastageEntry1939 struct {
	PodName   string  `json:"podName"`
	Namespace string  `json:"namespace"`
	Container string  `json:"container"`
	CPUReq    float64 `json:"cpuReqCores"`
	CPULim    float64 `json:"cpuLimCores"`
	MemReqMB  int     `json:"memReqMB"`
	MemLimMB  int     `json:"memLimMB"`
	CPURatio  float64 `json:"cpuLimitReqRatio"`
	MemRatio  float64 `json:"memLimitReqRatio"`
}

func (s *Server) handleResWastage(w http.ResponseWriter, r *http.Request) {
	result := ResWastageResult1939{ScannedAt: time.Now()}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	var totalOverCPU, totalOverMem float64

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			cpuReq := c.Resources.Requests.Cpu().AsApproximateFloat64()
			cpuLim := c.Resources.Limits.Cpu().AsApproximateFloat64()
			memReqMB := int(c.Resources.Requests.Memory().Value() / (1024 * 1024))
			memLimMB := int(c.Resources.Limits.Memory().Value() / (1024 * 1024))

			entry := ResWastageEntry1939{
				PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				CPUReq: cpuReq, CPULim: cpuLim, MemReqMB: memReqMB, MemLimMB: memLimMB,
			}

			if cpuLim > 0 {
				result.Summary.WithLimits++
				entry.CPURatio = cpuLim / maxFloat1939(cpuReq, 0.001)
				if entry.CPURatio > 4 {
					result.Summary.HighRatioCount++
					result.TopWaste = append(result.TopWaste, entry)
					totalOverCPU += (cpuLim - cpuReq)
					score -= 1
				}
			} else {
				result.Summary.WithoutLimits++
			}

			if memLimMB > 0 && memReqMB > 0 {
				entry.MemRatio = float64(memLimMB) / float64(maxInt1939(memReqMB, 1))
				if entry.MemRatio > 3 {
					totalOverMem += float64(memLimMB-memReqMB) / 1024
					score -= 1
				}
			}

			if len(result.Containers) < 100 {
				result.Containers = append(result.Containers, entry)
			}
		}
	}

	result.Summary.OvercommCPUCores = totalOverCPU
	result.Summary.OvercommMemGB = totalOverMem
	result.Summary.EstWasteUSD = totalOverCPU*28 + totalOverMem*3.5

	sort.Slice(result.TopWaste, func(i, j int) bool {
		return result.TopWaste[i].CPURatio > result.TopWaste[j].CPURatio
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OvercommCPUCores > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%.1f CPU cores overcommitted (limit >> request) — tighten limits", result.Summary.OvercommCPUCores))
	}
	if result.Summary.HighRatioCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with >4x limit/request ratio — review for throttling risk", result.Summary.HighRatioCount))
	}
	if result.Summary.WithoutLimits > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without limits — add for predictable resource governance", result.Summary.WithoutLimits))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func maxFloat1939(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt1939(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------
// 2. Service Account Usage Tracker
// ---------------------------------------------------------------

type SAUsageResult1939 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SAUsageSummary1939  `json:"summary"`
	UsedSAs         []SAUsageEntry1939  `json:"usedSAs"`
	OrphanedSAs     []SAOrphanEntry1939 `json:"orphanedSAs"`
	Recommendations []string            `json:"recommendations"`
}

type SAUsageSummary1939 struct {
	TotalSAs       int `json:"totalServiceAccounts"`
	UsedSAs        int `json:"usedServiceAccounts"`
	OrphanedSAs    int `json:"orphanedServiceAccounts"`
	DefaultSAUsed  int `json:"defaultSAUsed"`
	WithToken      int `json:"withTokenSecret"`
	WithAnnotation int `json:"withAnnotation"`
}

type SAUsageEntry1939 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	IsDefault bool   `json:"isDefault"`
}

type SAOrphanEntry1939 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
}

func (s *Server) handleSAUsageTracker(w http.ResponseWriter, r *http.Request) {
	result := SAUsageResult1939{ScannedAt: time.Now()}
	score := 100

	saList, err := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count SA usage from pods
	saUsage := make(map[string]int) // "ns/sa" -> pod count
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		key := fmt.Sprintf("%s/%s", pod.Namespace, saName)
		saUsage[key]++
	}

	for _, sa := range saList.Items {
		if isSystemNamespace(sa.Namespace) {
			continue
		}
		result.Summary.TotalSAs++

		key := fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)
		podCount := saUsage[key]
		isDefault := sa.Name == "default"

		if podCount > 0 {
			result.Summary.UsedSAs++
			result.UsedSAs = append(result.UsedSAs, SAUsageEntry1939{
				Name: sa.Name, Namespace: sa.Namespace,
				PodCount: podCount, IsDefault: isDefault,
			})
			if isDefault {
				result.Summary.DefaultSAUsed++
			}
		} else {
			result.Summary.OrphanedSAs++
			age := fmt.Sprintf("%.0fd", time.Since(sa.CreationTimestamp.Time).Hours()/24)
			reason := "ServiceAccount not used by any running pod"
			if isDefault {
				reason = "Default SA unused — normal if namespace has no pods"
			}
			result.OrphanedSAs = append(result.OrphanedSAs, SAOrphanEntry1939{
				Name: sa.Name, Namespace: sa.Namespace, Age: age, Reason: reason,
			})
			if !isDefault {
				score -= 2
			}
		}

		if len(sa.Secrets) > 0 || len(sa.ImagePullSecrets) > 0 {
			result.Summary.WithToken++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OrphanedSAs > 5 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d orphaned ServiceAccounts — clean up unused", result.Summary.OrphanedSAs))
	}
	if result.Summary.DefaultSAUsed > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces using default SA — create dedicated SAs", result.Summary.DefaultSAUsed))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Endpoint Slice Health
// ---------------------------------------------------------------

type EPSliceResult1939 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EPSliceSummary1939 `json:"summary"`
	Services        []EPSliceEntry1939 `json:"services"`
	Issues          []EPSliceIssue1939 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type EPSliceSummary1939 struct {
	TotalSlices     int `json:"totalSlices"`
	TotalEndpoints  int `json:"totalEndpoints"`
	ReadyEndpoints  int `json:"readyEndpoints"`
	NotReadyEPs     int `json:"notReadyEndpoints"`
	ServicesCovered int `json:"servicesCovered"`
	ServicesNoEP    int `json:"servicesNoEndpoints"`
}

type EPSliceEntry1939 struct {
	ServiceName string `json:"serviceName"`
	Namespace   string `json:"namespace"`
	SliceCount  int    `json:"sliceCount"`
	Endpoints   int    `json:"endpoints"`
	Ready       int    `json:"readyEndpoints"`
}

type EPSliceIssue1939 struct {
	ServiceName string `json:"serviceName"`
	Namespace   string `json:"namespace"`
	IssueType   string `json:"issueType"`
	Severity    string `json:"severity"`
	Detail      string `json:"detail"`
}

func (s *Server) handleEPSliceHealth(w http.ResponseWriter, r *http.Request) {
	result := EPSliceResult1939{ScannedAt: time.Now()}
	score := 100

	// EndpointSlices are in discovery.k8s.io/v1
	// Use the discovery client
	epsList, err := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		// Fallback: use Endpoints API
		epList, err2 := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
		if err2 != nil {
			writeJSON(w, result)
			return
		}
		for _, ep := range epList.Items {
			if isSystemNamespace(ep.Namespace) {
				continue
			}
			readyCount := 0
			notReadyCount := 0
			for _, subset := range ep.Subsets {
				readyCount += len(subset.Addresses)
				notReadyCount += len(subset.NotReadyAddresses)
			}
			result.Summary.TotalEndpoints += readyCount + notReadyCount
			result.Summary.ReadyEndpoints += readyCount
			result.Summary.NotReadyEPs += notReadyCount
			if readyCount > 0 {
				result.Summary.ServicesCovered++
			} else {
				result.Summary.ServicesNoEP++
				result.Issues = append(result.Issues, EPSliceIssue1939{
					ServiceName: ep.Name, Namespace: ep.Namespace,
					IssueType: "no-ready-endpoints", Severity: "high",
					Detail: "Service has no ready endpoints — traffic will fail",
				})
				score -= 5
			}
			result.Services = append(result.Services, EPSliceEntry1939{
				ServiceName: ep.Name, Namespace: ep.Namespace,
				Endpoints: readyCount + notReadyCount, Ready: readyCount,
			})
		}
		result.Summary.TotalSlices = len(epList.Items)
		if score < 0 {
			score = 0
		}
		result.HealthScore = score
		result.Grade = scoreToGrade(score)

		if result.Summary.ServicesNoEP > 0 {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services with no endpoints — check pod selectors", result.Summary.ServicesNoEP))
		}
		if result.Summary.NotReadyEPs > 0 {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d not-ready endpoints — check pod health probes", result.Summary.NotReadyEPs))
		}
		sort.Strings(result.Recommendations)
		writeJSON(w, result)
		return
	}

	// EndpointSlice path
	svcStats := make(map[string]*EPSliceEntry1939)
	for _, eps := range epsList.Items {
		if isSystemNamespace(eps.Namespace) {
			continue
		}
		result.Summary.TotalSlices++

		svcName := ""
		if eps.Labels["kubernetes.io/service-name"] != "" {
			svcName = eps.Labels["kubernetes.io/service-name"]
		} else {
			svcName = eps.Name
		}

		key := fmt.Sprintf("%s/%s", eps.Namespace, svcName)
		if svcStats[key] == nil {
			svcStats[key] = &EPSliceEntry1939{
				ServiceName: svcName, Namespace: eps.Namespace,
			}
		}
		svcStats[key].SliceCount++

		for _, ep := range eps.Endpoints {
			result.Summary.TotalEndpoints++
			svcStats[key].Endpoints++
			ready := false
			if ep.Conditions.Ready != nil {
				ready = *ep.Conditions.Ready
			}
			if ready {
				result.Summary.ReadyEndpoints++
				svcStats[key].Ready++
			} else {
				result.Summary.NotReadyEPs++
			}
		}
	}

	for key, stat := range svcStats {
		result.Services = append(result.Services, *stat)
		if stat.Ready > 0 {
			result.Summary.ServicesCovered++
		} else {
			result.Summary.ServicesNoEP++
			parts := splitKey(key)
			result.Issues = append(result.Issues, EPSliceIssue1939{
				ServiceName: stat.ServiceName, Namespace: parts,
				IssueType: "no-ready-endpoints", Severity: "high",
				Detail: "No ready endpoints in any slice",
			})
			score -= 5
		}
	}

	sort.Slice(result.Services, func(i, j int) bool {
		return result.Services[i].Endpoints > result.Services[j].Endpoints
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.ServicesNoEP > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services with no ready endpoints — investigate pod health", result.Summary.ServicesNoEP))
	}
	if result.Summary.NotReadyEPs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d not-ready endpoints — check readiness probes", result.Summary.NotReadyEPs))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func splitKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}
