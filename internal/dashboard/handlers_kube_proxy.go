package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeProxyHealthResult is the kube-proxy & network routing stability audit.
type KubeProxyHealthResult struct {
	Timestamp       time.Time           `json:"timestamp"`
	Score           int                 `json:"score"`
	Status          string              `json:"status"`
	Summary         KubeProxySummary    `json:"summary"`
	DaemonSetStatus DaemonSetStatus     `json:"daemonSetStatus"`
	ProxyMode       string              `json:"proxyMode"`
	ConfigIssues    []KubeProxyIssue    `json:"configIssues"`
	NodeCoverage    []NodeProxyCoverage `json:"nodeCoverage"`
	ServiceRouting  ServiceRoutingInfo  `json:"serviceRouting"`
	Recommendations []string            `json:"recommendations"`
}

// KubeProxySummary holds aggregate kube-proxy health metrics.
type KubeProxySummary struct {
	KubeProxyFound   bool   `json:"kubeProxyFound"`
	ProxyMode        string `json:"proxyMode"`
	DesiredNodes     int    `json:"desiredNodes"`
	ReadyNodes       int    `json:"readyNodes"`
	MissingNodes     int    `json:"missingNodes"`
	UnhealthyPods    int    `json:"unhealthyPods"`
	TotalServices    int    `json:"totalServices"`
	TotalEndpoints   int    `json:"totalEndpoints"`
	ConfigIssueCount int    `json:"configIssueCount"`
}

// DaemonSetStatus describes the kube-proxy DaemonSet state.
type DaemonSetStatus struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Desired    int    `json:"desired"`
	Current    int    `json:"current"`
	Ready      int    `json:"ready"`
	Updated    int    `json:"updated"`
	Available  int    `json:"available"`
	Image      string `json:"image"`
	Generation int64  `json:"generation"`
}

// KubeProxyIssue identifies a kube-proxy configuration or health issue.
type KubeProxyIssue struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Node     string `json:"node"`
	Pod      string `json:"pod"`
	Message  string `json:"message"`
}

// NodeProxyCoverage describes kube-proxy coverage per node.
type NodeProxyCoverage struct {
	Node     string `json:"node"`
	HasProxy bool   `json:"hasProxy"`
	Ready    bool   `json:"ready"`
	Status   string `json:"status"`
}

// ServiceRoutingInfo summarizes service routing health.
type ServiceRoutingInfo struct {
	TotalServices      int `json:"totalServices"`
	ClusterIPServices  int `json:"clusterIPServices"`
	NodePortServices   int `json:"nodePortServices"`
	LoadBalancerSVcs   int `json:"loadBalancerServices"`
	ExternalNameSVcs   int `json:"externalNameServices"`
	HeadlessServices   int `json:"headlessServices"`
	NoSelectorServices int `json:"noSelectorServices"`
}

func (s *Server) handleKubeProxyHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// List kube-proxy DaemonSet
	dss, err := rc.clientset.AppsV1().DaemonSets("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list daemonsets: %v", err))
		return
	}

	// List all pods in kube-system
	pods, err := rc.clientset.CoreV1().Pods("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	// List nodes
	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	// List services across all namespaces
	svcs, err := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		svcs = &corev1.ServiceList{}
	}

	// List configmaps in kube-system (for proxy mode detection)
	cms, err := rc.clientset.CoreV1().ConfigMaps("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		cms = &corev1.ConfigMapList{}
	}

	result := analyzeKubeProxyHealth(dss.Items, pods.Items, nodes.Items, svcs.Items, cms.Items)
	writeJSON(w, result)
}

func analyzeKubeProxyHealth(dss []appsv1.DaemonSet, pods []corev1.Pod, nodes []corev1.Node, svcs []corev1.Service, cms []corev1.ConfigMap) KubeProxyHealthResult {
	now := time.Now()

	// Find kube-proxy DaemonSet
	var kpDS *appsv1.DaemonSet
	for i := range dss {
		if strings.Contains(strings.ToLower(dss[i].Name), "kube-proxy") {
			kpDS = &dss[i]
			break
		}
	}

	// Detect proxy mode from configmaps
	proxyMode := "iptables" // default
	for _, cm := range cms {
		if strings.Contains(strings.ToLower(cm.Name), "kube-proxy") {
			if mode, ok := cm.Data["mode"]; ok && mode != "" {
				proxyMode = mode
			}
			if config, ok := cm.Data["config.conf"]; ok {
				if strings.Contains(config, "mode: ipvs") {
					proxyMode = "ipvs"
				} else if strings.Contains(config, "mode: ebpf") {
					proxyMode = "ebpf"
				} else if strings.Contains(config, "mode: nft") {
					proxyMode = "nftables"
				}
			}
		}
	}

	summary := KubeProxySummary{
		KubeProxyFound: kpDS != nil,
		ProxyMode:      proxyMode,
	}
	svcRouting := ServiceRoutingInfo{}

	for _, svc := range svcs {
		svcRouting.TotalServices++
		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			if svc.Spec.ClusterIP == "None" {
				svcRouting.HeadlessServices++
			} else {
				svcRouting.ClusterIPServices++
			}
		case corev1.ServiceTypeNodePort:
			svcRouting.NodePortServices++
		case corev1.ServiceTypeLoadBalancer:
			svcRouting.LoadBalancerSVcs++
		case corev1.ServiceTypeExternalName:
			svcRouting.ExternalNameSVcs++
		}
		if len(svc.Spec.Selector) == 0 {
			svcRouting.NoSelectorServices++
		}
	}
	summary.TotalServices = svcRouting.TotalServices

	// Count schedulable nodes
	schedulableNodes := []corev1.Node{}
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
		if isReady {
			schedulableNodes = append(schedulableNodes, n)
		}
	}
	summary.DesiredNodes = len(schedulableNodes)

	var dsStatus DaemonSetStatus
	var nodeCoverage []NodeProxyCoverage
	var issues []KubeProxyIssue

	if kpDS != nil {
		dsStatus = DaemonSetStatus{
			Name:      kpDS.Name,
			Namespace: kpDS.Namespace,
			Desired:   int(kpDS.Status.DesiredNumberScheduled),
			Current:   int(kpDS.Status.CurrentNumberScheduled),
			Ready:     int(kpDS.Status.NumberReady),
			Updated:   int(kpDS.Status.UpdatedNumberScheduled),
			Available: int(kpDS.Status.NumberAvailable),
		}

		// Get image
		if len(kpDS.Spec.Template.Spec.Containers) > 0 {
			dsStatus.Image = kpDS.Spec.Template.Spec.Containers[0].Image
		}

		summary.ReadyNodes = dsStatus.Ready
		summary.MissingNodes = dsStatus.Desired - dsStatus.Current

		// Check for unhealthy kube-proxy pods
		kpPods := []corev1.Pod{}
		for _, p := range pods {
			if strings.Contains(strings.ToLower(p.Name), "kube-proxy") {
				kpPods = append(kpPods, p)
			}
		}

		// Build node coverage map
		podNodeMap := make(map[string]bool)
		for _, p := range kpPods {
			podNodeMap[p.Spec.NodeName] = true
			if p.Status.Phase != corev1.PodRunning {
				summary.UnhealthyPods++
				issues = append(issues, KubeProxyIssue{
					Type:     "PodNotRunning",
					Severity: "high",
					Node:     p.Spec.NodeName,
					Pod:      p.Name,
					Message:  fmt.Sprintf("kube-proxy pod %s on node %s is %s", p.Name, p.Spec.NodeName, p.Status.Phase),
				})
			} else {
				// Check restart count
				for _, cs := range p.Status.ContainerStatuses {
					if cs.RestartCount > 5 {
						issues = append(issues, KubeProxyIssue{
							Type:     "HighRestartCount",
							Severity: "medium",
							Node:     p.Spec.NodeName,
							Pod:      p.Name,
							Message:  fmt.Sprintf("kube-proxy pod %s has restarted %d times", p.Name, cs.RestartCount),
						})
					}
				}
			}
		}

		// Check node coverage
		for _, n := range schedulableNodes {
			hasProxy := podNodeMap[n.Name]
			coverage := NodeProxyCoverage{
				Node:     n.Name,
				HasProxy: hasProxy,
				Ready:    hasProxy,
				Status:   "healthy",
			}
			if !hasProxy {
				coverage.Status = "missing"
				issues = append(issues, KubeProxyIssue{
					Type:     "MissingProxy",
					Severity: "critical",
					Node:     n.Name,
					Message:  fmt.Sprintf("No kube-proxy pod found on node %s; service routing may be broken", n.Name),
				})
			}
			nodeCoverage = append(nodeCoverage, coverage)
		}
	} else {
		// kube-proxy not found — could be using CNI with built-in routing (e.g., Cilium, Calico eBPF)
		issues = append(issues, KubeProxyIssue{
			Type:     "KubeProxyNotFound",
			Severity: "info",
			Message:  "kube-proxy DaemonSet not found; cluster may use eBPF-based CNI (Cilium/Calico) with kube-proxy replacement",
		})
		// Still build node coverage
		for _, n := range schedulableNodes {
			nodeCoverage = append(nodeCoverage, NodeProxyCoverage{
				Node:     n.Name,
				HasProxy: false,
				Ready:    true,
				Status:   "ebpf-replacement",
			})
		}
	}

	summary.ConfigIssueCount = len(issues)

	// Score
	score := 100
	if !summary.KubeProxyFound {
		// Not necessarily bad if using eBPF replacement
		score = 95 // slight deduction for unable to verify
	} else {
		score -= summary.MissingNodes * 10
		score -= summary.UnhealthyPods * 5
		score -= summary.ConfigIssueCount * 3
		if proxyMode == "iptables" && svcRouting.TotalServices > 1000 {
			score -= 5 // iptables doesn't scale well with many services
			issues = append(issues, KubeProxyIssue{
				Type:     "IPTablesScale",
				Severity: "medium",
				Message:  fmt.Sprintf("iptables mode with %d services may cause latency; consider switching to ipvs mode", svcRouting.TotalServices),
			})
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
	if !summary.KubeProxyFound {
		recs = append(recs, "kube-proxy not detected; verify CNI provides kube-proxy replacement (Cilium/Calico eBPF)")
	} else {
		if summary.MissingNodes > 0 {
			recs = append(recs, fmt.Sprintf("%d node(s) missing kube-proxy; check DaemonSet scheduling and node taints", summary.MissingNodes))
		}
		if summary.UnhealthyPods > 0 {
			recs = append(recs, fmt.Sprintf("%d unhealthy kube-proxy pod(s); investigate pod logs and node conditions", summary.UnhealthyPods))
		}
		if proxyMode == "iptables" && svcRouting.TotalServices > 500 {
			recs = append(recs, "Consider switching to ipvs mode for better performance with many services")
		}
		if svcRouting.NoSelectorServices > 0 {
			recs = append(recs, fmt.Sprintf("%d service(s) without selector; verify manually created endpoints exist", svcRouting.NoSelectorServices))
		}
	}
	if len(recs) == 0 {
		recs = append(recs, "kube-proxy health and network routing stability look good")
	}

	sort.Slice(nodeCoverage, func(i, j int) bool {
		if nodeCoverage[i].Status != nodeCoverage[j].Status {
			return nodeCoverage[i].Status < nodeCoverage[j].Status // missing < healthy
		}
		return nodeCoverage[i].Node < nodeCoverage[j].Node
	})

	return KubeProxyHealthResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		DaemonSetStatus: dsStatus,
		ProxyMode:       proxyMode,
		ConfigIssues:    issues,
		NodeCoverage:    nodeCoverage,
		ServiceRouting:  svcRouting,
		Recommendations: recs,
	}
}
