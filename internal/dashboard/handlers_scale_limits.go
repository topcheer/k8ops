package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScLimResult is the cluster scalability limits analysis.
type ScLimResult struct {
	ScannedAt       time.Time    `json:"scannedAt"`
	Summary         ScLimSummary `json:"summary"`
	Limits          []ScLimEntry `json:"limits"`
	Recommendations []string     `json:"recommendations"`
}

// ScLimSummary aggregates scale stats.
type ScLimSummary struct {
	NodeCount      int `json:"nodeCount"`
	PodCount       int `json:"podCount"`
	ServiceCount   int `json:"serviceCount"`
	NamespaceCount int `json:"namespaceCount"`
	ConfigMapCount int `json:"configMapCount"`
	SecretCount    int `json:"secretCount"`
	MaxPodsPerNode int `json:"maxPodsPerNode"`
	TotalCapacity  int `json:"totalCapacity"`  // nodes * maxPodsPerNode
	UtilizationPct int `json:"utilizationPct"` // pods / totalCapacity * 100
	ScaleScore     int `json:"scaleScore"`     // 0-100
}

// ScLimEntry describes one Kubernetes scalability limit.
type ScLimEntry struct {
	Name        string  `json:"name"`
	Current     int     `json:"current"`
	Maximum     int     `json:"maximum"`
	Percent     float64 `json:"percent"` // current/maximum * 100
	Status      string  `json:"status"`  // safe, warning, critical
	Description string  `json:"description"`
}

// handleScaleLimits checks cluster proximity to Kubernetes scalability limits.
// GET /api/scalability/scale-limits
func (s *Server) handleScaleLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	result := ScLimResult{ScannedAt: time.Now()}

	// Determine max pods per node (default 110 for kubelet, 256 for some distros)
	maxPodsPerNode := 110
	for _, node := range nodes.Items {
		if val, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			podLimit := int(val.Value())
			if podLimit > maxPodsPerNode {
				maxPodsPerNode = podLimit
			}
		}
	}

	nodeCount := len(nodes.Items)
	podCount := len(pods.Items)
	serviceCount := len(services.Items)
	nsCount := len(nss.Items)
	cmCount := 0
	secretCount := 0
	if cms != nil {
		cmCount = len(cms.Items)
	}
	if secrets != nil {
		secretCount = len(secrets.Items)
	}

	totalCapacity := nodeCount * maxPodsPerNode

	result.Summary = ScLimSummary{
		NodeCount:      nodeCount,
		PodCount:       podCount,
		ServiceCount:   serviceCount,
		NamespaceCount: nsCount,
		ConfigMapCount: cmCount,
		SecretCount:    secretCount,
		MaxPodsPerNode: maxPodsPerNode,
		TotalCapacity:  totalCapacity,
	}

	if totalCapacity > 0 {
		result.Summary.UtilizationPct = podCount * 100 / totalCapacity
	}

	// Kubernetes scalability thresholds (official limits)
	// https://kubernetes.io/docs/setup/best-practices/cluster-large/
	result.Limits = append(result.Limits, sclimMakeLimit("Nodes", nodeCount, 5000, "Maximum recommended nodes per cluster"))
	result.Limits = append(result.Limits, sclimMakeLimit("Pods (total)", podCount, 150000, "Maximum recommended total pods"))
	result.Limits = append(result.Limits, sclimMakeLimit("Pods per node", sclimMaxPodsOnNode(nodes.Items, pods.Items), maxPodsPerNode, "Maximum pods schedulable per node"))
	result.Limits = append(result.Limits, sclimMakeLimit("Services", serviceCount, 5000, "Maximum recommended services (iptables mode)"))
	result.Limits = append(result.Limits, sclimMakeLimit("Namespaces", nsCount, 10000, "Maximum recommended namespaces"))
	result.Limits = append(result.Limits, sclimMakeLimit("ConfigMaps", cmCount, 0, "No hard limit, but high count increases API server load"))
	result.Limits = append(result.Limits, sclimMakeLimit("Secrets", secretCount, 0, "No hard limit, but high count increases etcd size"))

	// Pod capacity utilization
	result.Limits = append(result.Limits, ScLimEntry{
		Name:        "Pod Capacity Utilization",
		Current:     podCount,
		Maximum:     totalCapacity,
		Percent:     float64(result.Summary.UtilizationPct),
		Status:      sclimPctStatus(float64(result.Summary.UtilizationPct)),
		Description: fmt.Sprintf("%d pods / %d capacity (nodes=%d * maxPods=%d)", podCount, totalCapacity, nodeCount, maxPodsPerNode),
	})

	// Sort by percent descending
	sort.Slice(result.Limits, func(i, j int) bool {
		return result.Limits[i].Percent > result.Limits[j].Percent
	})

	result.Summary.ScaleScore = sclimScore(result.Limits)
	result.Recommendations = sclimGenRecs(result.Summary, result.Limits)

	writeJSON(w, result)
}

// sclimMaxPodsOnNode finds the node with the most pods.
func sclimMaxPodsOnNode(nodes []corev1.Node, pods []corev1.Pod) int {
	podByNode := make(map[string]int)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			podByNode[pod.Spec.NodeName]++
		}
	}
	maxCount := 0
	for _, count := range podByNode {
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}

// sclimMakeLimit creates a limit entry with status.
func sclimMakeLimit(name string, current, max int, desc string) ScLimEntry {
	l := ScLimEntry{
		Name:        name,
		Current:     current,
		Maximum:     max,
		Description: desc,
	}
	if max > 0 {
		l.Percent = float64(current) * 100 / float64(max)
		l.Status = sclimPctStatus(l.Percent)
	} else {
		// No hard limit — show info
		l.Percent = 0
		l.Status = "info"
	}
	return l
}

// sclimPctStatus determines status from percentage.
func sclimPctStatus(pct float64) string {
	if pct >= 80 {
		return "critical"
	}
	if pct >= 60 {
		return "warning"
	}
	return "safe"
}

// sclimScore computes scale score 0-100.
func sclimScore(limits []ScLimEntry) int {
	score := 100
	for _, l := range limits {
		switch l.Status {
		case "critical":
			score -= 20
		case "warning":
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

// sclimGenRecs produces actionable advice.
func sclimGenRecs(s ScLimSummary, limits []ScLimEntry) []string {
	var recs []string

	for _, l := range limits {
		if l.Status == "critical" {
			recs = append(recs, fmt.Sprintf("%s at %.0f%% of limit (%d/%d) — approaching Kubernetes scalability ceiling, plan cluster expansion", l.Name, l.Percent, l.Current, l.Maximum))
		} else if l.Status == "warning" {
			recs = append(recs, fmt.Sprintf("%s at %.0f%% of limit (%d/%d) — monitor growth and prepare expansion plan", l.Name, l.Percent, l.Current, l.Maximum))
		}
	}

	if s.UtilizationPct >= 70 {
		recs = append(recs, fmt.Sprintf("Pod capacity utilization is %d%% (%d/%d) — consider adding nodes to maintain scheduling headroom", s.UtilizationPct, s.PodCount, s.TotalCapacity))
	}
	if s.ConfigMapCount > 1000 {
		recs = append(recs, fmt.Sprintf("%d ConfigMaps — high object count increases API server list latency", s.ConfigMapCount))
	}
	if s.SecretCount > 1000 {
		recs = append(recs, fmt.Sprintf("%d Secrets — high object count increases etcd database size, consider external secret management", s.SecretCount))
	}
	if s.ScaleScore < 70 {
		recs = append(recs, fmt.Sprintf("Scale readiness score is %d/100 — cluster is approaching scalability limits", s.ScaleScore))
	}
	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("Cluster is well within scalability limits (%d nodes, %d pods, %d%% utilization) — good growth headroom", s.NodeCount, s.PodCount, s.UtilizationPct))
	}

	return recs
}
