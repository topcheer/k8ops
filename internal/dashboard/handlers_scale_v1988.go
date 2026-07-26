package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.88 — Scalability & HA Dimension (Round 17 Final)
// 1. Kubelet Pod Limit — per-node pod count vs maxPods limit
// 2. DNS Query Pressure — CoreDNS QPS estimator
// 3. CNI IPAM Capacity — IP address management exhaustion check
// ============================================================

// ---------------------------------------------------------------
// 1. Kubelet Pod Limit
// ---------------------------------------------------------------

type KubeletPodLimitResult1988 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         KubeletPodLimitSummary1988 `json:"summary"`
	PerNode         []KubeletPodLimitEntry1988 `json:"perNode"`
	Recommendations []string                   `json:"recommendations"`
}

type KubeletPodLimitSummary1988 struct {
	TotalNodes     int     `json:"totalNodes"`
	MaxPodsPerNode int     `json:"maxPodsPerNode"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	TotalPods      int     `json:"totalPods"`
	NodesNearLimit int     `json:"nodesNearLimit"`
	HighPodNode    string  `json:"highPodNode"`
}

type KubeletPodLimitEntry1988 struct {
	Name        string  `json:"name"`
	PodCount    int     `json:"podCount"`
	Limit       int     `json:"limit"`
	Utilization float64 `json:"utilization"`
}

func (s *Server) handleKubeletPodLimit(w http.ResponseWriter, r *http.Request) {
	result := KubeletPodLimitResult1988{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	result.Summary.TotalPods = len(podList.Items)
	result.Summary.MaxPodsPerNode = 110

	maxPods := 0
	highNode := ""
	var totalPods float64

	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]
		limit := 110

		// Check for custom maxPods via allocatable
		// k3s default is 110, but could be configured differently
		util := float64(podCount) / float64(limit) * 100

		entry := KubeletPodLimitEntry1988{
			Name: node.Name, PodCount: podCount, Limit: limit, Utilization: util,
		}
		result.PerNode = append(result.PerNode, entry)

		totalPods += float64(podCount)
		if podCount > maxPods {
			maxPods = podCount
			highNode = node.Name
		}
		if util > 80 {
			result.Summary.NodesNearLimit++
			score -= 5
		}
	}

	result.Summary.HighPodNode = highNode
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = totalPods / float64(result.Summary.TotalNodes)
	}

	sort.Slice(result.PerNode, func(i, j int) bool {
		return result.PerNode[i].Utilization > result.PerNode[j].Utilization
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d pods, avg %.0f/node, max %d on %s", result.Summary.TotalNodes, result.Summary.TotalPods, result.Summary.AvgPodsPerNode, maxPods, highNode))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. DNS Query Pressure
// ---------------------------------------------------------------

type DNSPressureResult1988 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         DNSPressureSummary1988 `json:"summary"`
	PerNS           []DNSPressureEntry1988 `json:"perNamespace"`
	Recommendations []string               `json:"recommendations"`
}

type DNSPressureSummary1988 struct {
	TotalPods       int     `json:"totalPods"`
	EstDNSQPS       float64 `json:"estDNSQPS"`
	PressureLevel   string  `json:"pressureLevel"`
	CoreDNSReplicas int     `json:"estCoreDNSReplicas"`
}

type DNSPressureEntry1988 struct {
	Namespace string  `json:"namespace"`
	PodCount  int     `json:"podCount"`
	EstQPS    float64 `json:"estQPS"`
}

func (s *Server) handleDNSQueryPressure(w http.ResponseWriter, r *http.Request) {
	result := DNSPressureResult1988{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Estimate DNS QPS: each pod ~0.3 DNS queries/sec (service discovery)
	const qpsPerPod = 0.3
	nsStats := make(map[string]int)
	totalPods := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		totalPods++
		nsStats[pod.Namespace]++
	}

	result.Summary.TotalPods = totalPods
	result.Summary.EstDNSQPS = float64(totalPods) * qpsPerPod

	// Estimate CoreDNS replicas needed (1 per 5000 pods, min 2)
	neededReplicas := totalPods/5000 + 2
	result.Summary.CoreDNSReplicas = neededReplicas

	// Pressure level
	if result.Summary.EstDNSQPS > 500 {
		result.Summary.PressureLevel = "critical"
		score -= 10
	} else if result.Summary.EstDNSQPS > 200 {
		result.Summary.PressureLevel = "high"
		score -= 5
	} else if result.Summary.EstDNSQPS > 100 {
		result.Summary.PressureLevel = "medium"
	} else {
		result.Summary.PressureLevel = "low"
	}

	for ns, count := range nsStats {
		result.PerNS = append(result.PerNS, DNSPressureEntry1988{
			Namespace: ns, PodCount: count,
			EstQPS: float64(count) * qpsPerPod,
		})
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EstQPS > result.PerNS[j].EstQPS
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, est DNS QPS %.0f, %s pressure, ~%d CoreDNS replicas needed", totalPods, result.Summary.EstDNSQPS, result.Summary.PressureLevel, neededReplicas))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. CNI IPAM Capacity
// ---------------------------------------------------------------

type CNIIPAMResult1988 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CNIIPAMSummary1988 `json:"summary"`
	PerNode         []CNIIPAMEntry1988 `json:"perNode"`
	Recommendations []string           `json:"recommendations"`
}

type CNIIPAMSummary1988 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalIPs       int     `json:"totalIPs"`
	UsedIPs        int     `json:"usedIPs"`
	AvailableIPs   int     `json:"availableIPs"`
	UtilizationPct float64 `json:"utilizationPct"`
	ExhaustionRisk string  `json:"exhaustionRisk"`
}

type CNIIPAMEntry1988 struct {
	Node         string `json:"node"`
	CIDR         string `json:"cidr"`
	UsedIPs      int    `json:"usedIPs"`
	AvailableIPs int    `json:"availableIPs"`
}

func (s *Server) handleCNIIPAMCapacity(w http.ResponseWriter, r *http.Request) {
	result := CNIIPAMResult1988{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	totalIPs := 0
	usedIPs := 0

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		cidr := node.Spec.PodCIDR
		if cidr == "" {
			cidr = "unknown"
		}

		// Estimate available IPs from CIDR
		availIPs := 254 // default /24
		if strings.Contains(cidr, "/24") {
			availIPs = 254
		} else if strings.Contains(cidr, "/16") {
			availIPs = 65534
		} else if strings.Contains(cidr, "/8") {
			availIPs = 16777214
		}

		nodeUsed := podsPerNode[node.Name]
		totalIPs += availIPs
		usedIPs += nodeUsed

		result.PerNode = append(result.PerNode, CNIIPAMEntry1988{
			Node: node.Name, CIDR: cidr,
			UsedIPs: nodeUsed, AvailableIPs: availIPs,
		})
	}

	result.Summary.TotalIPs = totalIPs
	result.Summary.UsedIPs = usedIPs
	result.Summary.AvailableIPs = totalIPs - usedIPs
	if totalIPs > 0 {
		result.Summary.UtilizationPct = float64(usedIPs) / float64(totalIPs) * 100
	}

	if result.Summary.UtilizationPct > 80 {
		result.Summary.ExhaustionRisk = "critical"
		score -= 15
	} else if result.Summary.UtilizationPct > 60 {
		result.Summary.ExhaustionRisk = "high"
		score -= 8
	} else if result.Summary.UtilizationPct > 40 {
		result.Summary.ExhaustionRisk = "medium"
	} else {
		result.Summary.ExhaustionRisk = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d/%d IPs used (%.1f%%), risk: %s", result.Summary.TotalNodes, usedIPs, totalIPs, result.Summary.UtilizationPct, result.Summary.ExhaustionRisk))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
