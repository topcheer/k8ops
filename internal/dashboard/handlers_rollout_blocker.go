package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RolloutBlockerResult detects deployments with conditions blocking successful rollouts.
type RolloutBlockerResult struct {
	ScannedAt          time.Time             `json:"scannedAt"`
	Summary            RolloutBlockerSummary `json:"summary"`
	ByWorkload         []RolloutBlockerEntry `json:"byWorkload"`
	BlockedDeployments []RolloutBlockerEntry `json:"blockedDeployments"`
	HealthScore        int                   `json:"healthScore"`
	BlockedRollouts    []RolloutBlockerEntry `json:"blockedRollouts"`
	PodConditions      []string              `json:"podConditions"`
	Grade              string                `json:"grade"`
	Recommendations    []string              `json:"recommendations"`
}

type RolloutBlockerSummary struct {
	TotalDeployments     int `json:"totalDeployments"`
	Healthy              int `json:"healthy"`
	HealthyDeployments   int `json:"healthyDeployments"`
	Blocked              int `json:"blocked"`
	BlockedRollouts      int `json:"blockedRollouts"`
	PodsPending          int `json:"podsPending"`
	PodsCrashLooping     int `json:"podsCrashLooping"`
	PodsImagePullBackOff int `json:"podsImagePullBackOff"`
	ProgressDeadline     int `json:"progressDeadlineHit"`
	ReplicaMismatch      int `json:"replicaMismatch"`
	ImagePullBlocked     int `json:"imagePullBlocked"`
	InsufficientQuota    int `json:"insufficientQuota"`
}

type RolloutBlockerEntry struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Desired    int32    `json:"desiredReplicas"`
	Ready      int32    `json:"readyReplicas"`
	Updated    int32    `json:"updatedReplicas"`
	Conditions []string `json:"blockingConditions"`
	RiskLevel  string   `json:"riskLevel"`
	RootCause  string   `json:"rootCauseGuess"`
}

// handleRolloutBlockerDetect handles GET /api/deployment/rollout-blocker-detect
func (s *Server) handleRolloutBlockerDetect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RolloutBlockerResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := RolloutBlockerEntry{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.Desired = replicas
		entry.Ready = dep.Status.ReadyReplicas
		entry.Updated = dep.Status.UpdatedReplicas

		var blockers []string
		rootCause := ""

		// Check conditions
		for _, cond := range dep.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case appsv1.DeploymentProgressing:
				if cond.Reason == "ProgressDeadlineExceeded" {
					blockers = append(blockers, "ProgressDeadlineExceeded")
					rootCause = "rollout too slow - check image pull, resource limits, probes"
					result.Summary.ProgressDeadline++
				}
			case appsv1.DeploymentReplicaFailure:
				blockers = append(blockers, "ReplicaFailure: "+cond.Reason)
				rootCause = "pod creation failed - check quota, PVC, admission"
				if containsStr1876(cond.Reason, "quota") || containsStr1876(cond.Message, "quota") {
					result.Summary.InsufficientQuota++
				}
				if containsStr1876(cond.Reason, "ImagePull") || containsStr1876(cond.Message, "ImagePull") {
					result.Summary.ImagePullBlocked++
				}
			}
		}

		// Check replica mismatch
		if entry.Updated < entry.Desired {
			blockers = append(blockers, fmt.Sprintf("updated %d/%d", entry.Updated, entry.Desired))
			result.Summary.ReplicaMismatch++
			if rootCause == "" {
				rootCause = "rollout in progress or stuck"
			}
		}
		if entry.Ready < entry.Desired && entry.Desired > 0 {
			blockers = append(blockers, fmt.Sprintf("ready %d/%d", entry.Ready, entry.Desired))
			if rootCause == "" {
				rootCause = "pods not ready - check probes, crashes, resources"
			}
		}

		entry.Conditions = blockers
		entry.RootCause = rootCause
		if len(blockers) == 0 {
			result.Summary.Healthy++
			result.Summary.HealthyDeployments++
			entry.RiskLevel = "low"
		} else {
			result.Summary.Blocked++
			switch {
			case len(blockers) >= 3:
				entry.RiskLevel = "critical"
			case len(blockers) >= 2:
				entry.RiskLevel = "high"
			default:
				entry.RiskLevel = "medium"
			}
			result.BlockedDeployments = append(result.BlockedDeployments, entry)
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.BlockedDeployments, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return rank[result.BlockedDeployments[i].RiskLevel] < rank[result.BlockedDeployments[j].RiskLevel]
	})

	if result.Summary.TotalDeployments > 0 {
		result.HealthScore = result.Summary.Healthy * 100 / result.Summary.TotalDeployments
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("滚动更新阻塞检测: %d 部署, %d 健康, %d 阻塞, %d 超时, %d 副本不匹配",
			result.Summary.TotalDeployments, result.Summary.Healthy,
			result.Summary.Blocked, result.Summary.ProgressDeadline,
			result.Summary.ReplicaMismatch),
	}
	if result.Summary.Blocked > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署被阻塞", result.Summary.Blocked))
	}
	writeJSON(w, result)
}

func getOwnerKind(refs []metav1.OwnerReference) string {
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return ref.Kind
		}
	}
	if len(refs) > 0 {
		return refs[0].Kind
	}
	return ""
}

func getOwnerName(refs []metav1.OwnerReference) string {
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return ref.Name
		}
	}
	if len(refs) > 0 {
		return refs[0].Name
	}
	return ""
}

// Compatibility alias for existing test
func (s *Server) handleRolloutBlocker(w http.ResponseWriter, r *http.Request) {
	s.handleRolloutBlockerDetect(w, r)
}
