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

// ============================================================
// v19.00 — Deployment Dimension (Round 3)
// 1. Graceful Shutdown Audit
// 2. Rollout Speed Analyzer
// 3. Deploy Conflict Detector
// ============================================================

// ---------------------------------------------------------------
// 1. Graceful Shutdown Audit
// ---------------------------------------------------------------

type GracefulShutdownResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         ShutdownSummary `json:"summary"`
	NonGraceful     []ShutdownEntry `json:"nonGraceful"`
	ByWorkload      []ShutdownEntry `json:"byWorkload"`
	Recommendations []string        `json:"recommendations"`
}

type ShutdownSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	WithPreStop      int `json:"withPreStop"`
	WithoutPreStop   int `json:"withoutPreStop"`
	LongGracePeriod  int `json:"longGracePeriod"`
	ShortGracePeriod int `json:"shortGracePeriod"`
	DefaultGrace     int `json:"defaultGrace"`
	ReadinessGates   int `json:"readinessGates"`
}

type ShutdownEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Kind         string `json:"kind"`
	HasPreStop   bool   `json:"hasPreStop"`
	GracePeriod  int64  `json:"gracePeriodSeconds"`
	HasReadiness bool   `json:"hasReadinessProbe"`
	RiskLevel    string `json:"riskLevel"`
	Issue        string `json:"issue"`
}

func (s *Server) handleGracefulShutdownAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := GracefulShutdownResult{ScannedAt: time.Now()}

	analyze := func(name, ns, kind string, spec *corev1.PodSpec) {
		result.Summary.TotalWorkloads++
		entry := ShutdownEntry{Name: name, Namespace: ns, Kind: kind}

		// Grace period
		entry.GracePeriod = 30 // default
		if spec.TerminationGracePeriodSeconds != nil {
			entry.GracePeriod = *spec.TerminationGracePeriodSeconds
		}
		if entry.GracePeriod > 60 {
			result.Summary.LongGracePeriod++
		} else if entry.GracePeriod < 10 {
			result.Summary.ShortGracePeriod++
		} else {
			result.Summary.DefaultGrace++
		}

		// PreStop hook
		for _, c := range spec.Containers {
			if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
				entry.HasPreStop = true
				break
			}
		}
		if entry.HasPreStop {
			result.Summary.WithPreStop++
		} else {
			result.Summary.WithoutPreStop++
			entry.RiskLevel = "medium"
			entry.Issue = "no preStop hook - connections may be dropped abruptly"
			result.NonGraceful = append(result.NonGraceful, entry)
		}

		// Readiness probe (for connection draining)
		for _, c := range spec.Containers {
			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
				break
			}
		}
		if !entry.HasReadiness && entry.RiskLevel == "" {
			entry.RiskLevel = "medium"
			entry.Issue = "no readiness probe - traffic not drained before termination"
		}

		if entry.RiskLevel == "" {
			entry.RiskLevel = "low"
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		analyze(dep.Name, dep.Namespace, "Deployment", &dep.Spec.Template.Spec)
	}
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		analyze(ss.Name, ss.Namespace, "StatefulSet", &ss.Spec.Template.Spec)
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		gracefulPct := result.Summary.WithPreStop * 100 / result.Summary.TotalWorkloads
		result.HealthScore = gracefulPct
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildShutdownRecs1900(&result)
	writeJSON(w, result)
}

func buildShutdownRecs1900(r *GracefulShutdownResult) []string {
	recs := []string{fmt.Sprintf("Graceful shutdown: %d workloads, %d with preStop (%d%%), %d without",
		r.Summary.TotalWorkloads, r.Summary.WithPreStop,
		safePercent1891(r.Summary.WithPreStop, r.Summary.TotalWorkloads),
		r.Summary.WithoutPreStop)}
	if r.Summary.WithoutPreStop > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without preStop hook - risk of connection drops during termination", r.Summary.WithoutPreStop))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Rollout Speed Analyzer
// ---------------------------------------------------------------

type RolloutSpeedResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         RolloutSpeedSummary `json:"summary"`
	ByWorkload      []RolloutSpeedEntry `json:"byWorkload"`
	SlowRollouts    []RolloutSpeedEntry `json:"slowRollouts"`
	Recommendations []string            `json:"recommendations"`
}

type RolloutSpeedSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	AvgReplicas      int `json:"avgReplicas"`
	FastRollout      int `json:"fastRollout"`
	SlowRollout      int `json:"slowRollout"`
	RecreateCount    int `json:"recreateCount"`
	NoHistory        int `json:"noHistory"`
}

type RolloutSpeedEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Strategy        string `json:"strategy"`
	Replicas        int32  `json:"replicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	Surge           string `json:"maxSurge"`
	Unavailable     string `json:"maxUnavailable"`
	RevisionHistory int32  `json:"revisionHistory"`
	RolloutStatus   string `json:"rolloutStatus"`
	RiskLevel       string `json:"riskLevel"`
}

func (s *Server) handleRolloutSpeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RolloutSpeedResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := RolloutSpeedEntry{
			Name: dep.Name, Namespace: dep.Namespace,
			Strategy: string(dep.Spec.Strategy.Type),
		}
		if dep.Spec.Replicas != nil {
			entry.Replicas = *dep.Spec.Replicas
		}
		entry.UpdatedReplicas = dep.Status.UpdatedReplicas
		entry.ReadyReplicas = dep.Status.ReadyReplicas
		result.Summary.AvgReplicas += int(entry.Replicas)

		// Strategy details
		if dep.Spec.Strategy.RollingUpdate != nil {
			ru := dep.Spec.Strategy.RollingUpdate
			if ru.MaxSurge != nil {
				entry.Surge = ru.MaxSurge.String()
			}
			if ru.MaxUnavailable != nil {
				entry.Unavailable = ru.MaxUnavailable.String()
			}
		}
		if dep.Spec.RevisionHistoryLimit != nil {
			entry.RevisionHistory = *dep.Spec.RevisionHistoryLimit
		} else {
			result.Summary.NoHistory++
		}

		// Determine rollout status
		if entry.UpdatedReplicas == entry.Replicas && entry.ReadyReplicas >= entry.Replicas {
			entry.RolloutStatus = "complete"
			entry.RiskLevel = "low"
			result.Summary.FastRollout++
		} else if entry.UpdatedReplicas < entry.Replicas {
			entry.RolloutStatus = "in-progress"
			entry.RiskLevel = "medium"
			result.Summary.SlowRollout++
			result.SlowRollouts = append(result.SlowRollouts, entry)
		} else {
			entry.RolloutStatus = "unknown"
			entry.RiskLevel = "medium"
		}

		if entry.Strategy == "Recreate" {
			result.Summary.RecreateCount++
			entry.RiskLevel = "high"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if result.Summary.TotalDeployments > 0 {
		result.Summary.AvgReplicas /= result.Summary.TotalDeployments
		completePct := result.Summary.FastRollout * 100 / result.Summary.TotalDeployments
		result.HealthScore = completePct
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildRolloutSpeedRecs1900(&result)
	writeJSON(w, result)
}

func buildRolloutSpeedRecs1900(r *RolloutSpeedResult) []string {
	recs := []string{fmt.Sprintf("Rollout speed: %d deployments, %d complete, %d in-progress, %d Recreate",
		r.Summary.TotalDeployments, r.Summary.FastRollout, r.Summary.SlowRollout, r.Summary.RecreateCount)}
	if r.Summary.RecreateCount > 0 {
		recs = append(recs, fmt.Sprintf("%d Recreate deployments - switch to RollingUpdate for zero-downtime", r.Summary.RecreateCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Deploy Conflict Detector
// ---------------------------------------------------------------

type DeployConflictResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ConflictSummary     `json:"summary"`
	Conflicts       []ConflictEntry     `json:"conflicts"`
	NamespacePair   []NSConflictEntry   `json:"namespacePairs"`
	RecentChanges   []RecentChangeEntry `json:"recentChanges"`
	Recommendations []string            `json:"recommendations"`
}

type ConflictSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	ConflictingPairs  int `json:"conflictingPairs"`
	ResourceConflicts int `json:"resourceConflicts"`
	NameConflicts     int `json:"nameConflicts"`
	RecentChanges     int `json:"recentChanges"`
	ConcurrentDeploys int `json:"concurrentDeploys"`
}

type ConflictEntry struct {
	Type      string `json:"type"`
	Resource1 string `json:"resource1"`
	NS1       string `json:"namespace1"`
	Resource2 string `json:"resource2"`
	NS2       string `json:"namespace2"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type NSConflictEntry struct {
	Namespace string `json:"namespace"`
	Workloads int    `json:"workloads"`
	CPUm      int    `json:"cpuMilli"`
	MemMB     int    `json:"memMB"`
	RiskLevel string `json:"riskLevel"`
}

type RecentChangeEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Timestamp  string `json:"timestamp"`
	ChangeType string `json:"changeType"`
}

func (s *Server) handleDeployConflict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DeployConflictResult{ScannedAt: time.Now()}

	// Track resource per namespace
	nsResources := map[string]*NSConflictEntry{}
	nameMap := map[string][]string{} // name -> []namespaces

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		// Track name collisions across namespaces
		nameMap[dep.Name] = append(nameMap[dep.Name], dep.Namespace)

		// Track resources
		nsE, ok := nsResources[dep.Namespace]
		if !ok {
			nsE = &NSConflictEntry{Namespace: dep.Namespace}
			nsResources[dep.Namespace] = nsE
		}
		nsE.Workloads++
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				nsE.CPUm += int(qty.MilliValue())
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				nsE.MemMB += int(qty.Value() / (1024 * 1024))
			}
		}
	}

	// Detect name conflicts (same deployment name in multiple namespaces)
	for name, nss := range nameMap {
		if len(nss) > 1 {
			result.Summary.NameConflicts++
			result.Summary.ConflictingPairs++
			result.Conflicts = append(result.Conflicts, ConflictEntry{
				Type: "name-collision", Resource1: name, NS1: nss[0],
				Resource2: name, NS2: nss[1],
				Severity: "low",
				Detail:   fmt.Sprintf("deployment name '%s' exists in %d namespaces", name, len(nss)),
			})
		}
	}

	// Detect high-resource namespaces (potential contention)
	for _, ns := range nsResources {
		if ns.CPUm > 5000 || ns.MemMB > 10000 {
			ns.RiskLevel = "high"
			result.Summary.ResourceConflicts++
			result.Conflicts = append(result.Conflicts, ConflictEntry{
				Type: "resource-pressure", Resource1: ns.Namespace, NS1: ns.Namespace,
				Severity: "medium",
				Detail:   fmt.Sprintf("namespace %s: %dm CPU / %dMB Mem requested", ns.Namespace, ns.CPUm, ns.MemMB),
			})
		} else {
			ns.RiskLevel = "low"
		}
		result.NamespacePair = append(result.NamespacePair, *ns)
	}

	// Check recent events for concurrent deployments
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	concurrentTimes := map[string]int{} // minute -> count

	for _, evt := range events.Items {
		if evt.LastTimestamp.IsZero() || evt.LastTimestamp.Time.Before(cutoff) {
			continue
		}
		if isSystemNamespace(evt.Namespace) {
			continue
		}
		reasonLower := strings.ToLower(evt.Reason)
		if strings.Contains(reasonLower, "scal") || strings.Contains(reasonLower, "rollout") {
			minute := evt.LastTimestamp.Format("2006-01-02T15:04")
			concurrentTimes[minute]++
			result.Summary.RecentChanges++
			result.RecentChanges = append(result.RecentChanges, RecentChangeEntry{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Timestamp:  evt.LastTimestamp.Format(time.RFC3339),
				ChangeType: classifyChangeEvent(reasonLower, evt.Message),
			})
		}
	}

	// Detect concurrent deployments (same minute)
	for minute, count := range concurrentTimes {
		if count > 3 {
			result.Summary.ConcurrentDeploys++
			result.Conflicts = append(result.Conflicts, ConflictEntry{
				Type: "concurrent-deploy", Resource1: minute, NS1: "",
				Severity: "medium",
				Detail:   fmt.Sprintf("%d deployment events in minute %s - potential resource contention", count, minute),
			})
		}
	}

	sort.Slice(result.NamespacePair, func(i, j int) bool {
		return result.NamespacePair[i].CPUm > result.NamespacePair[j].CPUm
	})
	sort.Slice(result.RecentChanges, func(i, j int) bool {
		return result.RecentChanges[i].Timestamp > result.RecentChanges[j].Timestamp
	})
	if len(result.RecentChanges) > 30 {
		result.RecentChanges = result.RecentChanges[:30]
	}

	if result.Summary.TotalWorkloads > 0 {
		conflictRate := result.Summary.ConflictingPairs * 100 / result.Summary.TotalWorkloads
		result.HealthScore = 100 - conflictRate
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildConflictRecs1900(&result)
	writeJSON(w, result)
}

func buildConflictRecs1900(r *DeployConflictResult) []string {
	recs := []string{fmt.Sprintf("Deploy conflicts: %d workloads, %d conflicts (%d name, %d resource, %d concurrent)",
		r.Summary.TotalWorkloads, r.Summary.ConflictingPairs,
		r.Summary.NameConflicts, r.Summary.ResourceConflicts, r.Summary.ConcurrentDeploys)}
	if r.Summary.ConcurrentDeploys > 0 {
		recs = append(recs, fmt.Sprintf("%d concurrent deployment windows detected - space out rollouts to reduce contention", r.Summary.ConcurrentDeploys))
	}
	return recs
}
