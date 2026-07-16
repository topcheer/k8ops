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

// TriageResult is the AIOps incident triage & remediation action plan engine.
// It correlates signals across multiple dimensions and produces prioritized actions.
type TriageResult struct {
	ScannedAt    time.Time        `json:"scannedAt"`
	Summary      TriageSummary    `json:"summary"`
	Priority     string           `json:"priority"` // P0-critical, P1-urgent, P2-important, P3-routine
	Incidents    []TriagedIncident `json:"incidents"`
	ActionPlan   []ActionItem     `json:"actionPlan"`
	QuickWins    []ActionItem     `json:"quickWins"`    // low-effort high-impact
	LongTermFixes []ActionItem    `json:"longTermFixes"` // strategic improvements
	HealthScore  int              `json:"healthScore"`
}

// TriageSummary aggregates triage statistics.
type TriageSummary struct {
	TotalSignals    int `json:"totalSignals"`
	P0Incidents     int `json:"p0Incidents"` // critical, fix immediately
	P1Incidents     int `json:"p1Incidents"` // urgent, fix within 24h
	P2Incidents     int `json:"p2Incidents"` // important, fix within 1 week
	P3Incidents     int `json:"p3Incidents"` // routine, plan when convenient
	WorkloadsAffected int `json:"workloadsAffected"`
	QuickWinCount   int `json:"quickWinCount"`
}

// TriagedIncident describes a correlated incident with root cause analysis.
type TriagedIncident struct {
	ID           string   `json:"id"`
	Priority     string   `json:"priority"` // P0, P1, P2, P3
	Title        string   `json:"title"`
	Category     string   `json:"category"` // crash-loop, resource-pressure, security, capacity, config, availability
	Severity     string   `json:"severity"` // critical, high, medium, low
	Workloads    []string `json:"workloads"`
	RootCause    string   `json:"rootCause"`
	Evidence     []string `json:"evidence"`
	ImpactRadius string   `json:"impactRadius"` // cluster, namespace, workload
	DetectedAt   string   `json:"detectedAt"`
	Status       string   `json:"status"` // active, investigating, resolved
}

// ActionItem describes a specific remediation action.
type ActionItem struct {
	Priority    string `json:"priority"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`  // kubectl command to execute
	Effort      string `json:"effort"`             // quick (<5min), moderate (<1h), significant (>1h)
	Impact      string `json:"impact"`             // high, medium, low
	IncidentIDs []string `json:"incidentIDs,omitempty"`
}

// handleTriage generates an AIOps incident triage and remediation action plan.
// GET /api/operations/triage
func (s *Server) handleTriage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TriageResult{ScannedAt: time.Now()}

	// Collect cluster state
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 500})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var incidents []TriagedIncident
	var actions []ActionItem
	totalSignals := 0

	// ========================================
	// CORRELATION 1: Crash Loop Clusters
	// ========================================
	crashPods := map[string][]string{} // namespace -> pods
	if pods != nil {
		for _, pod := range pods.Items {
			totalRestarts := 0
			for _, cs := range pod.Status.ContainerStatuses {
				totalRestarts += int(cs.RestartCount)
			}
			if totalRestarts > 5 {
				crashPods[pod.Namespace] = append(crashPods[pod.Namespace], pod.Name)
				totalSignals++
			}
		}
	}

	for ns, podList := range crashPods {
		if len(podList) < 3 {
			continue // Individual crash loops handled below
		}
		priority := "P1"
		severity := "high"
		if len(podList) > 10 {
			priority = "P0"
			severity = "critical"
		}
		incidents = append(incidents, TriagedIncident{
			ID:        fmt.Sprintf("crash-cluster-%s", ns),
			Priority:  priority,
			Category:  "crash-loop",
			Severity:  severity,
			Title:     fmt.Sprintf("%d pods crash-looping in %s", len(podList), ns),
			Workloads: podList[:min(10, len(podList))],
			RootCause: "Multiple pods restarting simultaneously — likely shared dependency failure, config issue, or resource exhaustion",
			ImpactRadius: "namespace",
			Status:    "active",
		})
		actions = append(actions, ActionItem{
			Priority: priority, Category: "crash-loop",
			Title:       fmt.Sprintf("Investigate crash cluster in %s", ns),
			Description: "Check shared ConfigMaps/Secrets, recent deployments, and node resource availability",
			Command:     fmt.Sprintf("kubectl get events -n %s --sort-by=.lastTimestamp | tail -20", ns),
			Effort:      "moderate", Impact: "high",
		})
	}

	// ========================================
	// CORRELATION 2: Node Pressure + Pending Pods
	// ========================================
	if nodes != nil && pods != nil {
		pressuredNodes := []string{}
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type != corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					pressuredNodes = append(pressuredNodes, node.Name)
					totalSignals++
				}
			}
		}

		pendingPods := []string{}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodPending {
				pendingPods = append(pendingPods, pod.Name)
				totalSignals++
			}
		}

		if len(pressuredNodes) > 0 && len(pendingPods) > 3 {
			incidents = append(incidents, TriagedIncident{
				ID:           "node-pressure-pending",
				Priority:     "P0",
				Category:     "resource-pressure",
				Severity:     "critical",
				Title:        fmt.Sprintf("%d nodes under pressure with %d pods pending", len(pressuredNodes), len(pendingPods)),
				Workloads:    pendingPods[:min(5, len(pendingPods))],
				RootCause:    "Node resource exhaustion preventing pod scheduling — cascading failure risk",
				Evidence:     []string{fmt.Sprintf("Pressured nodes: %s", strings.Join(pressuredNodes, ", "))},
				ImpactRadius: "cluster",
				Status:       "active",
			})
			actions = append(actions, ActionItem{
				Priority: "P0", Category: "resource-pressure",
				Title:       "Address node resource pressure immediately",
				Description: "Cordon pressured nodes, clean up completed/stale pods, or add capacity",
				Command:     fmt.Sprintf("kubectl describe node %s | grep -A10 Conditions", pressuredNodes[0]),
				Effort:      "quick", Impact: "high",
			})
		}
	}

	// ========================================
	// CORRELATION 3: Failed Deployments (ImagePullBackOff)
	// ========================================
	if pods != nil {
		failedImages := map[string][]string{} // image -> pods
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil {
					reason := cs.State.Waiting.Reason
					if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
						img := extractImageName(pod.Spec.Containers, cs.Name)
						failedImages[img] = append(failedImages[img], fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
						totalSignals++
					}
				}
			}
		}

		for img, podList := range failedImages {
			if len(podList) == 0 {
				continue
			}
			incidents = append(incidents, TriagedIncident{
				ID:        fmt.Sprintf("image-pull-%s", sanitizeID(img)),
				Priority:  "P1",
				Category:  "image-failure",
				Severity:  "high",
				Title:     fmt.Sprintf("Image pull failures for %s (%d pods)", truncateStr(img, 50), len(podList)),
				Workloads: podList[:min(5, len(podList))],
				RootCause: "Image not found, registry authentication failure, or network issue",
				ImpactRadius: "namespace",
				Status:    "active",
			})
			actions = append(actions, ActionItem{
				Priority: "P1", Category: "image-failure",
				Title:       fmt.Sprintf("Fix image pull for %s", truncateStr(img, 40)),
				Description: "Verify image exists in registry, check imagePullSecrets, validate network connectivity",
				Command:     fmt.Sprintf("kubectl describe pod %s | grep -A5 Events", strings.Split(podList[0], "/")[1]),
				Effort:      "quick", Impact: "high",
			})
		}
	}

	// ========================================
	// CORRELATION 4: Deployment Rollout Stuck
	// ========================================
	if deployments != nil {
		for _, d := range deployments.Items {
			if isSystemNSReliability(d.Namespace) {
				continue
			}
			if d.Spec.Replicas != nil && d.Status.UpdatedReplicas < *d.Spec.Replicas {
				if d.Status.Replicas > d.Status.AvailableReplicas {
					totalSignals++
					incidents = append(incidents, TriagedIncident{
						ID:        fmt.Sprintf("rollout-stuck-%s-%s", d.Namespace, d.Name),
						Priority:  "P2",
						Category:  "rollout",
						Severity:  "medium",
						Title:     fmt.Sprintf("Deployment %s/%s rollout stuck (%d/%d updated, %d available)",
							d.Namespace, d.Name, d.Status.UpdatedReplicas, *d.Spec.Replicas, d.Status.AvailableReplicas),
						Workloads:    []string{fmt.Sprintf("%s/%s", d.Namespace, d.Name)},
						RootCause:    "New pods failing health checks, insufficient resources, or crash in new version",
						ImpactRadius: "workload",
						Status:       "investigating",
					})
					actions = append(actions, ActionItem{
						Priority: "P2", Category: "rollout",
						Title:       fmt.Sprintf("Diagnose stuck rollout for %s/%s", d.Namespace, d.Name),
						Description: "Check new pod logs and events for crash or scheduling failure",
						Command:     fmt.Sprintf("kubectl rollout status deployment/%s -n %s", d.Name, d.Namespace),
						Effort:      "moderate", Impact: "medium",
					})
				}
			}
		}
	}

	// ========================================
	// CORRELATION 5: Event Burst Detection
	// ========================================
	if events != nil {
		nsEventCount := map[string]int{}
		warningCount := 0
		for _, ev := range events.Items {
			if ev.Type == "Warning" {
				nsEventCount[ev.Namespace]++
				warningCount++
			}
		}
		totalSignals += warningCount

		for ns, count := range nsEventCount {
			if count > 20 {
				incidents = append(incidents, TriagedIncident{
					ID:           fmt.Sprintf("event-burst-%s", ns),
					Priority:     "P2",
					Category:     "event-storm",
					Severity:     "medium",
					Title:        fmt.Sprintf("Event storm in %s (%d warnings)", ns, count),
					Workloads:    []string{},
					RootCause:    "Excessive warning events indicate ongoing issues — check for failing probes, scheduling errors, or controller conflicts",
					ImpactRadius: "namespace",
					Status:       "active",
				})
				actions = append(actions, ActionItem{
					Priority: "P2", Category: "event-storm",
					Title:       fmt.Sprintf("Investigate event storm in %s", ns),
					Description: "Review warning events for patterns",
					Command:     fmt.Sprintf("kubectl get events -n %s --field-selector type=Warning --sort-by=.lastTimestamp", ns),
					Effort:      "quick", Impact: "medium",
				})
			}
		}
	}

	// ========================================
	// Sort incidents by priority
	// ========================================
	sort.Slice(incidents, func(i, j int) bool {
		prioOrder := map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}
		return prioOrder[incidents[i].Priority] < prioOrder[incidents[j].Priority]
	})

	// ========================================
	// Build action plan
	// ========================================
	sort.Slice(actions, func(i, j int) bool {
		prioOrder := map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}
		return prioOrder[actions[i].Priority] < prioOrder[actions[j].Priority]
	})

	// Split into quick wins and long-term fixes
	var quickWins, longTerm []ActionItem
	for _, a := range actions {
		if a.Effort == "quick" && a.Impact == "high" {
			quickWins = append(quickWins, a)
		} else if a.Effort == "significant" {
			longTerm = append(longTerm, a)
		}
	}

	// Add proactive long-term fixes
	longTerm = append(longTerm, generateProactiveActions(nodes, pods)...)

	// ========================================
	// Compute summary
	// ========================================
	summary := TriageSummary{TotalSignals: totalSignals}
	for _, inc := range incidents {
		switch inc.Priority {
		case "P0":
			summary.P0Incidents++
		case "P1":
			summary.P1Incidents++
		case "P2":
			summary.P2Incidents++
		case "P3":
			summary.P3Incidents++
		}
	}
	summary.WorkloadsAffected = countAffectedWorkloads(incidents)
	summary.QuickWinCount = len(quickWins)

	// Determine overall priority
	switch {
	case summary.P0Incidents > 0:
		result.Priority = "P0-critical"
	case summary.P1Incidents > 0:
		result.Priority = "P1-urgent"
	case summary.P2Incidents > 0:
		result.Priority = "P2-important"
	default:
		result.Priority = "P3-routine"
	}

	// Health score
	score := 100
	score -= summary.P0Incidents * 30
	score -= summary.P1Incidents * 15
	score -= summary.P2Incidents * 5
	score = clampScore(score)

	result.Summary = summary
	result.Incidents = incidents
	result.ActionPlan = actions
	result.QuickWins = quickWins
	result.LongTermFixes = longTerm
	result.HealthScore = score

	writeJSON(w, result)
}

// extractImageName gets the image for a specific container.
func extractImageName(containers []corev1.Container, name string) string {
	for _, c := range containers {
		if c.Name == name {
			return c.Image
		}
	}
	return "unknown"
}

// sanitizeID creates a safe ID string from an image name.
func sanitizeID(s string) string {
	return strings.NewReplacer("/", "-", ":", "-", "@", "-", ".", "-").Replace(s)
}

// truncate shortens a string to maxLen with ellipsis.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// countAffectedWorkloads counts unique workloads across incidents.
func countAffectedWorkloads(incidents []TriagedIncident) int {
	seen := map[string]bool{}
	for _, inc := range incidents {
		for _, wl := range inc.Workloads {
			seen[wl] = true
		}
	}
	return len(seen)
}

// generateProactiveActions produces strategic improvement suggestions.
func generateProactiveActions(nodes *corev1.NodeList, pods *corev1.PodList) []ActionItem {
	var actions []ActionItem

	if nodes != nil && pods != nil {
		// Check for single-replica deployments (no HA)
		singleReplicaNS := map[string]int{}
		if pods != nil {
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					ownerKind := ""
					for _, ref := range pod.OwnerReferences {
						ownerKind = ref.Kind
					}
					if ownerKind == "ReplicaSet" || ownerKind == "" {
						// Approximate: count pods by deployment label
						if app, ok := pod.Labels["app"]; ok && app != "" {
							key := fmt.Sprintf("%s/%s", pod.Namespace, app)
							singleReplicaNS[key]++
						}
					}
				}
			}
		}
		singleApps := 0
		for _, count := range singleReplicaNS {
			if count == 1 {
				singleApps++
			}
		}
		if singleApps > 3 {
			actions = append(actions, ActionItem{
				Priority:    "P3",
				Category:    "availability",
				Title:       fmt.Sprintf("Scale %d single-replica workloads to >=2 for HA", singleApps),
				Description: "Single-replica deployments are SPOF — increase replicas and add PDB",
				Effort:      "moderate",
				Impact:      "high",
			})
		}
	}

	// Add monitoring recommendation if many signals detected
	actions = append(actions, ActionItem{
		Priority:    "P3",
		Category:    "observability",
		Title:       "Implement structured alerting from event patterns",
		Description: "Create Prometheus alerting rules based on recurring event patterns detected by triage",
		Effort:      "significant",
		Impact:      "medium",
	})

	return actions
}
