package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CPResult is the control plane health analysis.
type CPResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         CPSummary `json:"summary"`
	Components      []CPEntry `json:"components"`
	Issues          []CPIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// CPSummary aggregates control plane health.
type CPSummary struct {
	TotalComponents     int  `json:"totalComponents"`
	HealthyComponents   int  `json:"healthyComponents"`
	UnhealthyComponents int  `json:"unhealthyComponents"`
	TotalPods           int  `json:"totalPods"`
	ReadyPods           int  `json:"readyPods"`
	RestartedPods       int  `json:"restartedPods"`
	HasEtcd             bool `json:"hasEtcd"`
	HasScheduler        bool `json:"hasScheduler"`
	HasControllerMgr    bool `json:"hasControllerMgr"`
	HasAPIServer        bool `json:"hasAPIServer"`
	HealthScore         int  `json:"healthScore"` // 0-100
}

// CPEntry describes one control plane component's health.
type CPEntry struct {
	Component      string     `json:"component"` // kube-apiserver, kube-scheduler, etc.
	PodName        string     `json:"podName"`
	Namespace      string     `json:"namespace"`
	NodeName       string     `json:"nodeName"`
	Phase          string     `json:"phase"`
	Ready          bool       `json:"ready"`
	RestartCount   int32      `json:"restartCount"`
	StartTime      *time.Time `json:"startTime,omitempty"`
	UptimeHours    float64    `json:"uptimeHours"`
	KubeletVersion string     `json:"kubeletVersion,omitempty"`
	RiskLevel      string     `json:"riskLevel"`
}

// CPIssue is a detected control plane problem.
type CPIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// Control plane component identifiers
var cpComponents = map[string]bool{
	"kube-apiserver":          true,
	"kube-scheduler":          true,
	"kube-controller-manager": true,
	"etcd":                    true,
}

// handleControlPlaneHealth checks control plane component health.
// GET /api/operations/control-plane
func (s *Server) handleControlPlaneHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// List pods in kube-system
	pods, err := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build node → kubelet version map
	nodeVersions := make(map[string]string)
	for _, node := range nodes.Items {
		nodeVersions[node.Name] = node.Status.NodeInfo.KubeletVersion
	}

	result := CPResult{ScannedAt: time.Now()}
	compMap := make(map[string][]CPEntry)

	for _, pod := range pods.Items {
		// Check if this is a control plane pod
		for _, c := range pod.Spec.Containers {
			if cpComponents[c.Name] {
				entry := CPEntry{
					Component: c.Name,
					PodName:   pod.Name,
					Namespace: pod.Namespace,
					NodeName:  pod.Spec.NodeName,
					Phase:     string(pod.Status.Phase),
				}

				// Ready status from container statuses
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Name == c.Name {
						entry.Ready = cs.Ready
						entry.RestartCount = cs.RestartCount
						break
					}
				}

				// Uptime
				if pod.Status.StartTime != nil {
					entry.StartTime = &pod.Status.StartTime.Time
					entry.UptimeHours = time.Since(pod.Status.StartTime.Time).Hours()
				}

				entry.KubeletVersion = nodeVersions[pod.Spec.NodeName]
				entry.RiskLevel = cpAssessRisk(entry)
				compMap[c.Name] = append(compMap[c.Name], entry)
				break
			}
		}
	}

	// Build summary and entries
	for comp, entries := range compMap {
		for _, e := range entries {
			result.Components = append(result.Components, e)
			result.Summary.TotalComponents++

			if e.Ready {
				result.Summary.HealthyComponents++
			} else {
				result.Summary.UnhealthyComponents++
				result.Issues = append(result.Issues, CPIssue{
					Severity: "critical", Type: "component-not-ready",
					Resource: fmt.Sprintf("%s/%s", e.Namespace, e.PodName),
					Message:  fmt.Sprintf("Control plane component %s (%s) is not ready — cluster stability at risk", e.Component, e.PodName),
				})
			}

			if e.RestartCount > 0 {
				result.Summary.RestartedPods++
				if e.RestartCount >= 3 {
					result.Issues = append(result.Issues, CPIssue{
						Severity: "warning", Type: "component-restarts",
						Resource: fmt.Sprintf("%s/%s", e.Namespace, e.PodName),
						Message:  fmt.Sprintf("Control plane component %s has restarted %d times — may indicate instability", e.Component, e.RestartCount),
					})
				}
			}

			if e.UptimeHours < 1 && e.UptimeHours > 0 {
				result.Issues = append(result.Issues, CPIssue{
					Severity: "warning", Type: "recent-restart",
					Resource: fmt.Sprintf("%s/%s", e.Namespace, e.PodName),
					Message:  fmt.Sprintf("Control plane component %s restarted recently (uptime: %.1fh) — check for crashes", e.Component, e.UptimeHours),
				})
			}
		}

		switch comp {
		case "etcd":
			result.Summary.HasEtcd = true
		case "kube-scheduler":
			result.Summary.HasScheduler = true
		case "kube-controller-manager":
			result.Summary.HasControllerMgr = true
		case "kube-apiserver":
			result.Summary.HasAPIServer = true
		}
	}

	// Check for missing components
	if !result.Summary.HasEtcd {
		result.Issues = append(result.Issues, CPIssue{
			Severity: "critical", Type: "missing-component",
			Resource: "etcd",
			Message:  "No etcd pods found in kube-system — cluster state storage is at risk",
		})
	}
	if !result.Summary.HasAPIServer {
		result.Issues = append(result.Issues, CPIssue{
			Severity: "critical", Type: "missing-component",
			Resource: "kube-apiserver",
			Message:  "No kube-apiserver pods found in kube-system — API server is the cluster brain",
		})
	}

	// K3s/Docker Desktop may run components as processes, not pods
	if result.Summary.TotalComponents == 0 {
		result.Issues = append(result.Issues, CPIssue{
			Severity: "info", Type: "no-pod-components",
			Resource: "kube-system",
			Message:  "No control plane pods found in kube-system — cluster may use k3s/microk8s/kind which run components as host processes",
		})
	}

	sort.Slice(result.Components, func(i, j int) bool {
		return cpRiskRank(result.Components[i].RiskLevel) < cpRiskRank(result.Components[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return cpIssueRank(result.Issues[i].Severity) < cpIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = cpScore(result.Summary)
	result.Recommendations = cpGenRecs(result.Summary, result.Components)

	writeJSON(w, result)
}

// cpAssessRisk determines risk level.
func cpAssessRisk(entry CPEntry) string {
	if !entry.Ready {
		return "critical"
	}
	if entry.RestartCount >= 5 {
		return "high"
	}
	if entry.RestartCount >= 3 {
		return "medium"
	}
	if entry.UptimeHours < 1 && entry.UptimeHours > 0 {
		return "medium"
	}
	return "low"
}

// cpScore computes health score 0-100.
func cpScore(s CPSummary) int {
	if s.TotalComponents == 0 {
		return 100 // k3s/microk8s — can't monitor via pods
	}
	score := 100
	score -= s.UnhealthyComponents * 20
	score -= s.RestartedPods * 5
	// Missing critical components
	if !s.HasEtcd {
		score -= 20
	}
	if !s.HasAPIServer {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	return score
}

// cpGenRecs produces actionable advice.
func cpGenRecs(s CPSummary, components []CPEntry) []string {
	var recs []string

	if s.UnhealthyComponents > 0 {
		recs = append(recs, fmt.Sprintf("%d control plane component(s) are not ready — immediate investigation required, cluster stability at risk", s.UnhealthyComponents))
	}
	if s.RestartedPods > 0 {
		// Find top restarted
		var topComp *CPEntry
		for i := range components {
			if topComp == nil || components[i].RestartCount > topComp.RestartCount {
				topComp = &components[i]
			}
		}
		if topComp != nil && topComp.RestartCount > 0 {
			recs = append(recs, fmt.Sprintf("%d control plane pod(s) have restarted (top: %s with %d restarts) — check logs for crash patterns", s.RestartedPods, topComp.Component, topComp.RestartCount))
		}
	}
	if s.TotalComponents == 0 {
		recs = append(recs, "No control plane pods found — cluster likely uses k3s/microk8s/kind, monitor host processes directly")
	}
	if !s.HasEtcd && s.TotalComponents > 0 {
		recs = append(recs, "No etcd pods detected — if using managed Kubernetes (EKS/GKE/AKS), etcd is managed by provider")
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Control plane health score is %d/100 — review component health immediately", s.HealthScore))
	}
	if s.UnhealthyComponents == 0 && s.RestartedPods == 0 && s.TotalComponents > 0 {
		recs = append(recs, fmt.Sprintf("All %d control plane components are healthy — good cluster stability", s.HealthyComponents))
	}

	return recs
}

func cpRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func cpIssueRank(s string) int {
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

var _ = corev1.PodSpec{}
