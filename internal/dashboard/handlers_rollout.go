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

// RolloutStatus describes the rollout state of a workload.
type RolloutStatus string

const (
	RolloutComplete   RolloutStatus = "complete"    // all replicas updated and ready
	RolloutInProgress RolloutStatus = "in-progress" // rollout is actively happening
	RolloutStalled    RolloutStatus = "stalled"     // rollout not progressing (deadline exceeded or stuck)
	RolloutDegraded   RolloutStatus = "degraded"    // some replicas unavailable
	RolloutPaused     RolloutStatus = "paused"      // deployment explicitly paused
	RolloutFailed     RolloutStatus = "failed"      // rollout has failed (ProgressDeadlineExceeded)
	RolloutScaledZero RolloutStatus = "scaled-to-zero"
)

// WorkloadRollout describes the rollout health of a single workload.
type WorkloadRollout struct {
	Kind                string             `json:"kind"` // Deployment, StatefulSet, DaemonSet
	Name                string             `json:"name"`
	Namespace           string             `json:"namespace"`
	Status              RolloutStatus      `json:"status"`
	DesiredReplicas     int32              `json:"desiredReplicas"`
	ReadyReplicas       int32              `json:"readyReplicas"`
	UpdatedReplicas     int32              `json:"updatedReplicas"`
	AvailableReplicas   int32              `json:"availableReplicas,omitempty"`
	UnavailableReplicas int32              `json:"unavailableReplicas,omitempty"`
	UpdateStrategy      string             `json:"updateStrategy"`
	TemplateHash        string             `json:"templateHash,omitempty"`
	Images              []string           `json:"images"`
	AgeHours            float64            `json:"ageHours"`
	SinceUpdateHours    float64            `json:"sinceUpdateHours"`
	Conditions          []RolloutCondition `json:"conditions,omitempty"`
	ProgressReason      string             `json:"progressReason,omitempty"`
	ProgressMessage     string             `json:"progressMessage,omitempty"`
	Issues              []string           `json:"issues,omitempty"`
}

// RolloutCondition mirrors a workload condition relevant to rollout health.
type RolloutCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// RolloutResult is the full scan output.
type RolloutResult struct {
	ScannedAt time.Time         `json:"scannedAt"`
	Summary   RolloutSummary    `json:"summary"`
	Workloads []WorkloadRollout `json:"workloads"`
}

// RolloutSummary aggregates rollout statistics.
type RolloutSummary struct {
	Total        int `json:"total"`
	Deployments  int `json:"deployments"`
	StatefulSets int `json:"statefulSets"`
	DaemonSets   int `json:"daemonSets"`
	Complete     int `json:"complete"`
	InProgress   int `json:"inProgress"`
	Stalled      int `json:"stalled"`
	Degraded     int `json:"degraded"`
	Paused       int `json:"paused"`
	Failed       int `json:"failed"`
	ScaledZero   int `json:"scaledToZero"`
	WithIssues   int `json:"withIssues"`
}

// handleRolloutStatus scans all Deployments, StatefulSets, and DaemonSets for rollout health.
// GET /api/deployments/rollout?namespace=xxx&status=degraded
func (s *Server) handleRolloutStatus(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ns := r.URL.Query().Get("namespace")
	statusFilter := r.URL.Query().Get("status")

	result := RolloutResult{
		ScannedAt: time.Now(),
	}

	// --- Deployments ---
	depList, err := rc.clientset.AppsV1().Deployments(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range depList.Items {
		wr := analyzeDeploymentRollout(&depList.Items[i])
		result.Workloads = append(result.Workloads, wr)
	}

	// --- StatefulSets ---
	stsList, err := rc.clientset.AppsV1().StatefulSets(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range stsList.Items {
		wr := analyzeStatefulSetRollout(&stsList.Items[i])
		result.Workloads = append(result.Workloads, wr)
	}

	// --- DaemonSets ---
	dsList, err := rc.clientset.AppsV1().DaemonSets(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range dsList.Items {
		wr := analyzeDaemonSetRollout(&dsList.Items[i])
		result.Workloads = append(result.Workloads, wr)
	}

	// Update summary
	for _, wr := range result.Workloads {
		result.Summary.Total++
		switch wr.Kind {
		case "Deployment":
			result.Summary.Deployments++
		case "StatefulSet":
			result.Summary.StatefulSets++
		case "DaemonSet":
			result.Summary.DaemonSets++
		}
		switch wr.Status {
		case RolloutComplete:
			result.Summary.Complete++
		case RolloutInProgress:
			result.Summary.InProgress++
		case RolloutStalled:
			result.Summary.Stalled++
		case RolloutDegraded:
			result.Summary.Degraded++
		case RolloutPaused:
			result.Summary.Paused++
		case RolloutFailed:
			result.Summary.Failed++
		case RolloutScaledZero:
			result.Summary.ScaledZero++
		}
		if len(wr.Issues) > 0 {
			result.Summary.WithIssues++
		}
	}

	// Sort: failed/stalled/degraded first, then in-progress, then by age
	sort.Slice(result.Workloads, func(i, j int) bool {
		si := rolloutSeverity(result.Workloads[i].Status)
		sj := rolloutSeverity(result.Workloads[j].Status)
		if si != sj {
			return si < sj
		}
		return result.Workloads[i].Name < result.Workloads[j].Name
	})

	// Apply status filter
	if statusFilter != "" {
		filtered := make([]WorkloadRollout, 0, len(result.Workloads))
		for _, wr := range result.Workloads {
			if string(wr.Status) == statusFilter {
				filtered = append(filtered, wr)
			}
		}
		result.Workloads = filtered
	}

	writeJSON(w, result)
}

func rolloutSeverity(s RolloutStatus) int {
	switch s {
	case RolloutFailed:
		return 0
	case RolloutStalled:
		return 1
	case RolloutDegraded:
		return 2
	case RolloutInProgress:
		return 3
	case RolloutPaused:
		return 4
	case RolloutScaledZero:
		return 5
	case RolloutComplete:
		return 6
	}
	return 9
}

// analyzeDeploymentRollout evaluates the rollout state of a Deployment.
func analyzeDeploymentRollout(dep *appsv1.Deployment) WorkloadRollout {
	wr := WorkloadRollout{
		Kind:                "Deployment",
		Name:                dep.Name,
		Namespace:           dep.Namespace,
		DesiredReplicas:     *dep.Spec.Replicas,
		ReadyReplicas:       dep.Status.ReadyReplicas,
		UpdatedReplicas:     dep.Status.UpdatedReplicas,
		AvailableReplicas:   dep.Status.AvailableReplicas,
		UnavailableReplicas: dep.Status.UnavailableReplicas,
		UpdateStrategy:      string(dep.Spec.Strategy.Type),
		AgeHours:            hoursSince(dep.CreationTimestamp),
		Images:              extractImages(dep.Spec.Template.Spec.Containers),
	}

	// Template hash
	if h, ok := dep.Labels["pod-template-hash"]; ok {
		wr.TemplateHash = h
	}

	// Since update (based on conditions or creation)
	wr.SinceUpdateHours = wr.AgeHours
	for _, c := range dep.Status.Conditions {
		cond := RolloutCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		}
		wr.Conditions = append(wr.Conditions, cond)

		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionTrue {
			wr.ProgressReason = c.Reason
			wr.ProgressMessage = c.Message
			if c.Reason == "NewReplicaSetCreated" || c.Reason == "ReplicaSetUpdated" {
				if !c.LastUpdateTime.IsZero() {
					wr.SinceUpdateHours = time.Since(c.LastUpdateTime.Time).Hours()
				}
			}
		}

		// Detect failures
		if c.Type == appsv1.DeploymentProgressing && c.Reason == "ProgressDeadlineExceeded" {
			wr.Status = RolloutFailed
			wr.Issues = append(wr.Issues, "Rollout exceeded progress deadline — pods may be failing to start")
		}
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			if wr.Status != RolloutFailed {
				wr.Status = RolloutDegraded
			}
			wr.Issues = append(wr.Issues, fmt.Sprintf("Replica failure: %s", c.Reason))
		}
	}

	// Determine status if not already set by condition analysis
	if wr.Status == "" {
		// Check paused
		if dep.Annotations["deployment.kubernetes.io/paused"] == "true" {
			wr.Status = RolloutPaused
		} else if wr.DesiredReplicas == 0 {
			wr.Status = RolloutScaledZero
		} else if wr.UpdatedReplicas < wr.DesiredReplicas {
			wr.Status = RolloutInProgress
			wr.Issues = append(wr.Issues, fmt.Sprintf("Only %d/%d replicas updated", wr.UpdatedReplicas, wr.DesiredReplicas))
		} else if wr.AvailableReplicas < wr.DesiredReplicas {
			wr.Status = RolloutDegraded
			wr.Issues = append(wr.Issues, fmt.Sprintf("Only %d/%d replicas available", wr.AvailableReplicas, wr.DesiredReplicas))
		} else if wr.ReadyReplicas == wr.DesiredReplicas && wr.UpdatedReplicas == wr.DesiredReplicas {
			wr.Status = RolloutComplete
		} else {
			wr.Status = RolloutInProgress
		}
	}

	// Detect stalled rollout: generation mismatch means controller hasn't observed latest spec
	if dep.Generation > dep.Status.ObservedGeneration {
		if wr.Status == RolloutInProgress {
			wr.Status = RolloutStalled
		}
		wr.Issues = append(wr.Issues, "Controller has not observed latest spec generation — rollout may be stuck")
	}

	return wr
}

// analyzeStatefulSetRollout evaluates the rollout state of a StatefulSet.
func analyzeStatefulSetRollout(sts *appsv1.StatefulSet) WorkloadRollout {
	desired := int32(0)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}

	wr := WorkloadRollout{
		Kind:              "StatefulSet",
		Name:              sts.Name,
		Namespace:         sts.Namespace,
		DesiredReplicas:   desired,
		ReadyReplicas:     sts.Status.ReadyReplicas,
		UpdatedReplicas:   sts.Status.UpdatedReplicas,
		AvailableReplicas: sts.Status.AvailableReplicas,
		UpdateStrategy:    string(sts.Spec.UpdateStrategy.Type),
		TemplateHash:      sts.Status.CurrentRevision,
		AgeHours:          hoursSince(sts.CreationTimestamp),
		Images:            extractImages(sts.Spec.Template.Spec.Containers),
	}

	if sts.Status.CurrentRevision != sts.Status.UpdateRevision && sts.Status.UpdateRevision != "" {
		wr.SinceUpdateHours = wr.AgeHours // approximate
	}

	// Determine status
	if desired == 0 {
		wr.Status = RolloutScaledZero
	} else if sts.Status.UpdateRevision != sts.Status.CurrentRevision && sts.Status.UpdatedReplicas < desired {
		wr.Status = RolloutInProgress
		wr.Issues = append(wr.Issues, fmt.Sprintf("Rolling update: %d/%d pods on new revision", sts.Status.UpdatedReplicas, desired))
		if wr.TemplateHash != "" {
			wr.TemplateHash = fmt.Sprintf("%s → %s", sts.Status.CurrentRevision, sts.Status.UpdateRevision)
		}
	} else if sts.Status.ReadyReplicas < desired {
		wr.Status = RolloutDegraded
		wr.Issues = append(wr.Issues, fmt.Sprintf("Only %d/%d replicas ready", sts.Status.ReadyReplicas, desired))
	} else if sts.Status.ReadyReplicas == desired && sts.Status.UpdatedReplicas == desired {
		wr.Status = RolloutComplete
	} else {
		wr.Status = RolloutInProgress
	}

	// OnDelete strategy means updates require manual pod deletion
	if sts.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType && sts.Status.UpdateRevision != sts.Status.CurrentRevision {
		wr.Issues = append(wr.Issues, "Using OnDelete strategy — pods must be manually deleted to trigger update")
		if wr.Status == RolloutComplete {
			wr.Status = RolloutInProgress
		}
	}

	return wr
}

// analyzeDaemonSetRollout evaluates the rollout state of a DaemonSet.
func analyzeDaemonSetRollout(ds *appsv1.DaemonSet) WorkloadRollout {
	wr := WorkloadRollout{
		Kind:                "DaemonSet",
		Name:                ds.Name,
		Namespace:           ds.Namespace,
		DesiredReplicas:     ds.Status.DesiredNumberScheduled,
		ReadyReplicas:       ds.Status.NumberReady,
		UpdatedReplicas:     ds.Status.UpdatedNumberScheduled,
		AvailableReplicas:   ds.Status.NumberAvailable,
		UnavailableReplicas: ds.Status.NumberUnavailable,
		UpdateStrategy:      string(ds.Spec.UpdateStrategy.Type),
		AgeHours:            hoursSince(ds.CreationTimestamp),
		Images:              extractImages(ds.Spec.Template.Spec.Containers),
	}

	desired := ds.Status.DesiredNumberScheduled

	if desired == 0 {
		wr.Status = RolloutScaledZero
	} else if ds.Status.NumberUnavailable > 0 && ds.Status.UpdatedNumberScheduled >= desired {
		wr.Status = RolloutDegraded
		wr.Issues = append(wr.Issues, fmt.Sprintf("%d pods unavailable", ds.Status.NumberUnavailable))
	} else if ds.Status.UpdatedNumberScheduled < desired {
		wr.Status = RolloutInProgress
		wr.Issues = append(wr.Issues, fmt.Sprintf("Rolling update: %d/%d pods updated", ds.Status.UpdatedNumberScheduled, desired))
	} else if ds.Status.NumberReady < desired {
		wr.Status = RolloutDegraded
		wr.Issues = append(wr.Issues, fmt.Sprintf("Only %d/%d pods ready", ds.Status.NumberReady, desired))
	} else if ds.Status.NumberReady == desired && ds.Status.UpdatedNumberScheduled == desired {
		wr.Status = RolloutComplete
	} else {
		wr.Status = RolloutInProgress
	}

	return wr
}

// extractImages returns unique image references from container specs.
func extractImages(containers []corev1.Container) []string {
	seen := make(map[string]bool)
	var images []string
	for _, c := range containers {
		if !seen[c.Image] {
			seen[c.Image] = true
			images = append(images, c.Image)
		}
	}
	return images
}

// hoursSince computes hours since a metav1.Time.
func hoursSince(t metav1.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return time.Since(t.Time).Hours()
}

// RolloutSeverityLabel returns a human-readable severity label.
func RolloutSeverityLabel(s RolloutStatus) string {
	switch s {
	case RolloutFailed:
		return "critical"
	case RolloutStalled:
		return "critical"
	case RolloutDegraded:
		return "warning"
	case RolloutInProgress:
		return "info"
	case RolloutPaused:
		return "warning"
	case RolloutScaledZero:
		return "info"
	case RolloutComplete:
		return "ok"
	}
	return "unknown"
}
