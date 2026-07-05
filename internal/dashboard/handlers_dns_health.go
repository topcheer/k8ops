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

// DNSHealthResult is the DNS resolution health analysis.
type DNSHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         DNSHealthSummary   `json:"summary"`
	CoreDNS         CoreDNSStatus      `json:"coreDNS"`
	DNSConfig       DNSConfigAnalysis  `json:"dnsConfig"`
	HeadlessSvcs    []HeadlessDNSEntry `json:"headlessServices"`
	ExternalDNS     []ExternalDNSEntry `json:"externalDNS,omitempty"`
	Issues          []DNSIssue         `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

// DNSHealthSummary aggregates cluster DNS health.
type DNSHealthSummary struct {
	CoreDNSPods        int    `json:"corednsPods"`
	CoreDNSReady       int    `json:"corednsReady"`
	CoreDNSVersion     string `json:"corednsVersion"`
	DNSDomain          string `json:"dnsDomain"`
	HeadlessSvcCount   int    `json:"headlessSvcCount"`
	HeadlessWithEP     int    `json:"headlessWithEndpoints"`
	HeadlessNoEP       int    `json:"headlessNoEndpoints"`
	ExternalDNSCount   int    `json:"externalDNSCount"`
	NodesWithCustomDNS int    `json:"nodesWithCustomDNS"`
	PodsWithCustomDNS  int    `json:"podsWithCustomDNS"`
	NDotsIssues        int    `json:"ndotsIssues"`
	HealthScore        int    `json:"healthScore"` // 0-100
}

// CoreDNSStatus describes CoreDNS deployment health.
type CoreDNSStatus struct {
	Pods          []CoreDNSPod `json:"pods"`
	ConfigMapName string       `json:"configMapName"`
	HasCorefile   bool         `json:"hasCorefile"`
	Forwarders    []string     `json:"forwarders"`
	Plugins       []string     `json:"plugins"`
	IsHealthy     bool         `json:"isHealthy"`
}

// CoreDNSPod describes one CoreDNS pod.
type CoreDNSPod struct {
	Name     string `json:"name"`
	Node     string `json:"node"`
	Status   string `json:"status"`
	Restarts int32  `json:"restarts"`
	Age      string `json:"age"`
	Version  string `json:"version"`
}

// DNSConfigAnalysis analyzes DNS configuration across the cluster.
type DNSConfigAnalysis struct {
	ClusterDomain    string `json:"clusterDomain"`
	NDotsDefault     int    `json:"ndotsDefault"`
	HasNodeLocalDNS  bool   `json:"hasNodeLocalDNS"`
	SearchDomains    int    `json:"searchDomains"`
	CustomDNSConfigs int    `json:"customDNSConfigs"`
	Warning          string `json:"warning,omitempty"`
}

// HeadlessDNSEntry describes a headless service's DNS resolution.
type HeadlessDNSEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	HasEndpoints   bool   `json:"hasEndpoints"`
	EndpointCount  int    `json:"endpointCount"`
	ReadyEndpoints int    `json:"readyEndpoints"`
	Issue          string `json:"issue,omitempty"`
}

// ExternalDNSEntry describes external-dns managed services.
type ExternalDNSEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Hostname  string `json:"hostname"`
}

// DNSIssue is a detected DNS problem.
type DNSIssue struct {
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource,omitempty"`
	Message   string `json:"message"`
}

// handleDNSHealth analyzes DNS resolution health across the cluster.
// GET /api/product/dns-health
func (s *Server) handleDNSHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	configMaps, _ := rc.clientset.CoreV1().ConfigMaps("kube-system").List(ctx, metav1.ListOptions{})

	result := DNSHealthResult{ScannedAt: time.Now()}

	// --- Analyze CoreDNS ---
	corednsPods := []CoreDNSPod{}
	for _, pod := range pods.Items {
		if !isCoreDNSPod(&pod) {
			continue
		}
		entry := CoreDNSPod{
			Name: pod.Name,
			Node: pod.Spec.NodeName,
			Age:  time.Since(pod.CreationTimestamp.Time).Round(time.Hour).String(),
		}

		if pod.Status.Phase == corev1.PodRunning {
			entry.Status = "running"
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "coredns" || strings.Contains(cs.Image, "coredns") {
					entry.Restarts = cs.RestartCount
					entry.Version = extractCoreDNSVersion(cs.Image)
					result.Summary.CoreDNSVersion = entry.Version
				}
			}
		} else {
			entry.Status = string(pod.Status.Phase)
		}

		corednsPods = append(corednsPods, entry)
		result.Summary.CoreDNSPods++

		isReady := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if isReady {
			result.Summary.CoreDNSReady++
		}
	}

	result.CoreDNS.Pods = corednsPods
	result.CoreDNS.IsHealthy = result.Summary.CoreDNSReady > 0 && result.Summary.CoreDNSReady == result.Summary.CoreDNSPods

	// --- Analyze CoreDNS ConfigMap ---
	for _, cm := range configMaps.Items {
		if cm.Name == "coredns" || cm.Name == "kube-dns" {
			result.CoreDNS.ConfigMapName = cm.Name
			if corefile, ok := cm.Data["Corefile"]; ok {
				result.CoreDNS.HasCorefile = true
				result.CoreDNS.Forwarders = extractDNSForwarders(corefile)
				result.CoreDNS.Plugins = extractCoreDNSPlugins(corefile)
			}
		}
	}

	// --- DNS domain from nodes ---
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			_ = addr
		}
	}
	// K8s default domain is cluster.local
	result.Summary.DNSDomain = "cluster.local"
	result.DNSConfig.ClusterDomain = "cluster.local"
	result.DNSConfig.NDotsDefault = 5
	result.DNSConfig.SearchDomains = 3 // default.svc, svc, bare

	// Check for NodeLocal DNS cache
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "node-local-dns") || strings.Contains(pod.Name, "nodelocaldns") {
			result.DNSConfig.HasNodeLocalDNS = true
			break
		}
	}

	// --- Analyze headless services ---
	epMap := make(map[string]int) // ns/name → ready endpoint count
	epTotalMap := make(map[string]int)
	for _, ep := range endpoints.Items {
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		ready := 0
		total := 0
		for _, sub := range ep.Subsets {
			for _, addr := range sub.Addresses {
				total++
				ready++
				_ = addr
			}
			for _, addr := range sub.NotReadyAddresses {
				total++
				_ = addr
			}
		}
		epMap[key] = ready
		epTotalMap[key] = total
	}

	for _, svc := range services.Items {
		if svc.Spec.ClusterIP != "None" {
			continue
		}
		// Check for external-dns annotation
		if hostname := svc.Annotations["external-dns.alpha.kubernetes.io/hostname"]; hostname != "" {
			result.Summary.ExternalDNSCount++
			result.ExternalDNS = append(result.ExternalDNS, ExternalDNSEntry{
				Name: svc.Name, Namespace: svc.Namespace, Hostname: hostname,
			})
		}

		entry := HeadlessDNSEntry{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		}
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		entry.HasEndpoints = epTotalMap[key] > 0
		entry.EndpointCount = epTotalMap[key]
		entry.ReadyEndpoints = epMap[key]

		result.Summary.HeadlessSvcCount++
		if entry.HasEndpoints {
			result.Summary.HeadlessWithEP++
		} else {
			result.Summary.HeadlessNoEP++
			entry.Issue = "No endpoints — DNS will return NXDOMAIN"
			result.Issues = append(result.Issues, DNSIssue{
				Severity:  "warning",
				Type:      "headless-no-endpoints",
				Namespace: svc.Namespace,
				Resource:  svc.Name,
				Message:   fmt.Sprintf("Headless service %s/%s has no endpoints — DNS queries will return NXDOMAIN", svc.Namespace, svc.Name),
			})
		}

		result.HeadlessSvcs = append(result.HeadlessSvcs, entry)
	}

	// --- Check pods with custom dnsConfig (ndots override) ---
	for _, pod := range pods.Items {
		if pod.Spec.DNSConfig != nil {
			result.Summary.PodsWithCustomDNS++
			if pod.Spec.DNSConfig.Options != nil {
				for _, opt := range pod.Spec.DNSConfig.Options {
					if opt.Name == "ndots" && opt.Value != nil {
						var ndots int
						fmt.Sscanf(*opt.Value, "%d", &ndots)
						if ndots > 5 {
							result.Summary.NDotsIssues++
							result.Issues = append(result.Issues, DNSIssue{
								Severity:  "info",
								Type:      "high-ndots",
								Namespace: pod.Namespace,
								Resource:  pod.Name,
								Message:   fmt.Sprintf("Pod %s/%s has ndots=%d (>5 may cause excessive DNS queries)", pod.Namespace, pod.Name, ndots),
							})
						}
					}
				}
			}
		}
	}

	// --- Check CoreDNS health ---
	if result.Summary.CoreDNSPods == 0 {
		result.Issues = append(result.Issues, DNSIssue{
			Severity: "critical",
			Type:     "no-coredns",
			Message:  "No CoreDNS pods found — DNS resolution will fail cluster-wide",
		})
	} else if result.Summary.CoreDNSReady < result.Summary.CoreDNSPods {
		result.Issues = append(result.Issues, DNSIssue{
			Severity: "critical",
			Type:     "coredns-not-ready",
			Message:  fmt.Sprintf("%d/%d CoreDNS pods are not ready", result.Summary.CoreDNSPods-result.Summary.CoreDNSReady, result.Summary.CoreDNSPods),
		})
	}

	// Check CoreDNS restarts
	for _, p := range corednsPods {
		if p.Restarts >= 5 {
			result.Issues = append(result.Issues, DNSIssue{
				Severity: "warning",
				Type:     "coredns-restarts",
				Resource: p.Name,
				Message:  fmt.Sprintf("CoreDNS pod %s has %d restarts — check Corefile and upstream DNS", p.Name, p.Restarts),
			})
		}
	}

	// Check for missing Corefile
	if !result.CoreDNS.HasCorefile && result.Summary.CoreDNSPods > 0 {
		result.Issues = append(result.Issues, DNSIssue{
			Severity: "critical",
			Type:     "no-corefile",
			Message:  "CoreDNS ConfigMap has no Corefile — DNS will not start correctly",
		})
	}

	// --- ndots warning ---
	if !result.DNSConfig.HasNodeLocalDNS {
		result.DNSConfig.Warning = "NodeLocal DNS cache not detected — consider installing for high-DNS-traffic clusters to reduce CoreDNS load"
	}

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return dnsIssueRank(result.Issues[i].Severity) < dnsIssueRank(result.Issues[j].Severity)
	})

	// Sort headless services (problematic first)
	sort.Slice(result.HeadlessSvcs, func(i, j int) bool {
		if result.HeadlessSvcs[i].HasEndpoints != result.HeadlessSvcs[j].HasEndpoints {
			return !result.HeadlessSvcs[i].HasEndpoints
		}
		return result.HeadlessSvcs[i].Namespace < result.HeadlessSvcs[j].Namespace
	})

	result.Summary.HealthScore = calculateDNSHealthScore(result.Summary, result.CoreDNS.IsHealthy)
	result.Recommendations = generateDNSRecommendations(result.Summary, result.CoreDNS, result.DNSConfig)

	writeJSON(w, result)
}

// isCoreDNSPod checks if a pod is a CoreDNS pod.
func isCoreDNSPod(pod *corev1.Pod) bool {
	if pod.Namespace != "kube-system" {
		return false
	}
	for _, c := range pod.Spec.Containers {
		if strings.Contains(c.Image, "coredns") || strings.Contains(c.Image, "k8s-dns") {
			return true
		}
	}
	return false
}

// extractCoreDNSVersion gets version from image tag.
func extractCoreDNSVersion(image string) string {
	// Handle sha256 digests: image:v1.2.3@sha256:abc
	if idx := strings.LastIndex(image, "@"); idx > 0 {
		image = image[:idx]
	}
	parts := strings.Split(image, ":")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractDNSForwarders parses forwarders from Corefile.
func extractDNSForwarders(corefile string) []string {
	var forwarders []string
	lines := strings.Split(corefile, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "forward") {
			fields := strings.Fields(line)
			for _, f := range fields[1:] {
				// Skip the zone (e.g. ".") and only capture actual forwarder addresses
				if f == "." || f == "::" {
					continue
				}
				if strings.Contains(f, ".") || strings.Contains(f, ":") {
					forwarders = append(forwarders, f)
				}
			}
		}
	}
	return forwarders
}

// extractCoreDNSPlugins parses plugins from Corefile.
func extractCoreDNSPlugins(corefile string) []string {
	var plugins []string
	lines := strings.Split(corefile, "\n")
	seen := make(map[string]bool)
	// Known CoreDNS plugin names for validation
	knownPlugins := map[string]bool{
		"errors": true, "health": true, "ready": true, "kubernetes": true,
		"forward": true, "cache": true, "loop": true, "reload": true,
		"loadbalance": true, "prometheus": true, "pprof": true,
		"template": true, "hosts": true, "file": true, "auto": true,
		"etcd": true, "rewrite": true, "log": true, "trace": true,
		"metadata": true, "nsid": true, "root": true, "whoami": true,
		"alternate": true, "any": true, "autopath": true,
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") || strings.HasPrefix(line, ":") {
			continue
		}
		// Skip closing braces and inline directives
		if line == "}" || strings.HasPrefix(line, "}") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			plugin := fields[0]
			// Only add known plugins
			if knownPlugins[plugin] && !seen[plugin] {
				plugins = append(plugins, plugin)
				seen[plugin] = true
			}
		}
	}
	return plugins
}

// calculateDNSHealthScore computes 0-100.
func calculateDNSHealthScore(s DNSHealthSummary, corednsHealthy bool) int {
	score := 100
	if !corednsHealthy {
		score -= 40
	}
	if s.CoreDNSPods == 0 {
		score = 0
	}
	score -= s.HeadlessNoEP * 3
	if s.NDotsIssues > 0 {
		score -= s.NDotsIssues * 2
	}
	if !s.HasNodeLocalDNSSafe() && s.CoreDNSPods <= 1 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	return score
}

// HasNodeLocalDNSSafe is a safe accessor.
func (s DNSHealthSummary) HasNodeLocalDNSSafe() bool {
	// NodeLocal DNS is an optimization, not a requirement
	return true // Don't penalize in scoring
}

// generateDNSRecommendations produces actionable advice.
func generateDNSRecommendations(s DNSHealthSummary, coredns CoreDNSStatus, dnsConfig DNSConfigAnalysis) []string {
	var recs []string

	if s.CoreDNSPods == 0 {
		recs = append(recs, "CRITICAL: No CoreDNS pods found — install CoreDNS immediately")
	} else if s.CoreDNSReady < s.CoreDNSPods {
		recs = append(recs, fmt.Sprintf("%d/%d CoreDNS pods are not ready — check pod logs and Corefile", s.CoreDNSReady, s.CoreDNSPods))
	}
	if s.CoreDNSPods > 0 && s.CoreDNSPods < 2 {
		recs = append(recs, "Only 1 CoreDNS pod — deploy at least 2 replicas for high availability")
	}
	if s.HeadlessNoEP > 0 {
		recs = append(recs, fmt.Sprintf("%d headless service(s) have no endpoints — DNS returns NXDOMAIN for these services", s.HeadlessNoEP))
	}
	if s.NDotsIssues > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have ndots > 5 — this causes excessive DNS queries for external domains", s.NDotsIssues))
	}
	if !dnsConfig.HasNodeLocalDNS && s.CoreDNSPods > 0 {
		recs = append(recs, "NodeLocal DNS cache not detected — consider installing for clusters with high pod density or DNS-heavy workloads")
	}
	if len(coredns.Forwarders) == 0 && coredns.HasCorefile {
		recs = append(recs, "No upstream forwarders found in CoreDNS Corefile — external DNS resolution may fail")
	}
	if len(coredns.Plugins) > 0 && !dnsPluginHasReady(coredns.Plugins) {
		recs = append(recs, "CoreDNS 'ready' plugin not found — readiness probe may not work correctly")
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("DNS health score is %d/100 — investigate DNS configuration issues", s.HealthScore))
	}

	return recs
}

func dnsPluginHasReady(plugins []string) bool {
	for _, p := range plugins {
		if p == "ready" {
			return true
		}
	}
	return false
}

func dnsIssueRank(s string) int {
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
