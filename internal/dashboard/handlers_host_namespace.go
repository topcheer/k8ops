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

// HNResult is the host namespace & privilege exposure analysis.
type HNResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         HNSummary      `json:"summary"`
	ExposedPods     []HNEntry      `json:"exposedPods"`
	ByNamespace     map[string]int `json:"byNamespace"`
	ByExposure      map[string]int `json:"byExposure"`
	Issues          []HNIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// HNSummary aggregates exposure stats.
type HNSummary struct {
	TotalPods        int `json:"totalPods"`
	HostNetworkPods  int `json:"hostNetworkPods"`
	HostPIDPods      int `json:"hostPIDPods"`
	HostIPCPods      int `json:"hostIPCPods"`
	PrivilegedCtrs   int `json:"privilegedContainers"`
	HostPathMounts   int `json:"hostPathMounts"`
	CapAddContainers int `json:"capAddContainers"`
	RunAsRootCtrs    int `json:"runAsRootContainers"`
	ExposureScore    int `json:"exposureScore"` // 0-100, higher = safer
}

// HNEntry describes one exposed pod.
type HNEntry struct {
	PodName   string   `json:"podName"`
	Namespace string   `json:"namespace"`
	NodeName  string   `json:"nodeName"`
	Exposures []string `json:"exposures"` // hostNetwork, hostPID, hostIPC, privileged, hostPath, capAdd, runAsRoot
	RiskLevel string   `json:"riskLevel"`
}

// HNIssue is a detected exposure problem.
type HNIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleHostNamespace audits container host namespace and privilege exposure.
// GET /api/security/host-namespace
func (s *Server) handleHostNamespace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := HNResult{ScannedAt: time.Now(), ByNamespace: make(map[string]int), ByExposure: make(map[string]int)}
	result.Summary.TotalPods = len(pods.Items)

	for _, pod := range pods.Items {
		// Skip completed pods
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		var exposures []string

		// Check host namespaces
		if pod.Spec.HostNetwork {
			exposures = append(exposures, "hostNetwork")
			result.Summary.HostNetworkPods++
			result.ByExposure["hostNetwork"]++
		}
		if pod.Spec.HostPID {
			exposures = append(exposures, "hostPID")
			result.Summary.HostPIDPods++
			result.ByExposure["hostPID"]++
		}
		if pod.Spec.HostIPC {
			exposures = append(exposures, "hostIPC")
			result.Summary.HostIPCPods++
			result.ByExposure["hostIPC"]++
		}

		// Check containers for privileged, capabilities, hostPath, runAsRoot
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					exposures = append(exposures, fmt.Sprintf("privileged:%s", c.Name))
					result.Summary.PrivilegedCtrs++
					result.ByExposure["privileged"]++
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0 {
					exposures = append(exposures, fmt.Sprintf("runAsRoot:%s", c.Name))
					result.Summary.RunAsRootCtrs++
					result.ByExposure["runAsRoot"]++
				}
				if c.SecurityContext.Capabilities != nil && len(c.SecurityContext.Capabilities.Add) > 0 {
					caps := make([]string, len(c.SecurityContext.Capabilities.Add))
					for i, cap := range c.SecurityContext.Capabilities.Add {
						caps[i] = string(cap)
					}
					exposures = append(exposures, fmt.Sprintf("capAdd:%s:%s", c.Name, strings.Join(caps, ",")))
					result.Summary.CapAddContainers++
					result.ByExposure["capAdd"]++
				}
			}
		}

		// Check hostPath volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				exposures = append(exposures, fmt.Sprintf("hostPath:%s", vol.Name))
				result.Summary.HostPathMounts++
				result.ByExposure["hostPath"]++
			}
		}

		if len(exposures) > 0 {
			entry := HNEntry{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				NodeName:  pod.Spec.NodeName,
				Exposures: exposures,
				RiskLevel: hnAssessRisk(exposures),
			}
			result.ExposedPods = append(result.ExposedPods, entry)
			result.ByNamespace[pod.Namespace]++

			// Generate issues for critical exposures
			if hnAssessRisk(exposures) == "critical" {
				result.Issues = append(result.Issues, HNIssue{
					Severity: "critical", Type: "host-namespace-exposure",
					Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
					Message:  fmt.Sprintf("Pod %s/%s has critical host exposure: %s — can escape container isolation", pod.Namespace, pod.Name, strings.Join(exposures, ", ")),
				})
			}
		}
	}

	// Sort
	sort.Slice(result.ExposedPods, func(i, j int) bool {
		return hnRiskRank(result.ExposedPods[i].RiskLevel) < hnRiskRank(result.ExposedPods[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return hnIssueRank(result.Issues[i].Severity) < hnIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ExposureScore = hnScore(result.Summary)
	result.Recommendations = hnGenRecs(result.Summary)

	writeJSON(w, result)
}

// hnAssessRisk determines risk level from exposure types.
func hnAssessRisk(exposures []string) string {
	hasPrivileged := false
	hasHostNS := false
	for _, e := range exposures {
		if strings.HasPrefix(e, "privileged") {
			hasPrivileged = true
		}
		if e == "hostNetwork" || e == "hostPID" || e == "hostIPC" {
			hasHostNS = true
		}
	}

	if hasPrivileged && hasHostNS {
		return "critical"
	}
	if hasPrivileged || hasHostNS {
		return "high"
	}
	if len(exposures) >= 3 {
		return "high"
	}
	if len(exposures) > 0 {
		return "medium"
	}
	return "low"
}

// hnScore computes exposure safety score (higher = safer).
func hnScore(s HNSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.PrivilegedCtrs * 10
	score -= s.HostNetworkPods * 5
	score -= s.HostPIDPods * 5
	score -= s.HostIPCPods * 3
	score -= s.HostPathMounts * 3
	score -= s.CapAddContainers * 3
	score -= s.RunAsRootCtrs * 2
	if score < 0 {
		score = 0
	}
	return score
}

// hnGenRecs produces actionable advice.
func hnGenRecs(s HNSummary) []string {
	var recs []string

	if s.PrivilegedCtrs > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged container(s) — remove privileged flag, use specific capabilities instead for least privilege", s.PrivilegedCtrs))
	}
	if s.HostNetworkPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) using hostNetwork — grants access to node's network namespace, restrict to system pods only", s.HostNetworkPods))
	}
	if s.HostPIDPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) using hostPID — can see all node processes, restrict to monitoring agents only", s.HostPIDPods))
	}
	if s.HostPathMounts > 0 {
		recs = append(recs, fmt.Sprintf("%d hostPath mount(s) — grants node filesystem access, use PersistentVolumes or projected volumes instead", s.HostPathMounts))
	}
	if s.CapAddContainers > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) with added capabilities — drop ALL capabilities and add only required ones", s.CapAddContainers))
	}
	if s.RunAsRootCtrs > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) running as root (UID 0) — set runAsNonRoot: true or runAsUser: non-zero", s.RunAsRootCtrs))
	}
	if s.ExposureScore < 70 {
		recs = append(recs, fmt.Sprintf("Exposure safety score is %d/100 — multiple host namespace violations detected", s.ExposureScore))
	}
	if s.PrivilegedCtrs == 0 && s.HostNetworkPods == 0 && s.HostPIDPods == 0 && s.HostPathMounts == 0 {
		recs = append(recs, fmt.Sprintf("No critical host namespace exposures detected — good container isolation posture (score: %d/100)", s.ExposureScore))
	}

	return recs
}

func hnRiskRank(level string) int {
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

func hnIssueRank(s string) int {
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
