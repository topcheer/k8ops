package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RollbackRiskResult is the rollback risk & revision integrity assessment.
type RollbackRiskResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         RollbackSummary    `json:"summary"`
	Workloads       []RollbackWorkload `json:"workloads"`
	HighRiskTargets []RollbackWorkload `json:"highRiskTargets,omitempty"`
	Issues          []RollbackIssue    `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	ReadinessScore  int                `json:"readinessScore"`
}

// RollbackSummary aggregates rollback readiness statistics.
type RollbackSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithHistory       int `json:"withRevisionHistory"` // has revisionHistoryLimit > 0
	LimitedHistory    int `json:"limitedHistory"`      // revisionHistoryLimit < 5
	NoHistory         int `json:"noHistory"`           // revisionHistoryLimit = 0
	HighRollbackRisk  int `json:"highRollbackRisk"`
	SafeRollback      int `json:"safeRollback"`
	WithConfigChanges int `json:"withConfigChanges"` // current revision has config changes
	WithImageChanges  int `json:"withImageChanges"`  // current revision has image changes
	WithSchemaChanges int `json:"withSchemaChanges"` // detected breaking changes
}

// RollbackWorkload assesses rollback readiness for one workload.
type RollbackWorkload struct {
	Name              string          `json:"name"`
	Namespace         string          `json:"namespace"`
	Kind              string          `json:"kind"`
	Replicas          int             `json:"replicas"`
	RevisionLimit     int             `json:"revisionHistoryLimit"`
	RollbackReady     bool            `json:"rollbackReady"`
	RiskLevel         string          `json:"riskLevel"` // safe, low, medium, high, critical
	Image             string          `json:"currentImage"`
	ConfigRefs        []string        `json:"configRefs,omitempty"`
	HasBreakingChange bool            `json:"hasBreakingChange"`
	Reasons           []string        `json:"reasons,omitempty"`
	Checks            []RollbackCheck `json:"checks"`
}

// RollbackCheck is a single rollback readiness check.
type RollbackCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, warn, fail
	Detail string `json:"detail"`
}

// RollbackIssue describes a rollback risk issue.
type RollbackIssue struct {
	Workload string `json:"workload,omitempty"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// handleRollbackRisk assesses rollback readiness for all workloads.
// GET /api/deployment/rollback-risk
func (s *Server) handleRollbackRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := RollbackRiskResult{ScannedAt: time.Now()}

	// Collect ReplicaSets for revision analysis
	rsMap := map[string][]appsv1.ReplicaSet{} // "ns/deployname" → ReplicaSets
	allRS, err := rc.clientset.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, rs := range allRS.Items {
			for _, owner := range rs.OwnerReferences {
				if owner.Kind == "Deployment" {
					key := fmt.Sprintf("%s/%s", rs.Namespace, owner.Name)
					rsMap[key] = append(rsMap[key], rs)
				}
			}
		}
	}

	// Collect ConfigMaps and Secrets for config drift detection
	cmVersions := map[string]string{} // "ns/name" → resourceVersion
	configmaps, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{Limit: 500})
	if err == nil {
		for _, cm := range configmaps.Items {
			cmVersions[fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)] = cm.ResourceVersion
		}
	}

	var allWorkloads []RollbackWorkload

	// Process Deployments
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range deployments.Items {
			d := &deployments.Items[i]
			replicas := 1
			if d.Spec.Replicas != nil {
				replicas = int(*d.Spec.Replicas)
			}

			revLimit := 10 // default
			if d.Spec.RevisionHistoryLimit != nil {
				revLimit = int(*d.Spec.RevisionHistoryLimit)
			}

			// Count ReplicaSets (revisions)
			rsKey := fmt.Sprintf("%s/%s", d.Namespace, d.Name)
			revisions := rsMap[rsKey]

			// Get current image
			var images []string
			for _, c := range d.Spec.Template.Spec.Containers {
				images = append(images, c.Image)
			}

			// Get config refs
			var configRefs []string
			for _, c := range d.Spec.Template.Spec.Containers {
				for _, envFrom := range c.EnvFrom {
					if envFrom.ConfigMapRef != nil {
						configRefs = append(configRefs, envFrom.ConfigMapRef.Name)
					}
					if envFrom.SecretRef != nil {
						configRefs = append(configRefs, "secret/"+envFrom.SecretRef.Name)
					}
				}
			}

			wl := assessRollbackWorkload(
				"Deployment", d.Name, d.Namespace, replicas,
				revLimit, len(revisions), images, configRefs, d.CreationTimestamp.Time,
			)
			allWorkloads = append(allWorkloads, wl)
		}
	}

	// Process StatefulSets
	stss, err := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range stss.Items {
			sts := &stss.Items[i]
			replicas := 1
			if sts.Spec.Replicas != nil {
				replicas = int(*sts.Spec.Replicas)
			}

			revLimit := 10
			if sts.Spec.RevisionHistoryLimit != nil {
				revLimit = int(*sts.Spec.RevisionHistoryLimit)
			}

			var images []string
			for _, c := range sts.Spec.Template.Spec.Containers {
				images = append(images, c.Image)
			}

			wl := assessRollbackWorkload(
				"StatefulSet", sts.Name, sts.Namespace, replicas,
				revLimit, 0, images, nil, sts.CreationTimestamp.Time,
			)
			allWorkloads = append(allWorkloads, wl)
		}
	}

	// Sort by risk level (highest risk first)
	sort.Slice(allWorkloads, func(i, j int) bool {
		return rollbackRiskOrder(allWorkloads[i].RiskLevel) < rollbackRiskOrder(allWorkloads[j].RiskLevel)
	})

	result.Workloads = allWorkloads
	if len(allWorkloads) > 100 {
		result.Workloads = allWorkloads[:100]
	}

	// Identify high-risk targets
	for _, wl := range allWorkloads {
		if wl.RiskLevel == "high" || wl.RiskLevel == "critical" {
			result.HighRiskTargets = append(result.HighRiskTargets, wl)
		}
	}

	// Build summary
	result.Summary.TotalWorkloads = len(allWorkloads)
	for _, wl := range allWorkloads {
		switch {
		case wl.RevisionLimit == 0:
			result.Summary.NoHistory++
		case wl.RevisionLimit < 5:
			result.Summary.LimitedHistory++
		default:
			result.Summary.WithHistory++
		}
		if wl.RiskLevel == "high" || wl.RiskLevel == "critical" {
			result.Summary.HighRollbackRisk++
		}
		if wl.RiskLevel == "safe" {
			result.Summary.SafeRollback++
		}
		if wl.HasBreakingChange {
			result.Summary.WithSchemaChanges++
		}
	}

	// Generate issues
	result.Issues = generateRollbackIssues(result)

	// Calculate readiness score
	score := 100
	score -= result.Summary.NoHistory * 10
	score -= result.Summary.LimitedHistory * 3
	score -= result.Summary.HighRollbackRisk * 5
	if score < 0 {
		score = 0
	}
	result.ReadinessScore = score

	// Recommendations
	result.Recommendations = generateRollbackRecommendations(result)

	writeJSON(w, result)
}

// assessRollbackWorkload evaluates rollback readiness for a single workload.
func assessRollbackWorkload(
	kind, name, namespace string,
	replicas, revisionLimit, revisionCount int,
	images, configRefs []string,
	created time.Time,
) RollbackWorkload {
	wl := RollbackWorkload{
		Name:          name,
		Namespace:     namespace,
		Kind:          kind,
		Replicas:      replicas,
		RevisionLimit: revisionLimit,
		Image:         strings.Join(images, ", "),
		ConfigRefs:    configRefs,
		RiskLevel:     "safe",
	}

	// Check 1: Revision history availability
	if revisionLimit == 0 {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Revision History",
			Status: "fail",
			Detail: "revisionHistoryLimit=0 — rollback is impossible, no old versions retained",
		})
		wl.RiskLevel = "critical"
		wl.RollbackReady = false
		wl.Reasons = append(wl.Reasons, "No revision history retained")
	} else if revisionLimit < 5 {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Revision History",
			Status: "warn",
			Detail: fmt.Sprintf("revisionHistoryLimit=%d — limited rollback options (<5)", revisionLimit),
		})
		if wl.RiskLevel == "safe" {
			wl.RiskLevel = "low"
		}
		wl.RollbackReady = true
	} else {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Revision History",
			Status: "pass",
			Detail: fmt.Sprintf("revisionHistoryLimit=%d — sufficient rollback history", revisionLimit),
		})
		wl.RollbackReady = true
	}

	// Check 2: Multi-replica (single replica rollback is riskier)
	if replicas <= 1 {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Replica Count",
			Status: "warn",
			Detail: "Single replica — rollback will cause downtime during image switch",
		})
		if wl.RiskLevel == "safe" {
			wl.RiskLevel = "medium"
		}
		wl.Reasons = append(wl.Reasons, "Single replica: rollback causes brief downtime")
	} else {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Replica Count",
			Status: "pass",
			Detail: fmt.Sprintf("%d replicas — rolling rollback possible without downtime", replicas),
		})
	}

	// Check 3: Image tag analysis
	imageStr := strings.Join(images, " ")
	if strings.Contains(imageStr, ":latest") {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Image Stability",
			Status: "warn",
			Detail: "Uses :latest tag — rollback to previous version may not work as expected",
		})
		if wl.RiskLevel == "safe" {
			wl.RiskLevel = "medium"
		}
		wl.HasBreakingChange = true
		wl.Reasons = append(wl.Reasons, ":latest tag: image content may have changed")
	} else {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Image Stability",
			Status: "pass",
			Detail: "Versioned image tag — rollback to specific version is deterministic",
		})
	}

	// Check 4: Config dependency stability
	if len(configRefs) > 0 {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Config Dependencies",
			Status: "warn",
			Detail: fmt.Sprintf("References %d ConfigMap/Secret(s) — rollback may use incompatible config", len(configRefs)),
		})
		if wl.RiskLevel == "safe" || wl.RiskLevel == "low" {
			wl.RiskLevel = "medium"
		}
		wl.Reasons = append(wl.Reasons, "Config dependencies may have changed since last revision")
	}

	// Check 5: Workload age (very new workloads have less rollback context)
	age := time.Since(created)
	if age < 1*time.Hour {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Workload Maturity",
			Status: "warn",
			Detail: fmt.Sprintf("Created %.0f minutes ago — limited production validation", age.Minutes()),
		})
		if wl.RiskLevel == "safe" {
			wl.RiskLevel = "low"
		}
	} else {
		wl.Checks = append(wl.Checks, RollbackCheck{
			Name:   "Workload Maturity",
			Status: "pass",
			Detail: fmt.Sprintf("Running for %.0f hours — sufficient history", age.Hours()),
		})
	}

	return wl
}

// rollbackRiskOrder returns numeric ordering for risk levels.
func rollbackRiskOrder(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

// generateRollbackIssues creates issue entries from the assessment.
func generateRollbackIssues(result RollbackRiskResult) []RollbackIssue {
	var issues []RollbackIssue

	if result.Summary.NoHistory > 0 {
		issues = append(issues, RollbackIssue{
			Severity: "critical",
			Category: "no-revision-history",
			Message:  fmt.Sprintf("%d workload(s) have revisionHistoryLimit=0 — rollback is impossible", result.Summary.NoHistory),
		})
	}

	if result.Summary.LimitedHistory > 0 {
		issues = append(issues, RollbackIssue{
			Severity: "warning",
			Category: "limited-history",
			Message:  fmt.Sprintf("%d workload(s) have limited revision history (<5) — rollback options are restricted", result.Summary.LimitedHistory),
		})
	}

	for _, wl := range result.HighRiskTargets {
		for _, reason := range wl.Reasons {
			issues = append(issues, RollbackIssue{
				Workload: fmt.Sprintf("%s/%s", wl.Namespace, wl.Name),
				Severity: wl.RiskLevel,
				Category: "rollback-risk",
				Message:  reason,
			})
		}
	}

	if len(issues) > 100 {
		issues = issues[:100]
	}

	return issues
}

// generateRollbackRecommendations produces actionable recommendations.
func generateRollbackRecommendations(result RollbackRiskResult) []string {
	var recs []string

	if result.Summary.NoHistory > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) cannot be rolled back (revisionHistoryLimit=0) — increase to at least 5", result.Summary.NoHistory))
	}

	if result.Summary.LimitedHistory > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have limited history (<5 revisions) — consider increasing revisionHistoryLimit", result.Summary.LimitedHistory))
	}

	if len(result.HighRiskTargets) > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are high rollback risk — fix :latest tags, ensure multi-replica, and stabilize configs before relying on rollback", len(result.HighRiskTargets)))
	}

	if result.ReadinessScore < 50 {
		recs = append(recs, fmt.Sprintf("Rollback readiness score is %d/100 — cluster is not well-prepared for safe rollbacks", result.ReadinessScore))
	}

	if result.Summary.SafeRollback > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are rollback-safe — proper revision history, stable images, and multi-replica", result.Summary.SafeRollback))
	}

	if len(recs) == 0 {
		recs = append(recs, "All workloads are rollback-ready — revision history is preserved and images are versioned")
	}

	return recs
}
