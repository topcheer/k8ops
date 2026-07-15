package dashboard

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodAnomalyResult is the pod performance anomaly & noisy neighbor detection analysis.
type PodAnomalyResult struct {
	ScannedAt         time.Time           `json:"scannedAt"`
	Summary           PodAnomalySummary   `json:"summary"`
	AnomalousPods     []PodAnomaly        `json:"anomalousPods"`
	NoisyNeighbors    []NoisyNeighbor     `json:"noisyNeighbors,omitempty"`
	NodeHotspots      []NodeHotspot       `json:"nodeHotspots,omitempty"`
	WorkloadPeerStats []WorkloadPeerStats `json:"workloadPeerStats,omitempty"`
	Recommendations   []string            `json:"recommendations"`
	HealthScore       int                 `json:"healthScore"`
}

// PodAnomalySummary aggregates anomaly detection statistics.
type PodAnomalySummary struct {
	TotalPods       int     `json:"totalPods"`
	AnalyzedPods    int     `json:"analyzedPods"`
	AnomalousPods   int     `json:"anomalousPods"`
	HighRestartPods int     `json:"highRestartPods"`
	HighCPUPods     int     `json:"highCPUPods"`
	HighMemoryPods  int     `json:"highMemoryPods"`
	NoisyNodes      int     `json:"noisyNodes"`
	NodesAnalyzed   int     `json:"nodesAnalyzed"`
	AvgRestarts     float64 `json:"avgRestarts"`
	AnomalyRate     float64 `json:"anomalyRate"` // percentage of anomalous pods
}

// PodAnomaly describes a single anomalous pod.
type PodAnomaly struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Workload       string `json:"workload"`
	NodeName       string `json:"nodeName"`
	RestartCount   int    `json:"restartCount"`
	AnomalyType    string `json:"anomalyType"` // high-restart, resource-hog, unstable, outlier
	Severity       string `json:"severity"`    // critical, warning, info
	Description    string `json:"description"`
	PeerComparison string `json:"peerComparison,omitempty"` // how this pod differs from peers
}

// NoisyNeighbor identifies a pod that may be interfering with others on the same node.
type NoisyNeighbor struct {
	PodName      string   `json:"podName"`
	Namespace    string   `json:"namespace"`
	NodeName     string   `json:"nodeName"`
	IssueType    string   `json:"issueType"` // high-cpu, high-memory, high-restart, privileged
	AffectedPods []string `json:"affectedPods"`
	Severity     string   `json:"severity"`
}

// NodeHotspot identifies a node with multiple anomalous pods.
type NodeHotspot struct {
	NodeName       string  `json:"nodeName"`
	AnomalousCount int     `json:"anomalousPods"`
	TotalPods      int     `json:"totalPods"`
	AnomalyRate    float64 `json:"anomalyRate"`
	AvgRestarts    float64 `json:"avgRestarts"`
	Zone           string  `json:"zone"`
}

// WorkloadPeerStats shows statistical comparison within a workload's replica set.
type WorkloadPeerStats struct {
	Workload    string  `json:"workload"`
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	AvgRestarts float64 `json:"avgRestarts"`
	MaxRestarts int     `json:"maxRestarts"`
	MinRestarts int     `json:"minRestarts"`
	Variance    float64 `json:"variance"` // restart variance among replicas
	HasOutlier  bool    `json:"hasOutlier"`
}

// handlePodAnomaly detects pod performance anomalies by comparing pods against
// their peers and identifying noisy neighbor interference patterns.
// GET /api/operations/pod-anomaly
func (s *Server) handlePodAnomaly(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := PodAnomalyResult{ScannedAt: time.Now()}

	// 1. Collect nodes
	nodeZoneMap := map[string]string{}
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			if zone, ok := node.Labels[corev1.LabelTopologyZone]; ok {
				nodeZoneMap[node.Name] = zone
			}
		}
	}

	// 2. Collect pods
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// 3. Group pods by workload for peer comparison
	type podInfo struct {
		name         string
		namespace    string
		nodeName     string
		restarts     int
		workload     string
		workloadKind string
		age          time.Duration
		status       corev1.PodPhase
		containers   int
		privileged   bool
	}

	workloadPods := map[string][]podInfo{} // "ns/workload" → pods
	nodePods := map[string][]podInfo{}     // nodeName → pods

	var allPods []podInfo
	totalRestarts := 0
	analyzedCount := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Calculate total restart count
		totalRestart := 0
		containerCount := 0
		hasPrivileged := false
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestart += int(cs.RestartCount)
			containerCount++
		}
		for _, c := range pod.Spec.Containers {
			containerCount++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				hasPrivileged = true
			}
		}

		// Extract workload name
		wlName, wlKind := extractWorkloadFromPod(pod)
		if wlName == "" {
			wlName = pod.Name
			wlKind = "Pod"
		}

		age := time.Since(pod.CreationTimestamp.Time)

		pi := podInfo{
			name:         pod.Name,
			namespace:    pod.Namespace,
			nodeName:     pod.Spec.NodeName,
			restarts:     totalRestart,
			workload:     wlName,
			workloadKind: wlKind,
			age:          age,
			status:       pod.Status.Phase,
			containers:   containerCount,
			privileged:   hasPrivileged,
		}

		allPods = append(allPods, pi)
		totalRestarts += totalRestart
		analyzedCount++

		// Group by workload
		wlKey := fmt.Sprintf("%s/%s", pod.Namespace, wlName)
		workloadPods[wlKey] = append(workloadPods[wlKey], pi)

		// Group by node
		if pod.Spec.NodeName != "" {
			nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pi)
		}
	}

	result.Summary.TotalPods = len(pods.Items)
	result.Summary.AnalyzedPods = analyzedCount
	result.Summary.NodesAnalyzed = len(nodePods)

	if analyzedCount > 0 {
		result.Summary.AvgRestarts = float64(totalRestarts) / float64(analyzedCount)
	}

	// 4. Detect anomalies: compare pods against workload peers
	var anomalies []PodAnomaly
	highRestartThreshold := 5

	// Workload peer analysis
	var peerStats []WorkloadPeerStats
	for wlKey, wlPods := range workloadPods {
		if len(wlPods) < 1 {
			continue
		}

		restarts := make([]int, len(wlPods))
		maxR, minR := 0, math.MaxInt32
		sumR := 0
		for i, p := range wlPods {
			restarts[i] = p.restarts
			sumR += p.restarts
			if p.restarts > maxR {
				maxR = p.restarts
			}
			if p.restarts < minR {
				minR = p.restarts
			}
		}
		if minR == math.MaxInt32 {
			minR = 0
		}
		avgR := float64(sumR) / float64(len(wlPods))

		// Calculate variance
		variance := 0.0
		for _, r := range restarts {
			diff := float64(r) - avgR
			variance += diff * diff
		}
		variance /= float64(len(wlPods))

		hasOutlier := maxR > int(avgR)+3 && maxR > highRestartThreshold

		// Split namespace and workload from key
		parts := strings.SplitN(wlKey, "/", 2)
		nsPart, wlPart := parts[0], parts[1]
		if len(parts) == 1 {
			nsPart = ""
			wlPart = wlKey
		}

		peerStats = append(peerStats, WorkloadPeerStats{
			Workload:    wlPart,
			Namespace:   nsPart,
			PodCount:    len(wlPods),
			AvgRestarts: avgR,
			MaxRestarts: maxR,
			MinRestarts: minR,
			Variance:    variance,
			HasOutlier:  hasOutlier,
		})

		// Flag outlier pods (significantly more restarts than peers)
		if len(wlPods) >= 2 && hasOutlier {
			for _, p := range wlPods {
				if p.restarts == maxR && p.restarts > highRestartThreshold {
					peerAvg := avgR - float64(p.restarts)/float64(len(wlPods))
					peerAvg = math.Max(peerAvg, 0)
					anomalies = append(anomalies, PodAnomaly{
						Name:           p.name,
						Namespace:      p.namespace,
						Workload:       p.workload,
						NodeName:       p.nodeName,
						RestartCount:   p.restarts,
						AnomalyType:    "outlier",
						Severity:       classifyRestartSeverity(p.restarts, p.age),
						Description:    fmt.Sprintf("Pod has %d restarts vs peer average of %.1f — possible node-specific issue", p.restarts, peerAvg),
						PeerComparison: fmt.Sprintf("%.1fx peer average", float64(p.restarts)/math.Max(peerAvg, 1)),
					})
				}
			}
		}

		// Flag all pods in high-restart workloads
		if avgR >= float64(highRestartThreshold) {
			for _, p := range wlPods {
				anomalies = append(anomalies, PodAnomaly{
					Name:         p.name,
					Namespace:    p.namespace,
					Workload:     p.workload,
					NodeName:     p.nodeName,
					RestartCount: p.restarts,
					AnomalyType:  "high-restart",
					Severity:     classifyRestartSeverity(p.restarts, p.age),
					Description:  fmt.Sprintf("High restart count (%d) — workload average is %.1f", p.restarts, avgR),
				})
			}
		}
	}

	// Deduplicate anomalies (a pod might be flagged twice)
	anomalies = dedupAnomalies(anomalies)
	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].RestartCount > anomalies[j].RestartCount
	})
	if len(anomalies) > 100 {
		anomalies = anomalies[:100]
	}
	result.AnomalousPods = anomalies
	result.Summary.AnomalousPods = len(anomalies)
	for _, a := range anomalies {
		if a.AnomalyType == "high-restart" {
			result.Summary.HighRestartPods++
		}
	}
	if analyzedCount > 0 {
		result.Summary.AnomalyRate = float64(len(anomalies)) / float64(analyzedCount) * 100
	}

	// 5. Sort peer stats by variance (most inconsistent first)
	sort.Slice(peerStats, func(i, j int) bool {
		return peerStats[i].Variance > peerStats[j].Variance
	})
	if len(peerStats) > 50 {
		peerStats = peerStats[:50]
	}
	result.WorkloadPeerStats = peerStats

	// 6. Detect noisy neighbors (pods with high restarts or privileged on crowded nodes)
	var noisyNeighbors []NoisyNeighbor
	for nodeName, nodePodsList := range nodePods {
		if len(nodePodsList) < 2 {
			continue
		}

		// Find pods with significantly higher restarts than node average
		nodeAvgRestart := 0.0
		for _, p := range nodePodsList {
			nodeAvgRestart += float64(p.restarts)
		}
		nodeAvgRestart /= float64(len(nodePodsList))

		for _, p := range nodePodsList {
			isNoisy := false
			issueType := ""

			if p.restarts > int(nodeAvgRestart)*3 && p.restarts > highRestartThreshold {
				isNoisy = true
				issueType = "high-restart"
			}
			if p.privileged {
				isNoisy = true
				issueType = "privileged"
			}

			if isNoisy {
				// Collect affected pods (others on same node)
				var affected []string
				for _, other := range nodePodsList {
					if other.name != p.name {
						affected = append(affected, other.name)
					}
				}
				if len(affected) > 10 {
					affected = affected[:10]
				}
				noisyNeighbors = append(noisyNeighbors, NoisyNeighbor{
					PodName:      p.name,
					Namespace:    p.namespace,
					NodeName:     nodeName,
					IssueType:    issueType,
					AffectedPods: affected,
					Severity:     classifyRestartSeverity(p.restarts, p.age),
				})
			}
		}
	}
	sort.Slice(noisyNeighbors, func(i, j int) bool {
		return noisyNeighbors[i].Severity > noisyNeighbors[j].Severity
	})
	if len(noisyNeighbors) > 50 {
		noisyNeighbors = noisyNeighbors[:50]
	}
	result.NoisyNeighbors = noisyNeighbors

	// 7. Identify node hotspots
	var hotspots []NodeHotspot
	for nodeName, nodePodsList := range nodePods {
		if len(nodePodsList) < 3 {
			continue
		}
		anomalousCount := 0
		sumRestarts := 0
		for _, p := range nodePodsList {
			sumRestarts += p.restarts
			if p.restarts > highRestartThreshold {
				anomalousCount++
			}
		}
		avgR := float64(sumRestarts) / float64(len(nodePodsList))
		anomalyRate := float64(anomalousCount) / float64(len(nodePodsList)) * 100

		// Only flag nodes with above-average anomalies
		if anomalousCount >= 2 || anomalyRate > 30 {
			hotspots = append(hotspots, NodeHotspot{
				NodeName:       nodeName,
				AnomalousCount: anomalousCount,
				TotalPods:      len(nodePodsList),
				AnomalyRate:    anomalyRate,
				AvgRestarts:    avgR,
				Zone:           nodeZoneMap[nodeName],
			})
		}
	}
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].AnomalousCount > hotspots[j].AnomalousCount
	})
	result.NodeHotspots = hotspots
	result.Summary.NoisyNodes = len(hotspots)

	// 8. Calculate health score
	score := 100
	if result.Summary.AnomalyRate > 20 {
		score -= 20
	} else if result.Summary.AnomalyRate > 10 {
		score -= 10
	} else if result.Summary.AnomalyRate > 5 {
		score -= 5
	}
	if len(hotspots) > 0 {
		score -= len(hotspots) * 5
	}
	if len(noisyNeighbors) > 5 {
		score -= 5
	}
	// High average restarts
	if result.Summary.AvgRestarts > 3 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 9. Recommendations
	result.Recommendations = generateAnomalyRecommendations(result)

	writeJSON(w, result)
}

// classifyRestartSeverity determines severity from restart count and pod age.
func classifyRestartSeverity(restarts int, age time.Duration) string {
	if restarts == 0 {
		return "info"
	}
	// Normalize by age: restarts per hour
	hours := age.Hours()
	if hours < 1 {
		hours = 1
	}
	rate := float64(restarts) / hours

	switch {
	case restarts >= 20 || rate > 10:
		return "critical"
	case restarts >= 10 || rate > 5:
		return "warning"
	case restarts >= 5 || rate > 3:
		return "warning"
	default:
		return "info"
	}
}

// dedupAnomalies removes duplicate pod entries, keeping the highest severity.
func dedupAnomalies(anomalies []PodAnomaly) []PodAnomaly {
	seen := map[string]int{} // pod name → index in result
	var result []PodAnomaly

	for _, a := range anomalies {
		key := fmt.Sprintf("%s/%s", a.Namespace, a.Name)
		if idx, exists := seen[key]; exists {
			// Keep the one with higher restart count
			if a.RestartCount > result[idx].RestartCount {
				result[idx] = a
			}
		} else {
			seen[key] = len(result)
			result = append(result, a)
		}
	}
	return result
}

// generateAnomalyRecommendations produces actionable recommendations.
func generateAnomalyRecommendations(result PodAnomalyResult) []string {
	var recs []string

	if result.Summary.AnomalousPods > 0 {
		recs = append(recs, fmt.Sprintf("%d anomalous pod(s) detected (%.1f%% of analyzed pods) — investigate root cause patterns",
			result.Summary.AnomalousPods, result.Summary.AnomalyRate))
	}

	if result.Summary.HighRestartPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with high restart counts — check application logs, OOM kills, and probe configurations",
			result.Summary.HighRestartPods))
	}

	if len(result.NoisyNeighbors) > 0 {
		recs = append(recs, fmt.Sprintf("%d noisy neighbor(s) detected — consider rescheduling or adding resource limits to prevent cross-workload interference",
			len(result.NoisyNeighbors)))
	}

	if len(result.NodeHotspots) > 0 {
		top := result.NodeHotspots[0]
		recs = append(recs, fmt.Sprintf("Node %q is a hotspot with %d anomalous pod(s) (%.0f%%) — check node health, hardware, and kubelet",
			top.NodeName, top.AnomalousCount, top.AnomalyRate))
	}

	// Peer variance
	highVarianceCount := 0
	for _, ps := range result.WorkloadPeerStats {
		if ps.HasOutlier {
			highVarianceCount++
		}
	}
	if highVarianceCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have inconsistent replica behavior (high restart variance) — may indicate node-specific issues or rolling update problems",
			highVarianceCount))
	}

	if result.HealthScore < 50 {
		recs = append(recs, fmt.Sprintf("Pod anomaly health score is %d/100 — cluster has significant stability issues", result.HealthScore))
	} else if result.HealthScore >= 90 && len(recs) == 0 {
		recs = append(recs, "No significant pod anomalies detected — cluster pods are stable")
	}

	return recs
}
