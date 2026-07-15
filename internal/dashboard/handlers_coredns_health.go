package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CoreDNSResult is the CoreDNS configuration & resolution health audit.
type CoreDNSResult struct {
	Timestamp       time.Time        `json:"timestamp"`
	Score           int              `json:"score"`
	Status          string           `json:"status"`
	Summary         CoreDNSSummary   `json:"summary"`
	DaemonSetHealth CoreDNSDSHealth  `json:"daemonSetHealth"`
	ConfigAnalysis  CoreDNSConfig    `json:"configAnalysis"`
	NodeLocalDNS    NodeLocalDNSInfo `json:"nodeLocalDNS"`
	UpstreamServers []string         `json:"upstreamServers"`
	StubDomains     []StubDomain     `json:"stubDomains"`
	Issues          []CoreDNSIssue   `json:"issues"`
	NodeCoverage    []CoreDNSNodeCov `json:"nodeCoverage"`
	Recommendations []string         `json:"recommendations"`
}

// CoreDNSSummary holds aggregate CoreDNS metrics.
type CoreDNSSummary struct {
	CoreDNSFound    bool   `json:"coreDNSFound"`
	DeploymentType  string `json:"deploymentType"` // Deployment or DaemonSet
	Image           string `json:"image"`
	DesiredReplicas int    `json:"desiredReplicas"`
	ReadyReplicas   int    `json:"readyReplicas"`
	TotalNodes      int    `json:"totalNodes"`
	NodesWithDNS    int    `json:"nodesWithDNS"`
	RestartCount    int32  `json:"restartCount"`
	ConfigIssues    int    `json:"configIssues"`
	HasNodeLocalDNS bool   `json:"hasNodeLocalDNS"`
}

// CoreDNSDSHealth describes the CoreDNS Deployment/DaemonSet state.
type CoreDNSDSHealth struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Desired   int    `json:"desired"`
	Ready     int    `json:"ready"`
	Updated   int    `json:"updated"`
	Available int    `json:"available"`
	Strategy  string `json:"strategy"`
}

// CoreDNSConfig holds CoreDNS Corefile configuration analysis.
type CoreDNSConfig struct {
	HasCorefile      bool     `json:"hasCorefile"`
	ErrorsPlugin     bool     `json:"errorsPlugin"`
	HealthPlugin     bool     `json:"healthPlugin"`
	ReadyPlugin      bool     `json:"readyPlugin"`
	PrometheusPlugin bool     `json:"prometheusPlugin"`
	ForwardPlugin    bool     `json:"forwardPlugin"`
	CachePlugin      bool     `json:"cachePlugin"`
	LoopPlugin       bool     `json:"loopPlugin"`
	ReloadPlugin     bool     `json:"reloadPlugin"`
	LogDeny          bool     `json:"logDeny"`
	SearchDomains    []string `json:"searchDomains"`
}

// NodeLocalDNSInfo describes NodeLocal DNS Cache deployment.
type NodeLocalDNSInfo struct {
	Deployed      bool   `json:"deployed"`
	DaemonSetName string `json:"daemonSetName"`
	DesiredNodes  int    `json:"desiredNodes"`
	ReadyNodes    int    `json:"readyNodes"`
	Image         string `json:"image"`
}

// StubDomain describes a custom DNS stub domain.
type StubDomain struct {
	Domain   string `json:"domain"`
	Upstream string `json:"upstream"`
}

// CoreDNSIssue identifies a CoreDNS configuration or health issue.
type CoreDNSIssue struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Node     string `json:"node"`
	Message  string `json:"message"`
}

// CoreDNSNodeCov describes CoreDNS coverage per node.
type CoreDNSNodeCov struct {
	Node     string `json:"node"`
	HasDNS   bool   `json:"hasDNS"`
	HasLocal bool   `json:"hasLocalDNS"`
}

func (s *Server) handleCoreDNSHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Get CoreDNS Deployment and DaemonSet
	deploys, err := rc.clientset.AppsV1().Deployments("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		deploys = &appsv1.DeploymentList{}
	}

	daemonsets, err := rc.clientset.AppsV1().DaemonSets("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		daemonsets = &appsv1.DaemonSetList{}
	}

	pods, err := rc.clientset.CoreV1().Pods("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	cms, err := rc.clientset.CoreV1().ConfigMaps("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		cms = &corev1.ConfigMapList{}
	}

	result := analyzeCoreDNSHealth(deploys.Items, daemonsets.Items, pods.Items, nodes.Items, cms.Items)
	writeJSON(w, result)
}

func analyzeCoreDNSHealth(deploys []appsv1.Deployment, dss []appsv1.DaemonSet, pods []corev1.Pod, nodes []corev1.Node, cms []corev1.ConfigMap) CoreDNSResult {
	now := time.Now()

	summary := CoreDNSSummary{}
	var dsHealth CoreDNSDSHealth
	var configAnalysis CoreDNSConfig
	var nodeLocalDNS NodeLocalDNSInfo
	var upstreamServers []string
	var stubDomains []StubDomain
	var issues []CoreDNSIssue
	var nodeCov []CoreDNSNodeCov

	// Find CoreDNS deployment
	var corednsDeploy *appsv1.Deployment
	for i := range deploys {
		if strings.Contains(strings.ToLower(deploys[i].Name), "coredns") {
			corednsDeploy = &deploys[i]
			break
		}
	}

	// Find CoreDNS DaemonSet (some clusters use DaemonSet)
	var corednsDS *appsv1.DaemonSet
	for i := range dss {
		if strings.Contains(strings.ToLower(dss[i].Name), "coredns") {
			corednsDS = &dss[i]
			break
		}
	}

	// Find NodeLocal DNS Cache
	for _, ds := range dss {
		if strings.Contains(strings.ToLower(ds.Name), "node-local") || strings.Contains(strings.ToLower(ds.Name), "nodelocaldns") {
			nodeLocalDNS.Deployed = true
			nodeLocalDNS.DaemonSetName = ds.Name
			nodeLocalDNS.DesiredNodes = int(ds.Status.DesiredNumberScheduled)
			nodeLocalDNS.ReadyNodes = int(ds.Status.NumberReady)
			if len(ds.Spec.Template.Spec.Containers) > 0 {
				nodeLocalDNS.Image = ds.Spec.Template.Spec.Containers[0].Image
			}
			break
		}
	}
	summary.HasNodeLocalDNS = nodeLocalDNS.Deployed

	if corednsDeploy != nil {
		summary.CoreDNSFound = true
		summary.DeploymentType = "Deployment"
		summary.DesiredReplicas = int(*corednsDeploy.Spec.Replicas)
		summary.ReadyReplicas = int(corednsDeploy.Status.ReadyReplicas)
		dsHealth = CoreDNSDSHealth{
			Name:      corednsDeploy.Name,
			Namespace: corednsDeploy.Namespace,
			Kind:      "Deployment",
			Desired:   summary.DesiredReplicas,
			Ready:     summary.ReadyReplicas,
			Updated:   int(corednsDeploy.Status.UpdatedReplicas),
			Available: int(corednsDeploy.Status.AvailableReplicas),
			Strategy:  string(corednsDeploy.Spec.Strategy.Type),
		}
		if len(corednsDeploy.Spec.Template.Spec.Containers) > 0 {
			summary.Image = corednsDeploy.Spec.Template.Spec.Containers[0].Image
		}
	} else if corednsDS != nil {
		summary.CoreDNSFound = true
		summary.DeploymentType = "DaemonSet"
		summary.DesiredReplicas = int(corednsDS.Status.DesiredNumberScheduled)
		summary.ReadyReplicas = int(corednsDS.Status.NumberReady)
		dsHealth = CoreDNSDSHealth{
			Name:      corednsDS.Name,
			Namespace: corednsDS.Namespace,
			Kind:      "DaemonSet",
			Desired:   summary.DesiredReplicas,
			Ready:     summary.ReadyReplicas,
			Updated:   int(corednsDS.Status.UpdatedNumberScheduled),
			Available: int(corednsDS.Status.NumberAvailable),
		}
		if len(corednsDS.Spec.Template.Spec.Containers) > 0 {
			summary.Image = corednsDS.Spec.Template.Spec.Containers[0].Image
		}
	}

	// Analyze CoreDNS pods
	corednsPodNodes := make(map[string]bool)
	for _, pod := range pods {
		if !strings.Contains(strings.ToLower(pod.Name), "coredns") {
			continue
		}
		corednsPodNodes[pod.Spec.NodeName] = true
		for _, cs := range pod.Status.ContainerStatuses {
			summary.RestartCount += cs.RestartCount
		}
		if pod.Status.Phase != corev1.PodRunning {
			issues = append(issues, CoreDNSIssue{
				Type:     "CoreDNSPodNotRunning",
				Severity: "high",
				Node:     pod.Spec.NodeName,
				Message:  fmt.Sprintf("CoreDNS pod %s is %s", pod.Name, pod.Status.Phase),
			})
		}
	}

	// Analyze CoreDNS ConfigMap (Corefile)
	for _, cm := range cms {
		if !strings.Contains(strings.ToLower(cm.Name), "coredns") {
			continue
		}
		corefile, ok := cm.Data["Corefile"]
		if !ok {
			continue
		}
		configAnalysis.HasCorefile = true

		// Check for essential plugins
		lines := strings.Split(corefile, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "errors") {
				configAnalysis.ErrorsPlugin = true
			}
			if strings.HasPrefix(lower, "health") {
				configAnalysis.HealthPlugin = true
			}
			if strings.HasPrefix(lower, "ready") {
				configAnalysis.ReadyPlugin = true
			}
			if strings.HasPrefix(lower, "prometheus") {
				configAnalysis.PrometheusPlugin = true
			}
			if strings.HasPrefix(lower, "forward") {
				configAnalysis.ForwardPlugin = true
				// Extract upstream servers
				parts := strings.Fields(trimmed)
				for _, p := range parts[1:] {
					if p != "." && !strings.HasPrefix(p, "{") && p != "max_concurrent" && p != "1000" {
						upstreamServers = append(upstreamServers, p)
					}
				}
			}
			if strings.HasPrefix(lower, "cache") {
				configAnalysis.CachePlugin = true
			}
			if strings.HasPrefix(lower, "loop") {
				configAnalysis.LoopPlugin = true
			}
			if strings.HasPrefix(lower, "reload") {
				configAnalysis.ReloadPlugin = true
			}
			if strings.HasPrefix(lower, "log") {
				// Check if it has deny/stop options
				if strings.Contains(lower, "deny") {
					configAnalysis.LogDeny = true
				}
			}
		}

		// Check for missing plugins
		if !configAnalysis.ErrorsPlugin {
			issues = append(issues, CoreDNSIssue{
				Type:     "MissingPlugin",
				Severity: "low",
				Message:  "errors plugin not found in Corefile; DNS errors will be silently dropped",
			})
		}
		if !configAnalysis.HealthPlugin {
			issues = append(issues, CoreDNSIssue{
				Type:     "MissingPlugin",
				Severity: "medium",
				Message:  "health plugin not found; CoreDNS health endpoint unavailable for monitoring",
			})
		}
		if !configAnalysis.ReadyPlugin {
			issues = append(issues, CoreDNSIssue{
				Type:     "MissingPlugin",
				Severity: "medium",
				Message:  "ready plugin not found; CoreDNS readiness probe may not work correctly",
			})
		}
		if !configAnalysis.ForwardPlugin {
			issues = append(issues, CoreDNSIssue{
				Type:     "MissingPlugin",
				Severity: "critical",
				Message:  "forward plugin not found; CoreDNS cannot resolve upstream DNS queries",
			})
		}
		if !configAnalysis.LoopPlugin {
			issues = append(issues, CoreDNSIssue{
				Type:     "MissingPlugin",
				Severity: "high",
				Message:  "loop plugin not found; DNS forwarding loops possible",
			})
		}
	}

	// Check stub domains from kube-system ConfigMap (kube-root-ca.crt is separate)
	for _, cm := range cms {
		if cm.Name == "coredns-custom" || strings.Contains(cm.Name, "stubdomains") {
			for k, v := range cm.Data {
				stubDomains = append(stubDomains, StubDomain{Domain: k, Upstream: v})
			}
		}
	}

	// Node coverage
	schedulableNodes := 0
	for _, n := range nodes {
		if n.Spec.Unschedulable {
			continue
		}
		isReady := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if !isReady {
			continue
		}
		schedulableNodes++
		hasDNS := corednsPodNodes[n.Name] || summary.DeploymentType == "Deployment"
		hasLocal := false
		if nodeLocalDNS.Deployed {
			for _, p := range pods {
				if strings.Contains(strings.ToLower(p.Name), "node-local") && p.Spec.NodeName == n.Name {
					hasLocal = true
					break
				}
			}
		}
		nodeCov = append(nodeCov, CoreDNSNodeCov{
			Node:     n.Name,
			HasDNS:   hasDNS,
			HasLocal: hasLocal,
		})
	}

	summary.TotalNodes = schedulableNodes
	summary.NodesWithDNS = len(corednsPodNodes)
	if summary.DeploymentType == "Deployment" {
		summary.NodesWithDNS = schedulableNodes // Deployment mode serves all nodes
	}
	summary.ConfigIssues = len(issues)

	// Score
	score := 100
	if !summary.CoreDNSFound {
		score = 50 // Critical: no DNS
	} else {
		if summary.ReadyReplicas < summary.DesiredReplicas {
			score -= 20
		}
		if summary.RestartCount > 10 {
			score -= 10
		}
		score -= len(issues) * 3
		if !summary.HasNodeLocalDNS && schedulableNodes > 10 {
			score -= 5 // Recommend NodeLocal DNS for larger clusters
		}
	}
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if !summary.CoreDNSFound {
		recs = append(recs, "CoreDNS not found; cluster DNS resolution is non-functional")
	} else {
		if summary.ReadyReplicas < summary.DesiredReplicas {
			recs = append(recs, fmt.Sprintf("CoreDNS replicas %d/%d ready; check pod status and node resources", summary.ReadyReplicas, summary.DesiredReplicas))
		}
		if summary.RestartCount > 5 {
			recs = append(recs, fmt.Sprintf("CoreDNS pods have %d total restarts; investigate upstream DNS issues", summary.RestartCount))
		}
		if configAnalysis.HasCorefile && !configAnalysis.LoopPlugin {
			recs = append(recs, "Add 'loop' plugin to Corefile to prevent DNS forwarding loops")
		}
		if !summary.HasNodeLocalDNS && schedulableNodes > 10 {
			recs = append(recs, "Consider deploying NodeLocal DNS Cache to reduce DNS latency and upstream load")
		}
		if configAnalysis.ForwardPlugin && len(upstreamServers) == 0 {
			recs = append(recs, "Forward plugin found but no upstream servers detected; verify Corefile configuration")
		}
	}
	if len(recs) == 0 {
		recs = append(recs, "CoreDNS health and configuration look good")
	}

	return CoreDNSResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		DaemonSetHealth: dsHealth,
		ConfigAnalysis:  configAnalysis,
		NodeLocalDNS:    nodeLocalDNS,
		UpstreamServers: upstreamServers,
		StubDomains:     stubDomains,
		Issues:          issues,
		NodeCoverage:    nodeCov,
		Recommendations: recs,
	}
}
