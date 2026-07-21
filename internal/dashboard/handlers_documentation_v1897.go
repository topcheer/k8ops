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

// ============================================================
// v18.97 — Documentation Dimension
// 1. Resource Ownership Registry
// 2. Release Note Generator
// 3. Incident Postmortem Template
// ============================================================

// ---------------------------------------------------------------
// 1. Resource Ownership Registry — maps ownership & accountability
// ---------------------------------------------------------------

type OwnershipRegistryResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         OwnershipRegSummary  `json:"summary"`
	ByTeam          []TeamOwnership1897  `json:"byTeam"`
	Unowned         []UnownedResource    `json:"unowned"`
	ByNamespace     []NamespaceOwnership `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type OwnershipRegSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	WithOwner       int `json:"withOwner"`
	WithoutOwner    int `json:"withoutOwner"`
	TeamsIdentified int `json:"teamsIdentified"`
	WithContactInfo int `json:"withContactInfo"`
	CriticalUnowned int `json:"criticalUnowned"`
}

type TeamOwnership1897 struct {
	Team           string `json:"team"`
	WorkloadCount  int    `json:"workloadCount"`
	NamespaceCount int    `json:"namespaceCount"`
	Replicas       int    `json:"replicas"`
	HasContactInfo bool   `json:"hasContactInfo"`
	RiskLevel      string `json:"riskLevel"`
}

type UnownedResource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Replicas  int32  `json:"replicas"`
	RiskLevel string `json:"riskLevel"`
}

type NamespaceOwnership struct {
	Namespace      string `json:"namespace"`
	WorkloadCount  int    `json:"workloadCount"`
	WithOwnerLabel int    `json:"withOwnerLabel"`
	TeamLabel      string `json:"teamLabel"`
	RiskLevel      string `json:"riskLevel"`
}

func (s *Server) handleOwnershipRegistry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := OwnershipRegistryResult{ScannedAt: time.Now()}

	// Collect ownership labels from deployments
	teamMap := map[string]*TeamOwnership1897{}
	nsMap := map[string]*NamespaceOwnership{}

	extractTeam := func(labels map[string]string) string {
		// Common team label patterns
		teamKeys := []string{
			"app.kubernetes.io/managed-by", "team", "owner", "owner-team",
			"app.kubernetes.io/created-by", "department", "team.name", "maintainer",
		}
		for _, k := range teamKeys {
			if v, ok := labels[k]; ok {
				return v
			}
		}
		return ""
	}

	extractContact := func(annotations map[string]string) bool {
		if annotations == nil {
			return false
		}
		contactKeys := []string{
			"contact", "owner.email", "oncall", "support",
			"app.kubernetes.io/contact", "maintainer.email",
		}
		for _, k := range contactKeys {
			if _, ok := annotations[k]; ok {
				return true
			}
		}
		return false
	}

	analyze := func(name, ns, kind string, replicas int32, labels, annotations map[string]string) {
		result.Summary.TotalWorkloads++

		nsEntry, ok := nsMap[ns]
		if !ok {
			nsEntry = &NamespaceOwnership{Namespace: ns}
			nsMap[ns] = nsEntry
		}
		nsEntry.WorkloadCount++

		team := extractTeam(labels)
		hasContact := extractContact(annotations)

		if team != "" {
			result.Summary.WithOwner++
			nsEntry.WithOwnerLabel++
			nsEntry.TeamLabel = team

			t, ok := teamMap[team]
			if !ok {
				t = &TeamOwnership1897{Team: team}
				teamMap[team] = t
			}
			t.WorkloadCount++
			t.Replicas += int(replicas)
			t.HasContactInfo = hasContact || t.HasContactInfo

			// Track unique namespaces per team
			if t.NamespaceCount < 50 {
				// Count will be approximated
			}
		} else {
			result.Summary.WithoutOwner++
			entry := UnownedResource{
				Name: name, Namespace: ns, Kind: kind,
				Replicas: replicas, RiskLevel: "high",
			}
			if replicas > 3 {
				result.Summary.CriticalUnowned++
				entry.RiskLevel = "critical"
			}
			result.Unowned = append(result.Unowned, entry)
		}

		if hasContact {
			result.Summary.WithContactInfo++
		}
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		analyze(dep.Name, dep.Namespace, "Deployment", replicas, dep.Labels, dep.Annotations)
	}

	// Build team list
	for _, t := range teamMap {
		if !t.HasContactInfo {
			t.RiskLevel = "medium"
		} else {
			t.RiskLevel = "low"
		}
		result.ByTeam = append(result.ByTeam, *t)
	}
	sort.Slice(result.ByTeam, func(i, j int) bool {
		return result.ByTeam[i].WorkloadCount > result.ByTeam[j].WorkloadCount
	})
	result.Summary.TeamsIdentified = len(teamMap)

	// Build namespace list
	for _, ns := range nsMap {
		if ns.WithOwnerLabel == 0 {
			ns.RiskLevel = "high"
		} else if ns.WithOwnerLabel < ns.WorkloadCount/2 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].WorkloadCount > result.ByNamespace[j].WorkloadCount
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.WithOwner * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildOwnershipRecs1897(&result)
	writeJSON(w, result)
}

func buildOwnershipRecs1897(result *OwnershipRegistryResult) []string {
	recs := []string{
		fmt.Sprintf("Ownership registry: %d workloads, %d with owner (%d%%), %d teams identified",
			result.Summary.TotalWorkloads, result.Summary.WithOwner,
			safePercent1891(result.Summary.WithOwner, result.Summary.TotalWorkloads),
			result.Summary.TeamsIdentified),
	}
	if result.Summary.WithoutOwner > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without team/owner label - add ownership labels for accountability", result.Summary.WithoutOwner))
	}
	if result.Summary.CriticalUnowned > 0 {
		recs = append(recs, fmt.Sprintf("%d critical unowned workloads (>3 replicas) - assign ownership immediately", result.Summary.CriticalUnowned))
	}
	if result.Summary.WithContactInfo < result.Summary.WithOwner/2 {
		recs = append(recs, fmt.Sprintf("Only %d workloads have contact info - add annotations like 'contact' or 'oncall'", result.Summary.WithContactInfo))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Release Note Generator — auto-generates release notes from changes
// ---------------------------------------------------------------

type ReleaseNoteResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ReleaseNoteSummary      `json:"summary"`
	ReleaseNotes    string                  `json:"releaseNotes"`
	Changes         []ReleaseChange         `json:"changes"`
	ImageUpdates    []ImageUpdateEntry      `json:"imageUpdates"`
	ConfigChanges   []ConfigChangeEntry1897 `json:"configChanges"`
	SummaryStats    map[string]int          `json:"summaryStats"`
	Recommendations []string                `json:"recommendations"`
}

type ReleaseNoteSummary struct {
	Window             string `json:"window"`
	TotalChanges       int    `json:"totalChanges"`
	ImageUpdates       int    `json:"imageUpdates"`
	ConfigChanges      int    `json:"configChanges"`
	ScalingEvents      int    `json:"scalingEvents"`
	Creations          int    `json:"creations"`
	Deletions          int    `json:"deletions"`
	NamespacesAffected int    `json:"namespacesAffected"`
}

type ReleaseChange struct {
	Timestamp  string `json:"timestamp"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	ChangeType string `json:"changeType"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
}

type ImageUpdateEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
}

type ConfigChangeEntry1897 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"` // configmap, secret
	Timestamp string `json:"timestamp"`
}

func (s *Server) handleReleaseNoteGen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ReleaseNoteResult{
		ScannedAt:    time.Now(),
		SummaryStats: map[string]int{},
	}

	now := time.Now()
	windowHours := 24
	cutoff := now.Add(-time.Duration(windowHours) * time.Hour)
	result.Summary.Window = fmt.Sprintf("Last %d hours (%s to %s)", windowHours,
		cutoff.Format("2006-01-02 15:04"), now.Format("2006-01-02 15:04"))

	// Collect events
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	affectedNs := map[string]bool{}

	for _, evt := range events.Items {
		if evt.LastTimestamp.IsZero() || evt.LastTimestamp.Time.Before(cutoff) {
			continue
		}
		if isSystemNamespace(evt.Namespace) {
			continue
		}

		reason := evt.Reason
		msg := strings.ToLower(evt.Message)
		reasonLower := strings.ToLower(reason)

		change := ReleaseChange{
			Timestamp: evt.LastTimestamp.Format(time.RFC3339),
			Kind:      evt.InvolvedObject.Kind,
			Name:      evt.InvolvedObject.Name,
			Namespace: evt.Namespace,
			Reason:    reason,
			Message:   evt.Message,
		}
		affectedNs[evt.Namespace] = true

		// Classify change
		switch {
		case strings.Contains(msg, "image") || strings.Contains(msg, "pulled"):
			change.ChangeType = "image-update"
			result.Summary.ImageUpdates++
			result.ImageUpdates = append(result.ImageUpdates, ImageUpdateEntry{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Timestamp: evt.LastTimestamp.Format(time.RFC3339),
			})
		case strings.Contains(msg, "configmap") || strings.Contains(reasonLower, "configmap"):
			change.ChangeType = "config-change"
			result.Summary.ConfigChanges++
			result.ConfigChanges = append(result.ConfigChanges, ConfigChangeEntry1897{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Type:      "configmap",
				Timestamp: evt.LastTimestamp.Format(time.RFC3339),
			})
		case strings.Contains(msg, "secret"):
			change.ChangeType = "secret-change"
			result.Summary.ConfigChanges++
			result.ConfigChanges = append(result.ConfigChanges, ConfigChangeEntry1897{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Type:      "secret",
				Timestamp: evt.LastTimestamp.Format(time.RFC3339),
			})
		case strings.Contains(reasonLower, "scal"):
			change.ChangeType = "scaling"
			result.Summary.ScalingEvents++
		case strings.Contains(reasonLower, "create") || strings.Contains(reasonLower, "started"):
			change.ChangeType = "creation"
			result.Summary.Creations++
		case strings.Contains(reasonLower, "delete") || strings.Contains(reasonLower, "killing"):
			change.ChangeType = "deletion"
			result.Summary.Deletions++
		default:
			change.ChangeType = "other"
		}
		result.Summary.TotalChanges++
		result.SummaryStats[change.ChangeType]++
		result.Changes = append(result.Changes, change)
	}

	result.Summary.NamespacesAffected = len(affectedNs)

	// Sort by timestamp descending
	sort.Slice(result.Changes, func(i, j int) bool {
		return result.Changes[i].Timestamp > result.Changes[j].Timestamp
	})
	if len(result.Changes) > 50 {
		result.Changes = result.Changes[:50]
	}

	// Generate release notes markdown
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Release Notes - %s\n\n", now.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Window**: %s\n\n", result.Summary.Window))
	sb.WriteString(fmt.Sprintf("**Summary**: %d changes across %d namespaces\n\n", result.Summary.TotalChanges, result.Summary.NamespacesAffected))

	if result.Summary.ImageUpdates > 0 {
		sb.WriteString(fmt.Sprintf("## Image Updates (%d)\n\n", result.Summary.ImageUpdates))
		for _, iu := range result.ImageUpdates {
			sb.WriteString(fmt.Sprintf("- **%s/%s**: container image updated\n", iu.Namespace, iu.Name))
		}
		sb.WriteString("\n")
	}
	if result.Summary.ConfigChanges > 0 {
		sb.WriteString(fmt.Sprintf("## Configuration Changes (%d)\n\n", result.Summary.ConfigChanges))
		for _, cc := range result.ConfigChanges {
			sb.WriteString(fmt.Sprintf("- **%s/%s**: %s modified\n", cc.Namespace, cc.Name, cc.Type))
		}
		sb.WriteString("\n")
	}
	if result.Summary.ScalingEvents > 0 {
		sb.WriteString(fmt.Sprintf("## Scaling Events (%d)\n\n", result.Summary.ScalingEvents))
		sb.WriteString("Several workloads were scaled up or down during this period.\n\n")
	}
	if result.Summary.Deletions > 0 {
		sb.WriteString(fmt.Sprintf("## Resource Cleanup (%d)\n\n", result.Summary.Deletions))
		sb.WriteString("Pods were terminated as part of normal lifecycle or deployment.\n\n")
	}

	result.ReleaseNotes = sb.String()

	// Score: more changes = lower stability score
	if result.Summary.TotalChanges < 10 {
		result.HealthScore = 90
	} else if result.Summary.TotalChanges < 50 {
		result.HealthScore = 70
	} else if result.Summary.TotalChanges < 100 {
		result.HealthScore = 50
	} else {
		result.HealthScore = 30
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildReleaseNoteRecs1897(&result)
	writeJSON(w, result)
}

func buildReleaseNoteRecs1897(result *ReleaseNoteResult) []string {
	recs := []string{
		fmt.Sprintf("Release notes: %d changes in last 24h across %d namespaces (%d image updates, %d config changes)",
			result.Summary.TotalChanges, result.Summary.NamespacesAffected,
			result.Summary.ImageUpdates, result.Summary.ConfigChanges),
	}
	if result.Summary.TotalChanges > 100 {
		recs = append(recs, "High change velocity (>100 changes/24h) - consider batching deployments to reduce risk")
	}
	if result.Summary.ImageUpdates > 20 {
		recs = append(recs, fmt.Sprintf("%d image updates - verify CI/CD pipeline is promoting tested images only", result.Summary.ImageUpdates))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Incident Postmortem Template — generates structured postmortem
// ---------------------------------------------------------------

type PostmortemResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PostmortemSummary   `json:"summary"`
	Template        string              `json:"template"`
	Incidents       []IncidentEntry     `json:"incidents"`
	Timeline        []TimelineEntry1897 `json:"timeline"`
	ActionItems     []ActionItem1897    `json:"actionItems"`
	Recommendations []string            `json:"recommendations"`
}

type PostmortemSummary struct {
	DetectedIncidents int `json:"detectedIncidents"`
	CrashLoops        int `json:"crashLoops"`
	OOMKills          int `json:"oomKills"`
	NodeFailures      int `json:"nodeFailures"`
	HighRestartPods   int `json:"highRestartPods"`
	AffectedServices  int `json:"affectedServices"`
}

type IncidentEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type TimelineEntry1897 struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Source    string `json:"source"`
}

type ActionItem1897 struct {
	Priority    string `json:"priority"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func (s *Server) handleIncidentPostmortem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PostmortemResult{ScannedAt: time.Now()}

	now := time.Now()
	cutoff := now.Add(-72 * time.Hour) // Last 72 hours

	// Detect incidents from pod restarts and events
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	affectedServices := map[string]bool{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			// Check for OOM kills
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				if term.Reason == "OOMKilled" {
					result.Summary.OOMKills++
					result.Summary.DetectedIncidents++
					result.Incidents = append(result.Incidents, IncidentEntry{
						Name: pod.Name, Namespace: pod.Namespace,
						Type: "OOMKill", Severity: "high",
						Description: fmt.Sprintf("container %s killed by OOM (exit %d)", cs.Name, term.ExitCode),
						Timestamp:   term.FinishedAt.Format(time.RFC3339),
					})
					affectedServices[pod.Namespace+"/"+pod.Name] = true
				}
			}

			// Check for high restart counts
			if cs.RestartCount >= 5 {
				result.Summary.HighRestartPods++
				if cs.RestartCount >= 20 {
					result.Summary.DetectedIncidents++
					result.Incidents = append(result.Incidents, IncidentEntry{
						Name: pod.Name, Namespace: pod.Namespace,
						Type: "CrashLoopBackOff", Severity: "critical",
						Description: fmt.Sprintf("container %s restarted %d times", cs.Name, cs.RestartCount),
					})
					result.Summary.CrashLoops++
				}
				affectedServices[pod.Namespace+"/"+pod.Name] = true
			}
		}
	}

	// Check events for node failures and recent issues
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	for _, evt := range events.Items {
		if evt.LastTimestamp.IsZero() || evt.LastTimestamp.Time.Before(cutoff) {
			continue
		}
		if isSystemNamespace(evt.Namespace) {
			continue
		}

		reasonLower := strings.ToLower(evt.Reason)
		msgLower := strings.ToLower(evt.Message)

		// Node-related failures
		if strings.Contains(reasonLower, "node") || strings.Contains(reasonLower, "nodecordon") || strings.Contains(reasonLower, "nodenotready") {
			result.Summary.NodeFailures++
			result.Summary.DetectedIncidents++
			result.Incidents = append(result.Incidents, IncidentEntry{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Type: "NodeFailure", Severity: "critical",
				Description: evt.Message,
				Timestamp:   evt.LastTimestamp.Format(time.RFC3339),
			})
		}

		// Add to timeline
		if isIncidentEvent1897(reasonLower, msgLower) {
			result.Timeline = append(result.Timeline, TimelineEntry1897{
				Timestamp: evt.LastTimestamp.Format(time.RFC3339),
				Event:     fmt.Sprintf("[%s] %s: %s", evt.InvolvedObject.Kind, evt.Reason, evt.Message),
				Source:    evt.Namespace + "/" + evt.InvolvedObject.Name,
			})
		}
	}

	result.Summary.AffectedServices = len(affectedServices)

	// Sort timeline
	sort.Slice(result.Timeline, func(i, j int) bool {
		return result.Timeline[i].Timestamp > result.Timeline[j].Timestamp
	})
	if len(result.Timeline) > 30 {
		result.Timeline = result.Timeline[:30]
	}

	// Generate action items based on findings
	if result.Summary.OOMKills > 0 {
		result.ActionItems = append(result.ActionItems, ActionItem1897{
			Priority: "high", Category: "capacity",
			Description: fmt.Sprintf("Review and increase memory limits for %d pods with OOM kills", result.Summary.OOMKills),
		})
	}
	if result.Summary.CrashLoops > 0 {
		result.ActionItems = append(result.ActionItems, ActionItem1897{
			Priority: "critical", Category: "reliability",
			Description: fmt.Sprintf("Fix %d CrashLoopBackOff pods - investigate startup failure root cause", result.Summary.CrashLoops),
		})
	}
	if result.Summary.HighRestartPods > 0 {
		result.ActionItems = append(result.ActionItems, ActionItem1897{
			Priority: "medium", Category: "stability",
			Description: fmt.Sprintf("Add liveness/readiness probes for %d pods with high restart counts", result.Summary.HighRestartPods),
		})
	}
	if len(result.ActionItems) == 0 {
		result.ActionItems = append(result.ActionItems, ActionItem1897{
			Priority: "low", Category: "improvement",
			Description: "No major incidents detected - continue monitoring",
		})
	}

	// Generate markdown template
	var sb strings.Builder
	sb.WriteString("# Incident Postmortem\n\n")
	sb.WriteString(fmt.Sprintf("**Date**: %s\n\n", now.Format("2006-01-02")))
	sb.WriteString("## Summary\n\n")
	if result.Summary.DetectedIncidents > 0 {
		sb.WriteString(fmt.Sprintf("During the last 72 hours, %d incidents were detected:\n", result.Summary.DetectedIncidents))
		sb.WriteString(fmt.Sprintf("- %d OOM kills\n", result.Summary.OOMKills))
		sb.WriteString(fmt.Sprintf("- %d CrashLoopBackOff patterns\n", result.Summary.CrashLoops))
		sb.WriteString(fmt.Sprintf("- %d node failure events\n", result.Summary.NodeFailures))
		sb.WriteString(fmt.Sprintf("- %d pods with high restart rates\n\n", result.Summary.HighRestartPods))
	} else {
		sb.WriteString("No major incidents detected in the last 72 hours.\n\n")
	}

	sb.WriteString("## Impact\n\n")
	sb.WriteString(fmt.Sprintf("- Affected services: %d\n", result.Summary.AffectedServices))
	sb.WriteString("- Duration: [TO BE FILLED]\n\n")
	sb.WriteString("## Root Cause\n\n")
	sb.WriteString("[TO BE FILLED - describe what caused the incident]\n\n")
	sb.WriteString("## Timeline\n\n")
	if len(result.Timeline) > 0 {
		for _, tl := range result.Timeline {
			if len(result.Timeline) > 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tl.Timestamp, tl.Event))
		}
	} else {
		sb.WriteString("- [TO BE FILLED - add key timestamps]\n")
	}
	sb.WriteString("\n## Action Items\n\n")
	for _, ai := range result.ActionItems {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", strings.ToUpper(ai.Priority), ai.Description))
	}
	sb.WriteString("\n## Lessons Learned\n\n")
	sb.WriteString("[TO BE FILLED - what did we learn?]\n\n")

	result.Template = sb.String()

	// Score: more incidents = lower score
	if result.Summary.DetectedIncidents == 0 {
		result.HealthScore = 95
	} else if result.Summary.DetectedIncidents <= 2 {
		result.HealthScore = 75
	} else if result.Summary.DetectedIncidents <= 5 {
		result.HealthScore = 50
	} else {
		result.HealthScore = 25
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildPostmortemRecs1897(&result)
	writeJSON(w, result)
}

func isIncidentEvent1897(reason, msg string) bool {
	keywords := []string{"crash", "oom", "kill", "fail", "unhealthy", "backoff", "evict", "lost", "error"}
	for _, kw := range keywords {
		if strings.Contains(reason, kw) || strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func buildPostmortemRecs1897(result *PostmortemResult) []string {
	recs := []string{
		fmt.Sprintf("Incident postmortem: %d incidents detected (72h), %d affected services, %d action items",
			result.Summary.DetectedIncidents, result.Summary.AffectedServices, len(result.ActionItems)),
	}
	if result.Summary.OOMKills > 0 {
		recs = append(recs, fmt.Sprintf("%d OOM kill incidents - review memory requests/limits and add HPA for memory pressure", result.Summary.OOMKills))
	}
	if result.Summary.CrashLoops > 0 {
		recs = append(recs, fmt.Sprintf("%d CrashLoopBackOff patterns - investigate application startup failures", result.Summary.CrashLoops))
	}
	return recs
}

// keep reference to avoid unused import
var _ = appsv1.DeploymentStrategy{}
var _ corev1.Pod
