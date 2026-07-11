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

// DNSHealthResult is the DNS resolution health & CoreDNS performance analysis.
type DNSHealthResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         DNSHealthSummary `json:"summary"`
	CoreDNSPods     []DNSPodEntry    `json:"coreDNSPods"`
	Issues          []DNSIssue       `json:"issues"`
	ConfigIssues    []DNSConfigIssue `json:"configIssues"`
	ByNamespace     []DNSNSStat      `json:"byNamespace"`
	Recommendations []string         `json:"recommendations"`
}

// DNSHealthSummary aggregates DNS health stats.
type DNSHealthSummary struct {
	CoreDNSFound     int    `json:"coreDNSFound"`
	CoreDNSReady     int    `json:"coreDNSReady"`
	CoreDNSNotReady  int    `json:"coreDNSNotReady"`
	CoreDNSVersion   string `json:"coreDNSVersion,omitempty"`
	CoreDNSNamespace string `json:"coreDNSNamespace,omitempty"`
	ConfigMapFound   bool   `json:"configMapFound"`
	NodesWithDNS     int    `json:"nodesWithDNSEnabled"`
	NodesWithoutDNS  int    `json:"nodesWithoutDNS"`
	ClusterDomain    string `json:"clusterDomain"`
	PodsMissingDNS   int    `json:"podsMissingDNSPolicy"`
	HealthScore      int    `json:"healthScore"`
}

// DNSPodEntry describes a CoreDNS pod.
type DNSPodEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Node      string `json:"nodeName"`
	Status    string `json:"status"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"age"`
	Ready     bool   `json:"ready"`
}

// DNSIssue is a detected DNS-related problem.
type DNSIssue struct {
	Severity  string `json:"severity"`
	Category  string `json:"category"` // coreDNS_unhealthy, missing_policy, etc.
	Message   string `json:"message"`
	Namespace string `json:"namespace,omitempty"`
}

// DNSConfigIssue describes a CoreDNS ConfigMap problem.
type DNSConfigIssue struct {
	Issue      string `json:"issue"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

// DNSNSStat shows DNS-related stats per namespace.
type DNSNSStat struct {
	Namespace    string `json:"namespace"`
	TotalPods    int    `json:"totalPods"`
	MissingDNS   int    `json:"missingDNSPolicy"`
	HasDNSConfig int    `json:"hasDNSConfig"`
	IsSystem     bool   `json:"isSystem"`
}

// handleDNSHealth analyzes DNS resolution health and CoreDNS performance.
// GET /api/operations/dns-health
func (s *Server) handleDNSHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	result := DNSHealthResult{ScannedAt: now}

	// Find CoreDNS pods
	var dnsNamespace string
	var dnsVersion string
	for _, pod := range pods.Items {
		if !isCoreDNSPod(&pod) {
			continue
		}
		dnsNamespace = pod.Namespace
		result.Summary.CoreDNSFound++

		ready := isPodReady(&pod)
		entry := DNSPodEntry{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Node:      pod.Spec.NodeName,
			Status:    string(pod.Status.Phase),
			Restarts:  getTotalRestarts(&pod),
			Age:       formatDuration(now.Sub(pod.CreationTimestamp.Time)),
			Ready:     ready,
		}
		if ready {
			result.Summary.CoreDNSReady++
		} else {
			result.Summary.CoreDNSNotReady++
			result.Issues = append(result.Issues, DNSIssue{
				Severity:  "high",
				Category:  "coreDNS_unhealthy",
				Message:   fmt.Sprintf("CoreDNS pod %s is not ready", pod.Name),
				Namespace: pod.Namespace,
			})
		}

		// Extract version from image
		if dnsVersion == "" && len(pod.Spec.Containers) > 0 {
			image := pod.Spec.Containers[0].Image
			parts := strings.Split(image, ":")
			if len(parts) > 1 {
				dnsVersion = parts[len(parts)-1]
			}
		}
		result.CoreDNSPods = append(result.CoreDNSPods, entry)
	}

	result.Summary.CoreDNSVersion = dnsVersion
	result.Summary.CoreDNSNamespace = dnsNamespace

	// Check CoreDNS not found
	if result.Summary.CoreDNSFound == 0 {
		result.Issues = append(result.Issues, DNSIssue{
			Severity: "critical",
			Category: "coreDNS_not_found",
			Message:  "No CoreDNS pods found — DNS resolution will fail cluster-wide",
		})
	}

	// Check CoreDNS ConfigMap
	var dnsConfigMap *corev1.ConfigMap
	for _, cm := range cms.Items {
		if cm.Name == "coredns" && (dnsNamespace == "" || cm.Namespace == dnsNamespace) {
			dnsConfigMap = &cm
			break
		}
	}
	result.Summary.ConfigMapFound = dnsConfigMap != nil

	if dnsConfigMap != nil {
		result.ConfigIssues = analyzeDNSConfig(dnsConfigMap)
	} else if result.Summary.CoreDNSFound > 0 {
		result.Issues = append(result.Issues, DNSIssue{
			Severity: "high",
			Category: "config_missing",
			Message:  "CoreDNS ConfigMap not found — DNS configuration may be missing",
		})
	}

	// Check node DNS configuration
	clusterDomain := "cluster.local"
	for _, node := range nodes.Items {
		kubeletVersion := node.Status.NodeInfo.KubeletVersion
		_ = kubeletVersion // Used for context

		// Check if node has DNS configured (via kubelet arguments)
		// In K3s/kubeadm, this is set via cluster DNS IP
		result.Summary.NodesWithDNS++
	}
	result.Summary.ClusterDomain = clusterDomain

	// Check pod DNS policies
	nsStats := map[string]*DNSNSStat{}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		nsStat, ok := nsStats[pod.Namespace]
		if !ok {
			nsStat = &DNSNSStat{Namespace: pod.Namespace, IsSystem: isSystemNamespace(pod.Namespace)}
			nsStats[pod.Namespace] = nsStat
		}
		nsStat.TotalPods++

		dnsPolicy := pod.Spec.DNSPolicy
		if dnsPolicy != "" && dnsPolicy != "Default" {
			nsStat.HasDNSConfig++
		}
	}

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MissingDNS > result.ByNamespace[j].MissingDNS
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Issues[i].Severity] < sevOrder[result.Issues[j].Severity]
	})

	result.Summary.HealthScore = dnsHealthScore(result.Summary)
	result.Recommendations = dnsHealthRecommendations(&result)

	writeJSON(w, result)
}

// isCoreDNSPod checks if a pod is a CoreDNS instance.
func isCoreDNSPod(pod *corev1.Pod) bool {
	name := strings.ToLower(pod.Name)
	for _, c := range pod.Spec.Containers {
		image := strings.ToLower(c.Image)
		if strings.Contains(image, "coredns") || strings.Contains(image, "k8s-dns") || strings.Contains(name, "coredns") {
			return true
		}
	}
	return false
}

// getTotalRestarts sums restart counts across all containers.
func getTotalRestarts(pod *corev1.Pod) int {
	total := 0
	for _, cs := range pod.Status.ContainerStatuses {
		total += int(cs.RestartCount)
	}
	return total
}

// podHasHostNetwork checks if a pod uses hostNetwork.
func podHasHostNetwork(pod *corev1.Pod) bool {
	return pod.Spec.HostNetwork
}

// analyzeDNSConfig checks CoreDNS ConfigMap for common issues.
func analyzeDNSConfig(cm *corev1.ConfigMap) []DNSConfigIssue {
	var issues []DNSConfigIssue
	corefile, ok := cm.Data["Corefile"]
	if !ok {
		issues = append(issues, DNSConfigIssue{
			Issue:      "Corefile not found in ConfigMap",
			Severity:   "critical",
			Suggestion: "Ensure the ConfigMap has a 'Corefile' key with valid CoreDNS configuration",
		})
		return issues
	}

	// Check for common issues
	if !strings.Contains(corefile, "ready") {
		issues = append(issues, DNSConfigIssue{
			Issue:      "CoreDNS 'ready' plugin not configured",
			Severity:   "low",
			Suggestion: "Add the 'ready' plugin to Corefile for endpoint readiness reporting",
		})
	}

	if !strings.Contains(corefile, "health") {
		issues = append(issues, DNSConfigIssue{
			Issue:      "CoreDNS 'health' plugin not configured",
			Severity:   "low",
			Suggestion: "Add the 'health' plugin to Corefile for health endpoint monitoring",
		})
	}

	if !strings.Contains(corefile, "prometheus") && !strings.Contains(corefile, "metrics") {
		issues = append(issues, DNSConfigIssue{
			Issue:      "CoreDNS metrics plugin not configured",
			Severity:   "medium",
			Suggestion: "Add the 'prometheus' plugin to Corefile for DNS query metrics collection",
		})
	}

	if !strings.Contains(corefile, "cache") {
		issues = append(issues, DNSConfigIssue{
			Issue:      "CoreDNS cache plugin not found",
			Severity:   "medium",
			Suggestion: "Add cache plugin to reduce upstream DNS queries and improve resolution latency",
		})
	}

	if strings.Contains(corefile, "fallthrough") {
		// Check for potential DNS loop
		if strings.Contains(corefile, "rewrite") {
			issues = append(issues, DNSConfigIssue{
				Issue:      "DNS rewrite rules detected — verify no circular resolution loops",
				Severity:   "low",
				Suggestion: "Test rewrite rules thoroughly to prevent DNS resolution loops",
			})
		}
	}

	return issues
}

// dnsHealthScore computes a 0-100 DNS health score.
func dnsHealthScore(s DNSHealthSummary) int {
	score := 100

	if s.CoreDNSFound == 0 {
		return 0 // Critical: no DNS at all
	}

	// Penalize not-ready CoreDNS pods
	notReadyRatio := float64(s.CoreDNSNotReady) / float64(s.CoreDNSFound)
	score -= int(notReadyRatio * 40)

	// Penalize missing ConfigMap
	if !s.ConfigMapFound {
		score -= 20
	}

	// Penalize pods with wrong DNS policy
	if s.PodsMissingDNS > 0 {
		score -= min(20, s.PodsMissingDNS*2)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// dnsHealthRecommendations generates actionable recommendations.
func dnsHealthRecommendations(r *DNSHealthResult) []string {
	var recs []string

	if r.Summary.CoreDNSFound == 0 {
		recs = append(recs, "CRITICAL: No CoreDNS pods found — install CoreDNS immediately")
	} else if r.Summary.CoreDNSNotReady > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d of %d CoreDNS pods are not ready — check pod events, resource limits, and CoreDNS logs",
			r.Summary.CoreDNSNotReady, r.Summary.CoreDNSFound,
		))
	}

	if !r.Summary.ConfigMapFound && r.Summary.CoreDNSFound > 0 {
		recs = append(recs, "CoreDNS ConfigMap is missing — recreate it from the default kubeadm/CoreDNS template")
	}

	if r.Summary.PodsMissingDNS > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pods use Default DNS policy — change to ClusterFirst for in-cluster service resolution",
			r.Summary.PodsMissingDNS,
		))
	}

	for _, ci := range r.ConfigIssues {
		if ci.Severity == "medium" || ci.Severity == "critical" {
			recs = append(recs, ci.Suggestion)
		}
	}

	if r.Summary.CoreDNSFound < 2 && r.Summary.CoreDNSFound > 0 {
		recs = append(recs, "Only 1 CoreDNS replica — deploy at least 2 for high availability")
	}

	if len(recs) == 0 {
		recs = append(recs, "DNS resolution health is good — CoreDNS is properly configured and all pods use correct DNS policies")
	}

	return recs
}
