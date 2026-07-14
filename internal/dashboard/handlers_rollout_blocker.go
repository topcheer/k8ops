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

// RolloutBlockerResult is the deployment rollout blocker & pod condition audit.
type RolloutBlockerResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         RolloutBlockerSummary `json:"summary"`
	Deployments     []RolloutDeployment   `json:"deployments"`
	StatefulSets    []RolloutStatefulSet  `json:"statefulSets"`
	BlockedRollouts []RolloutBlocker      `json:"blockedRollouts"`
	PodConditions   []PodConditionIssue   `json:"podConditions"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// RolloutBlockerSummary aggregates rollout statistics.
type RolloutBlockerSummary struct {
	TotalDeployments     int `json:"totalDeployments"`
	TotalStatefulSets    int `json:"totalStatefulSets"`
	HealthyDeployments   int `json:"healthyDeployments"`
	UnhealthyDeployments int `json:"unhealthyDeployments"`
	BlockedRollouts      int `json:"blockedRollouts"`
	PodsPending          int `json:"podsPending"`
	PodsFailed           int `json:"podsFailed"`
	PodsWithConditions   int `json:"podsWithConditions"`
	PodsCrashLooping     int `json:"podsCrashLooping"`
	PodsImagePullBackOff int `json:"podsImagePullBackOff"`
	PodsOOMKilled        int `json:"podsOOMKilled"`
}

// RolloutDeployment describes a deployment's rollout status.
type RolloutDeployment struct {
	Name              string             `json:"name"`
	Namespace         string             `json:"namespace"`
	Replicas          int                `json:"replicas"`
	UpdatedReplicas   int32              `json:"updatedReplicas"`
	ReadyReplicas     int32              `json:"readyReplicas"`
	AvailableReplicas int32              `json:"availableReplicas"`
	Conditions        []RolloutCondEntry `json:"conditions"`
	Status            string             `json:"status"`
}

// RolloutStatefulSet describes a StatefulSet's rollout status.
type RolloutStatefulSet struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Replicas        int32  `json:"replicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	Status          string `json:"status"`
}

// RolloutCondition describes a deployment condition.
type RolloutCondEntry struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// RolloutBlocker describes a blocked rollout.
type RolloutBlocker struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Blocker   string `json:"blocker"`
	Severity  string `json:"severity"`
	Age       string `json:"age"`
}

// PodConditionIssue describes a pod condition issue blocking rollout.
type PodConditionIssue struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	OwnerKind string `json:"ownerKind"`
	OwnerName string `json:"ownerName"`
	Condition string `json:"condition"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// handleRolloutBlocker audits deployment rollout blockers & pod conditions.
// GET /api/deployment/rollout-blocker
func (s *Server) handleRolloutBlocker(w http.ResponseWriter, r *http.Request) {
	result := RolloutBlockerResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// 1. Check Deployments
	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			if systemNamespaces[dep.Namespace] {
				continue
			}
			result.Summary.TotalDeployments++

			desired := 0
			if dep.Spec.Replicas != nil {
				desired = int(*dep.Spec.Replicas)
			}

			status := "healthy"
			var conditions []RolloutCondEntry
			for _, c := range dep.Status.Conditions {
				cond := RolloutCondEntry{
					Type:    string(c.Type),
					Reason:  c.Reason,
					Message: c.Message,
					Status:  string(c.Status),
				}
				conditions = append(conditions, cond)

				if c.Status == "False" && (c.Type == appsv1.DeploymentProgressing || c.Type == appsv1.DeploymentAvailable) {
					status = "unhealthy"
					if c.Reason == "ProgressDeadlineExceeded" {
						status = "blocked"
						result.Summary.BlockedRollouts++
						result.BlockedRollouts = append(result.BlockedRollouts, RolloutBlocker{
							Namespace: dep.Namespace,
							Name:      dep.Name,
							Kind:      "Deployment",
							Blocker:   "ProgressDeadlineExceeded — rollout stuck",
							Severity:  "critical",
							Age:       "",
						})
					}
				}
			}

			if desired > 0 && dep.Status.UpdatedReplicas < int32(desired) {
				if status == "healthy" {
					status = "progressing"
				}
				if dep.Status.UpdatedReplicas == 0 && dep.Status.Replicas > 0 {
					status = "blocked"
					result.Summary.BlockedRollouts++
					result.BlockedRollouts = append(result.BlockedRollouts, RolloutBlocker{
						Namespace: dep.Namespace,
						Name:      dep.Name,
						Kind:      "Deployment",
						Blocker:   "No updated replicas — new rollout not progressing",
						Severity:  "high",
					})
				}
			}

			if dep.Status.ReadyReplicas < int32(desired) {
				if status == "healthy" {
					status = "degraded"
				}
				if dep.Status.ReadyReplicas == 0 && desired > 0 {
					status = "critical"
					result.BlockedRollouts = append(result.BlockedRollouts, RolloutBlocker{
						Namespace: dep.Namespace,
						Name:      dep.Name,
						Kind:      "Deployment",
						Blocker:   "No ready replicas — deployment unavailable",
						Severity:  "critical",
					})
				}
			}

			result.Deployments = append(result.Deployments, RolloutDeployment{
				Name:              dep.Name,
				Namespace:         dep.Namespace,
				Replicas:          desired,
				UpdatedReplicas:   dep.Status.UpdatedReplicas,
				ReadyReplicas:     dep.Status.ReadyReplicas,
				AvailableReplicas: dep.Status.AvailableReplicas,
				Conditions:        conditions,
				Status:            status,
			})

			if status == "healthy" {
				result.Summary.HealthyDeployments++
			} else {
				result.Summary.UnhealthyDeployments++
			}
		}
	}

	// 2. Check StatefulSets
	statefulsets, err := rc.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range statefulsets.Items {
			if systemNamespaces[sts.Namespace] {
				continue
			}
			result.Summary.TotalStatefulSets++

			desired := int32(0)
			if sts.Spec.Replicas != nil {
				desired = *sts.Spec.Replicas
			}

			status := "healthy"
			if sts.Status.ReadyReplicas < desired {
				status = "degraded"
				if sts.Status.ReadyReplicas == 0 && desired > 0 {
					status = "critical"
					result.BlockedRollouts = append(result.BlockedRollouts, RolloutBlocker{
						Namespace: sts.Namespace,
						Name:      sts.Name,
						Kind:      "StatefulSet",
						Blocker:   "No ready replicas — StatefulSet unavailable",
						Severity:  "critical",
					})
				}
			}
			if sts.Status.UpdatedReplicas < desired {
				if status == "healthy" {
					status = "progressing"
				}
			}

			result.StatefulSets = append(result.StatefulSets, RolloutStatefulSet{
				Name:            sts.Name,
				Namespace:       sts.Namespace,
				Replicas:        desired,
				UpdatedReplicas: sts.Status.UpdatedReplicas,
				ReadyReplicas:   sts.Status.ReadyReplicas,
				Status:          status,
			})
		}
	}

	// 3. Check pod conditions for blockers
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}

			// Check pod phase
			switch pod.Status.Phase {
			case corev1.PodPending:
				result.Summary.PodsPending++
				result.PodConditions = append(result.PodConditions, PodConditionIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: getOwnerKind(pod.OwnerReferences),
					OwnerName: getOwnerName(pod.OwnerReferences),
					Condition: string(pod.Status.Phase),
					Reason:    getPodConditionReason(pod.Status.Conditions),
					Severity:  "medium",
				})
			case corev1.PodFailed:
				result.Summary.PodsFailed++
				result.PodConditions = append(result.PodConditions, PodConditionIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: getOwnerKind(pod.OwnerReferences),
					OwnerName: getOwnerName(pod.OwnerReferences),
					Condition: string(pod.Status.Phase),
					Reason:    "Pod failed",
					Severity:  "high",
				})
			}

			// Check container status for crash loops
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil {
					reason := cs.State.Waiting.Reason
					if reason == "CrashLoopBackOff" {
						result.Summary.PodsCrashLooping++
						result.PodConditions = append(result.PodConditions, PodConditionIssue{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							OwnerKind: getOwnerKind(pod.OwnerReferences),
							OwnerName: getOwnerName(pod.OwnerReferences),
							Condition: "CrashLoopBackOff",
							Reason:    cs.State.Waiting.Message,
							Severity:  "critical",
						})
					} else if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
						result.Summary.PodsImagePullBackOff++
						result.PodConditions = append(result.PodConditions, PodConditionIssue{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							OwnerKind: getOwnerKind(pod.OwnerReferences),
							OwnerName: getOwnerName(pod.OwnerReferences),
							Condition: "ImagePullBackOff",
							Reason:    cs.State.Waiting.Message,
							Severity:  "high",
						})
					}
				}
				if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					result.Summary.PodsOOMKilled++
					result.PodConditions = append(result.PodConditions, PodConditionIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						OwnerKind: getOwnerKind(pod.OwnerReferences),
						OwnerName: getOwnerName(pod.OwnerReferences),
						Condition: "OOMKilled",
						Reason:    fmt.Sprintf("Container %s was OOMKilled (restart count: %d)", cs.Name, cs.RestartCount),
						Severity:  "high",
					})
				}
			}
		}
	}

	result.Summary.PodsWithConditions = len(result.PodConditions)

	// Sort results
	sort.Slice(result.BlockedRollouts, func(i, j int) bool {
		return result.BlockedRollouts[i].Severity > result.BlockedRollouts[j].Severity
	})
	sort.Slice(result.PodConditions, func(i, j int) bool {
		return result.PodConditions[i].Severity > result.PodConditions[j].Severity
	})
	sort.Slice(result.Deployments, func(i, j int) bool {
		return result.Deployments[i].Status > result.Deployments[j].Status
	})

	// Recommendations
	if result.Summary.BlockedRollouts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d rollouts are blocked — check ProgressDeadlineExceeded and rollout conditions", result.Summary.BlockedRollouts))
	}
	if result.Summary.PodsCrashLooping > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods in CrashLoopBackOff — check container logs and application errors", result.Summary.PodsCrashLooping))
	}
	if result.Summary.PodsImagePullBackOff > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods in ImagePullBackOff — verify image names and pull secrets", result.Summary.PodsImagePullBackOff))
	}
	if result.Summary.PodsOOMKilled > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods OOMKilled — increase memory limits or fix memory leaks", result.Summary.PodsOOMKilled))
	}
	if result.Summary.PodsPending > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods in Pending state — check resource requests, node capacity, and scheduling constraints", result.Summary.PodsPending))
	}
	if result.Summary.UnhealthyDeployments > 0 && result.Summary.BlockedRollouts == 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments are degraded but not blocked — monitor for progress", result.Summary.UnhealthyDeployments))
	}

	// Health score
	score := 100
	score -= result.Summary.BlockedRollouts * 15
	score -= result.Summary.PodsCrashLooping * 10
	score -= result.Summary.PodsImagePullBackOff * 8
	score -= result.Summary.PodsOOMKilled * 5
	score -= result.Summary.PodsPending * 3
	score -= result.Summary.PodsFailed * 5
	if result.Summary.UnhealthyDeployments > 0 {
		score -= result.Summary.UnhealthyDeployments * 2
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

func getOwnerKind(refs []metav1.OwnerReference) string {
	if len(refs) == 0 {
		return ""
	}
	return refs[0].Kind
}

func getOwnerName(refs []metav1.OwnerReference) string {
	if len(refs) == 0 {
		return ""
	}
	return refs[0].Name
}

func getPodConditionReason(conditions []corev1.PodCondition) string {
	for _, c := range conditions {
		if c.Type == corev1.PodScheduled && c.Status == "False" {
			return fmt.Sprintf("Unscheduled: %s", c.Reason)
		}
		if c.Type == corev1.PodReady && c.Status == "False" {
			return fmt.Sprintf("NotReady: %s", c.Reason)
		}
	}
	return ""
}

var _ = strings.Contains
