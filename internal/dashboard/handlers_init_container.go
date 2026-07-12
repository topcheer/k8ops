package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitContainerResult is the init container reliability & startup dependency audit.
type InitContainerResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         InitContainerSummary  `json:"summary"`
	ByWorkload      []InitContainerEntry  `json:"byWorkload"`
	Issues          []InitContainerIssue  `json:"issues"`
	ByNamespace     []InitContainerNSStat `json:"byNamespace"`
	Recommendations []string              `json:"recommendations"`
}

// InitContainerSummary aggregates init container statistics.
type InitContainerSummary struct {
	TotalPods           int `json:"totalPods"`
	PodsWithInit        int `json:"podsWithInit"`
	TotalInitContainers int `json:"totalInitContainers"`
	MissingResources    int `json:"missingResources"`
	MissingLimits       int `json:"missingLimits"`
	NoProbe             int `json:"noProbe"`
	ExcessiveRetries    int `json:"excessiveRetries"`
	HighRisk            int `json:"highRisk"`
	HealthScore         int `json:"healthScore"`
}

// InitContainerEntry describes one workload's init container configuration.
type InitContainerEntry struct {
	Name           string              `json:"name"`
	Namespace      string              `json:"namespace"`
	WorkloadType   string              `json:"workloadType"`
	InitCount      int                 `json:"initCount"`
	InitContainers []InitContainerSpec `json:"initContainers"`
	RiskLevel      string              `json:"riskLevel"`
}

// InitContainerSpec describes a single init container.
type InitContainerSpec struct {
	Name              string `json:"name"`
	Image             string `json:"image"`
	HasCPURequest     bool   `json:"hasCPURequest"`
	HasMemRequest     bool   `json:"hasMemRequest"`
	HasCPULimit       bool   `json:"hasCPULimit"`
	HasMemLimit       bool   `json:"hasMemLimit"`
	HasReadinessProbe bool   `json:"hasReadinessProbe"`
	RestartPolicy     string `json:"restartPolicy,omitempty"`
}

// InitContainerIssue is a detected init container issue.
type InitContainerIssue struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// InitContainerNSStat shows init container stats per namespace.
type InitContainerNSStat struct {
	Namespace    string `json:"namespace"`
	TotalPods    int    `json:"totalPods"`
	PodsWithInit int    `json:"podsWithInit"`
	IssueCount   int    `json:"issueCount"`
}

// initContainerAuditCore performs the audit logic on a pod list (testable).
func initContainerAuditCore(pods []corev1.Pod) InitContainerResult {
	result := InitContainerResult{
		ScannedAt: time.Now(),
	}

	nsStats := make(map[string]*InitContainerNSStat)

	for _, pod := range pods {
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &InitContainerNSStat{Namespace: ns}
		}
		nsStats[ns].TotalPods++
		result.Summary.TotalPods++

		initContainers := pod.Spec.InitContainers
		if len(initContainers) == 0 {
			continue
		}

		result.Summary.PodsWithInit++
		nsStats[ns].PodsWithInit++
		result.Summary.TotalInitContainers += len(initContainers)

		wlName, wlType := podOwnerInfo(&pod)

		entry := InitContainerEntry{
			Name:         wlName,
			Namespace:    ns,
			WorkloadType: wlType,
			InitCount:    len(initContainers),
		}

		podRisk := "low"
		for _, ic := range initContainers {
			spec := InitContainerSpec{
				Name:          ic.Name,
				Image:         ic.Image,
				RestartPolicy: initContainerRestartPolicyString(ic.RestartPolicy),
			}

			if ic.Resources.Requests.Cpu() != nil && !ic.Resources.Requests.Cpu().IsZero() {
				spec.HasCPURequest = true
			}
			if ic.Resources.Requests.Memory() != nil && !ic.Resources.Requests.Memory().IsZero() {
				spec.HasMemRequest = true
			}
			if ic.Resources.Limits.Cpu() != nil && !ic.Resources.Limits.Cpu().IsZero() {
				spec.HasCPULimit = true
			}
			if ic.Resources.Limits.Memory() != nil && !ic.Resources.Limits.Memory().IsZero() {
				spec.HasMemLimit = true
			}
			if ic.ReadinessProbe != nil {
				spec.HasReadinessProbe = true
			}

			if !spec.HasCPURequest || !spec.HasMemRequest {
				result.Summary.MissingResources++
				result.Issues = append(result.Issues, InitContainerIssue{
					PodName:   pod.Name,
					Namespace: ns,
					Container: ic.Name,
					Issue:     "init container missing resource requests (CPU or memory)",
					Severity:  "medium",
				})
				if podRisk == "low" {
					podRisk = "medium"
				}
			}

			if !spec.HasCPULimit || !spec.HasMemLimit {
				result.Summary.MissingLimits++
				result.Issues = append(result.Issues, InitContainerIssue{
					PodName:   pod.Name,
					Namespace: ns,
					Container: ic.Name,
					Issue:     "init container missing resource limits (no bounded resource usage)",
					Severity:  "low",
				})
			}

			if ic.RestartPolicy != nil && *ic.RestartPolicy == corev1.ContainerRestartPolicyAlways {
				result.Summary.ExcessiveRetries++
				result.Issues = append(result.Issues, InitContainerIssue{
					PodName:   pod.Name,
					Namespace: ns,
					Container: ic.Name,
					Issue:     "init container uses RestartPolicy=Always (acts as sidecar, may delay startup)",
					Severity:  "low",
				})
			}

			entry.InitContainers = append(entry.InitContainers, spec)
		}

		if len(initContainers) > 5 {
			result.Issues = append(result.Issues, InitContainerIssue{
				PodName:   pod.Name,
				Namespace: ns,
				Container: fmt.Sprintf("%d init containers", len(initContainers)),
				Issue:     "excessive number of init containers (>5) increases startup latency and failure surface",
				Severity:  "medium",
			})
			podRisk = "medium"
		}

		entry.RiskLevel = podRisk
		if podRisk == "medium" {
			result.Summary.HighRisk++
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodsWithInit > result.ByNamespace[j].PodsWithInit
	})

	result.Summary.HealthScore = initContainerScore(result.Summary)
	result.Recommendations = initContainerRecommendations(result.Summary)

	return result
}

// initContainerScore calculates health score from summary stats.
func initContainerScore(s InitContainerSummary) int {
	if s.TotalInitContainers == 0 {
		return 100
	}
	base := 100
	resourcePenalty := s.MissingResources * 10
	limitPenalty := s.MissingLimits * 3
	retryPenalty := s.ExcessiveRetries * 2
	highRiskPenalty := s.HighRisk * 8
	score := base - resourcePenalty - limitPenalty - retryPenalty - highRiskPenalty
	if score < 0 {
		score = 0
	}
	return score
}

// initContainerRecommendations generates recommendations from summary.
func initContainerRecommendations(s InitContainerSummary) []string {
	var recs []string
	if s.MissingResources > 0 {
		recs = append(recs, fmt.Sprintf("%d init containers are missing resource requests — add CPU/memory requests to ensure proper scheduling", s.MissingResources))
	}
	if s.MissingLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d init containers are missing resource limits — add limits to prevent resource exhaustion during init phase", s.MissingLimits))
	}
	if s.ExcessiveRetries > 0 {
		recs = append(recs, fmt.Sprintf("%d init containers use RestartPolicy=Always — review if sidecar behavior is intended or if they should be regular init containers", s.ExcessiveRetries))
	}
	if s.PodsWithInit > 0 && s.HighRisk == 0 {
		recs = append(recs, "all init containers are properly configured — no high-risk issues detected")
	}
	return recs
}

// podOwnerInfo extracts workload name and type from pod owner references.
func podOwnerInfo(pod *corev1.Pod) (string, string) {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind != "" && ref.Name != "" {
			return ref.Name, ref.Kind
		}
	}
	return pod.Name, "Pod"
}

// handleInitContainerAudit audits init container reliability and startup dependencies.
// GET /api/product/init-container-audit
func (s *Server) handleInitContainerAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := initContainerAuditCore(pods.Items)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, result)
}

// initContainerRestartPolicyString safely converts a *ContainerRestartPolicy to string.
func initContainerRestartPolicyString(rp *corev1.ContainerRestartPolicy) string {
	if rp == nil {
		return ""
	}
	return string(*rp)
}
