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

// IPFResult is the image pull failure & container start failure analysis.
type IPFResult struct {
	ScannedAt        time.Time      `json:"scannedAt"`
	Summary          IPFSummary     `json:"summary"`
	FailedContainers []IPFEntry     `json:"failedContainers"`
	ByImage          []IPFImageStat `json:"byImage"`
	ByNamespace      []IPFNSEntry   `json:"byNamespace"`
	ByReason         map[string]int `json:"byReason"`
	Issues           []IPFIssue     `json:"issues"`
	Recommendations  []string       `json:"recommendations"`
}

// IPFSummary aggregates image pull failure statistics.
type IPFSummary struct {
	TotalPods            int `json:"totalPods"`
	FailedPods           int `json:"failedPods"` // pods with at least 1 failed container
	ImagePullBackOff     int `json:"imagePullBackOff"`
	ErrImagePull         int `json:"errImagePull"`
	ErrImageNeverPull    int `json:"errImageNeverPull"`
	CreateContainerError int `json:"createContainerError"`
	RegistryAuthFailure  int `json:"registryAuthFailure"`
	RateLimited          int `json:"rateLimited"`
	OtherStartErrors     int `json:"otherStartErrors"`
	UniqueFailedImages   int `json:"uniqueFailedImages"`
	RetriesTotal         int `json:"retriesTotal"`
	HealthScore          int `json:"healthScore"` // 0-100
}

// IPFEntry describes one failed container.
type IPFEntry struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	ContainerName string `json:"containerName"`
	Image         string `json:"image"`
	Reason        string `json:"reason"` // ImagePullBackOff, ErrImagePull, etc.
	Message       string `json:"message"`
	RestartCount  int32  `json:"restartCount"`
	Age           string `json:"age"`
	RiskLevel     string `json:"riskLevel"`
}

// IPFImageStat aggregates failures per unique image.
type IPFImageStat struct {
	Image        string         `json:"image"`
	FailureCount int            `json:"failureCount"`
	PodsAffected int            `json:"podsAffected"`
	Reasons      map[string]int `json:"reasons"`
	Registry     string         `json:"registry"`
	RiskLevel    string         `json:"riskLevel"`
}

// IPFNSEntry per-namespace failure stats.
type IPFNSEntry struct {
	Namespace      string `json:"namespace"`
	TotalPods      int    `json:"totalPods"`
	FailedPods     int    `json:"failedPods"`
	ImagePullFails int    `json:"imagePullFails"`
	HealthScore    int    `json:"healthScore"`
}

// IPFIssue is a detected problem.
type IPFIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleImagePullFailures tracks image pull and container start failures.
// GET /api/operations/image-pull-failures
func (s *Server) handleImagePullFailures(w http.ResponseWriter, r *http.Request) {
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

	result := IPFResult{ScannedAt: time.Now(), ByReason: make(map[string]int)}
	now := time.Now()

	// Build image → stat map
	imageStats := make(map[string]*IPFImageStat)
	nsMap := make(map[string]*IPFNSEntry)

	for _, pod := range pods.Items {
		result.Summary.TotalPods++
		nsStat := ipfGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.TotalPods++

		podHasFailure := false

		// Check container statuses
		allStatuses := append([]corev1.ContainerStatus{}, pod.Status.ContainerStatuses...)
		allStatuses = append(allStatuses, pod.Status.InitContainerStatuses...)

		for _, cs := range allStatuses {
			if cs.Ready && cs.State.Waiting == nil {
				continue // healthy
			}

			var reason, message string

			if cs.State.Waiting != nil {
				reason = cs.State.Waiting.Reason
				message = cs.State.Waiting.Message
			} else if cs.State.Terminated != nil {
				reason = cs.State.Terminated.Reason
				message = cs.State.Terminated.Message
			}

			if reason == "" {
				continue
			}

			// Only track actual failures, not normal "ContainerCreating"
			if !ipfIsFailure(reason) {
				continue
			}

			podHasFailure = true

			entry := IPFEntry{
				PodName:       pod.Name,
				Namespace:     pod.Namespace,
				ContainerName: cs.Name,
				Image:         cs.Image,
				Reason:        reason,
				Message:       ipfTruncateMessage(message),
				RestartCount:  cs.RestartCount,
				Age:           now.Sub(pod.CreationTimestamp.Time).Round(time.Minute).String(),
			}
			entry.RiskLevel = ipfAssessRisk(reason, cs.RestartCount)

			result.FailedContainers = append(result.FailedContainers, entry)
			result.Summary.RetriesTotal += int(cs.RestartCount)
			result.ByReason[reason]++

			// Classify reason
			switch reason {
			case "ImagePullBackOff":
				result.Summary.ImagePullBackOff++
			case "ErrImagePull":
				result.Summary.ErrImagePull++
			case "ErrImageNeverPull":
				result.Summary.ErrImageNeverPull++
			case "CreateContainerConfigError", "CreateContainerError":
				result.Summary.CreateContainerError++
			default:
				result.Summary.OtherStartErrors++
			}

			// Check for registry auth / rate limiting in message
			lowerMsg := strings.ToLower(message)
			if strings.Contains(lowerMsg, "unauthorized") || strings.Contains(lowerMsg, "authentication") {
				result.Summary.RegistryAuthFailure++
				result.Issues = append(result.Issues, IPFIssue{
					Severity: "critical", Type: "registry-auth",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
					Message:  fmt.Sprintf("Registry authentication failure for %s — check imagePullSecrets", cs.Image),
				})
			}
			if strings.Contains(lowerMsg, "rate limit") || strings.Contains(lowerMsg, "toomanyrequests") {
				result.Summary.RateLimited++
				result.Issues = append(result.Issues, IPFIssue{
					Severity: "warning", Type: "rate-limited",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
					Message:  fmt.Sprintf("Docker Hub rate limited for %s — use a mirror or internal registry", cs.Image),
				})
			}

			// Generate issue
			if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
				result.Issues = append(result.Issues, IPFIssue{
					Severity: "critical", Type: "image-pull-failure",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
					Message:  fmt.Sprintf("Pod %s/%s cannot pull image %s — %s", pod.Namespace, pod.Name, cs.Image, reason),
				})
			} else if reason == "CreateContainerError" || reason == "CreateContainerConfigError" {
				result.Issues = append(result.Issues, IPFIssue{
					Severity: "critical", Type: "container-start-error",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
					Message:  fmt.Sprintf("Pod %s/%s container %s failed to start: %s", pod.Namespace, pod.Name, cs.Name, reason),
				})
			}

			// Image stats
			stat := ipfGetOrCreateImage(imageStats, cs.Image)
			stat.FailureCount++
			stat.Reasons[reason]++

			// Track unique pods affected
			nsStat.ImagePullFails++
		}

		if podHasFailure {
			result.Summary.FailedPods++
			nsStat.FailedPods++
		}
	}

	// Finalize image stats
	for _, stat := range imageStats {
		stat.Registry = ipfExtractRegistry(stat.Image)
		stat.RiskLevel = ipfImageRisk(stat)
		result.ByImage = append(result.ByImage, *stat)
	}
	result.Summary.UniqueFailedImages = len(result.ByImage)

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		nsStat.HealthScore = ipfNSScore(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.FailedContainers, func(i, j int) bool {
		return ipfRiskRank(result.FailedContainers[i].RiskLevel) < ipfRiskRank(result.FailedContainers[j].RiskLevel)
	})
	sort.Slice(result.ByImage, func(i, j int) bool {
		return result.ByImage[i].FailureCount > result.ByImage[j].FailureCount
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].FailedPods > result.ByNamespace[j].FailedPods
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return ipfIssueRank(result.Issues[i].Severity) < ipfIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = ipfScore(result.Summary)
	result.Recommendations = ipfGenRecs(result.Summary, result.ByImage)

	writeJSON(w, result)
}

// ipfIsFailure checks if a reason indicates an actual failure.
func ipfIsFailure(reason string) bool {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull", "ErrImageNeverPull",
		"RegistryUnavailable", "InvalidImageName",
		"CreateContainerConfigError", "CreateContainerError",
		"CrashLoopBackOff":
		return true
	}
	return false
}

// ipfAssessRisk determines risk level for a failed container.
func ipfAssessRisk(reason string, restarts int32) string {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull", "ErrImageNeverPull":
		return "critical"
	case "CreateContainerError", "CreateContainerConfigError":
		return "critical"
	case "CrashLoopBackOff":
		if restarts > 20 {
			return "critical"
		}
		return "high"
	case "InvalidImageName":
		return "high"
	}
	if restarts > 10 {
		return "high"
	}
	return "medium"
}

// ipfTruncateMessage truncates long error messages.
func ipfTruncateMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 200 {
		return msg[:197] + "..."
	}
	return msg
}

// ipfExtractRegistry extracts the registry from an image string.
func ipfExtractRegistry(image string) string {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			return parts[0] // has domain
		}
		return "docker.io" // library image with path
	}
	return "docker.io" // no slash = official Docker Hub image
}

// ipfImageRisk assesses risk for a failing image.
func ipfImageRisk(stat *IPFImageStat) string {
	if stat.FailureCount > 5 {
		return "critical"
	}
	if stat.FailureCount > 2 {
		return "high"
	}
	return "medium"
}

// ipfNSScore computes 0-100 for a namespace.
func ipfNSScore(ns IPFNSEntry) int {
	if ns.TotalPods == 0 {
		return 100
	}
	failurePct := float64(ns.FailedPods) / float64(ns.TotalPods) * 100
	return 100 - int(failurePct)
}

// ipfScore computes cluster-wide 0-100.
func ipfScore(s IPFSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	failurePct := float64(s.FailedPods) / float64(s.TotalPods) * 100
	score := 100 - int(failurePct*2)
	if score < 0 {
		score = 0
	}
	return score
}

// ipfGenRecs produces actionable advice.
func ipfGenRecs(s IPFSummary, byImage []IPFImageStat) []string {
	var recs []string

	if s.ImagePullBackOff > 0 || s.ErrImagePull > 0 {
		recs = append(recs, fmt.Sprintf("%d image pull failure(s) — check image names, tags, and registry access", s.ImagePullBackOff+s.ErrImagePull))
	}
	if s.RegistryAuthFailure > 0 {
		recs = append(recs, fmt.Sprintf("%d registry authentication failure(s) — verify imagePullSecrets are configured", s.RegistryAuthFailure))
	}
	if s.RateLimited > 0 {
		recs = append(recs, fmt.Sprintf("%d rate-limited pull(s) — configure Docker Hub mirror or use internal registry", s.RateLimited))
	}
	if s.CreateContainerError > 0 {
		recs = append(recs, fmt.Sprintf("%d container start error(s) — check container config, entrypoint, and volume mounts", s.CreateContainerError))
	}
	if s.UniqueFailedImages > 0 {
		top := ""
		if len(byImage) > 0 {
			top = fmt.Sprintf(" (e.g. %s: %d failures)", byImage[0].Image, byImage[0].FailureCount)
		}
		recs = append(recs, fmt.Sprintf("%d unique image(s) failing to pull%s", s.UniqueFailedImages, top))
	}
	if s.RetriesTotal > 0 {
		recs = append(recs, fmt.Sprintf("Total %d container restarts across failed pods — exponential backoff is active", s.RetriesTotal))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Image pull health score is %d/100 — multiple containers cannot start", s.HealthScore))
	}
	if s.FailedPods == 0 {
		recs = append(recs, "No image pull or container start failures — all pods healthy")
	}

	return recs
}

func ipfGetOrCreateImage(m map[string]*IPFImageStat, image string) *IPFImageStat {
	if s, ok := m[image]; ok {
		return s
	}
	s := &IPFImageStat{Image: image, Reasons: make(map[string]int)}
	m[image] = s
	return s
}

func ipfGetOrCreateNS(m map[string]*IPFNSEntry, ns string) *IPFNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &IPFNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func ipfRiskRank(level string) int {
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

func ipfIssueRank(s string) int {
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
