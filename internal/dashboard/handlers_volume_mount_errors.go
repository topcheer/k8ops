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

// VMResult is the volume mount & attach error analysis.
type VMResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         VMSummary      `json:"summary"`
	ErrorPods       []VMEntry      `json:"errorPods"`
	ByErrorType     map[string]int `json:"byErrorType"`
	Issues          []VMIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// VMSummary aggregates volume mount error stats.
type VMSummary struct {
	TotalPods          int `json:"totalPods"`
	StuckPods          int `json:"stuckPods"` // ContainerCreating with volume errors
	MountFailErrors    int `json:"mountFailErrors"`
	AttachFailErrors   int `json:"attachFailErrors"`
	ProvisioningErrors int `json:"provisioningErrors"`
	TimeoutErrors      int `json:"timeoutErrors"`
	HealthScore        int `json:"healthScore"`
}

// VMEntry describes one pod with volume mount issues.
type VMEntry struct {
	PodName      string  `json:"podName"`
	Namespace    string  `json:"namespace"`
	NodeName     string  `json:"nodeName"`
	Phase        string  `json:"phase"`
	ErrorType    string  `json:"errorType"` // mount_fail, attach_fail, provisioning, timeout
	ErrorMessage string  `json:"errorMessage"`
	PendingMin   float64 `json:"pendingMinutes"`
	RiskLevel    string  `json:"riskLevel"`
}

// VMIssue is a detected volume problem.
type VMIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleVolumeMountErrors tracks pods stuck due to volume mount/attach failures.
// GET /api/operations/volume-mount-errors
func (s *Server) handleVolumeMountErrors(w http.ResponseWriter, r *http.Request) {
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

	now := time.Now()
	result := VMResult{ScannedAt: now, ByErrorType: make(map[string]int)}
	result.Summary.TotalPods = len(pods.Items)

	for _, pod := range pods.Items {
		// Check ContainerCreating pods
		if pod.Status.Phase != corev1.PodPending {
			continue
		}

		// Check container statuses for waiting state
		hasVolumeError := false
		errorType := ""
		errorMsg := ""

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				msg := cs.State.Waiting.Message

				if reason == "ContainerCreating" {
					// Check event messages for volume errors
					if vmContainsVolumeError(msg) {
						hasVolumeError = true
						errorType, errorMsg = vmClassifyError(msg)
					}
				}
			}
		}

		// Also check pod conditions and events
		if !hasVolumeError {
			for _, cond := range pod.Status.Conditions {
				if cond.Message != "" && vmContainsVolumeError(cond.Message) {
					hasVolumeError = true
					errorType, errorMsg = vmClassifyError(cond.Message)
					break
				}
			}
		}

		// Check if pod has been pending for a while
		if hasVolumeError {
			pendingMin := now.Sub(pod.CreationTimestamp.Time).Minutes()

			result.Summary.StuckPods++
			entry := VMEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				NodeName:     pod.Spec.NodeName,
				Phase:        string(pod.Status.Phase),
				ErrorType:    errorType,
				ErrorMessage: vmTruncate(errorMsg, 150),
				PendingMin:   pendingMin,
				RiskLevel:    vmAssessRisk(pendingMin),
			}

			switch errorType {
			case "mount_fail":
				result.Summary.MountFailErrors++
			case "attach_fail":
				result.Summary.AttachFailErrors++
			case "provisioning":
				result.Summary.ProvisioningErrors++
			case "timeout":
				result.Summary.TimeoutErrors++
			}
			result.ByErrorType[errorType]++

			result.ErrorPods = append(result.ErrorPods, entry)

			if pendingMin > 10 {
				result.Issues = append(result.Issues, VMIssue{
					Severity: "warning", Type: "volume-stuck",
					Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
					Message:  fmt.Sprintf("Pod %s/%s stuck in Pending for %.0f minutes — %s: %s", pod.Namespace, pod.Name, pendingMin, errorType, vmTruncate(errorMsg, 80)),
				})
			}
		}
	}

	sort.Slice(result.ErrorPods, func(i, j int) bool {
		return result.ErrorPods[i].PendingMin > result.ErrorPods[j].PendingMin
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return vmIssueRank(result.Issues[i].Severity) < vmIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = vmScore(result.Summary)
	result.Recommendations = vmGenRecs(result.Summary)

	writeJSON(w, result)
}

// vmContainsVolumeError checks if a message contains volume-related error keywords.
func vmContainsVolumeError(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	keywords := []string{
		"mountvolume", "attachvolume", "detachvolume",
		"unable to mount", "unable to attach", "unable to detach",
		"volume is not attached", "volume not found",
		"failed to mount", "failed to attach", "failed to detach",
		"volume.mount", "volume.attach", "volume.detach",
		"nfs mount", "iscsi", "fc mount",
		"csi", "storageclass", "persistentvolumeclaim",
		"timeout expired waiting for volumes",
		"unfinished work",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// vmClassifyError determines the error type and extracts the message.
func vmClassifyError(msg string) (errorType, cleanMsg string) {
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") {
		return "timeout", msg
	}
	if strings.Contains(lower, "provision") || strings.Contains(lower, "storageclass") ||
		strings.Contains(lower, "persistentvolumeclaim") || strings.Contains(lower, "waiting for pvc") {
		return "provisioning", msg
	}
	if strings.Contains(lower, "attach") || strings.Contains(lower, "detach") {
		return "attach_fail", msg
	}
	if strings.Contains(lower, "mount") || strings.Contains(lower, "mountvolume") {
		return "mount_fail", msg
	}
	return "mount_fail", msg
}

// vmAssessRisk determines risk level based on pending duration.
func vmAssessRisk(pendingMin float64) string {
	if pendingMin > 15 {
		return "high"
	}
	if pendingMin > 5 {
		return "medium"
	}
	return "low"
}

// vmScore computes health score 0-100.
func vmScore(s VMSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.StuckPods * 10
	score -= s.MountFailErrors * 3
	score -= s.AttachFailErrors * 3
	score -= s.ProvisioningErrors * 5
	if score < 0 {
		score = 0
	}
	return score
}

// vmGenRecs produces actionable advice.
func vmGenRecs(s VMSummary) []string {
	var recs []string

	if s.MountFailErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d mount failure(s) — check node filesystem permissions, NFS/CIFS server availability, and CSI driver logs", s.MountFailErrors))
	}
	if s.AttachFailErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d attach/detach failure(s) — check cloud provider volume API limits, CSI driver health, and node disk attachments", s.AttachFailErrors))
	}
	if s.ProvisioningErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d provisioning error(s) — check StorageClass provisioner, storage backend capacity, and volume quota limits", s.ProvisioningErrors))
	}
	if s.TimeoutErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d timeout error(s) — storage backend may be overloaded or unreachable, check network connectivity", s.TimeoutErrors))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Volume health score is %d/100 — active storage issues affecting pod scheduling", s.HealthScore))
	}
	if s.StuckPods == 0 {
		recs = append(recs, "No volume mount/attach errors detected — healthy storage operation")
	}

	return recs
}

func vmTruncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func vmIssueRank(s string) int {
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
