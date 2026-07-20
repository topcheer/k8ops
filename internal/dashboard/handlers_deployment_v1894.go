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
// v18.94 — Deployment Dimension
// 1. Deploy Reproducibility Audit
// 2. Update Compliance Deep
// 3. Container Restart Policy Deep
// ============================================================

// ---------------------------------------------------------------
// 1. Deploy Reproducibility Audit
// ---------------------------------------------------------------

type DeployReproducibilityResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ReproducibilitySummary `json:"summary"`
	NonReproducible []ReproducibilityEntry `json:"nonReproducible"`
	ImageTags       []ImageTagEntry        `json:"imageTags"`
	EnvConsistency  []EnvConsistencyEntry  `json:"envConsistency"`
	Recommendations []string               `json:"recommendations"`
}

type ReproducibilitySummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	FullyReproducible int `json:"fullyReproducible"`
	PartiallyRepro    int `json:"partiallyReproducible"`
	NonReproducible   int `json:"nonReproducible"`
	WithDigestPin     int `json:"withDigestPin"`
	WithLatestTag     int `json:"withLatestTag"`
	WithNoTag         int `json:"withNoTag"`
	WithConfigRef     int `json:"withConfigRef"`
	WithoutConfigRef  int `json:"withoutConfigRef"`
}

type ReproducibilityEntry struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	Issues    []string `json:"issues"`
	Score     int      `json:"score"`
	RiskLevel string   `json:"riskLevel"`
}

type ImageTagEntry struct {
	Image          string `json:"image"`
	Workload       string `json:"workload"`
	Namespace      string `json:"namespace"`
	TagType        string `json:"tagType"` // digest, version, latest, none
	IsReproducible bool   `json:"isReproducible"`
}

type EnvConsistencyEntry struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	EnvType   string `json:"envType"` // configmap, secret, literal
	EnvName   string `json:"envName"`
	Source    string `json:"source"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleDeployReproducibility(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DeployReproducibilityResult{ScannedAt: time.Now()}

	analyze := func(name, ns, kind string, podSpec *corev1.PodSpec) {
		result.Summary.TotalWorkloads++
		entry := ReproducibilityEntry{Name: name, Namespace: ns, Kind: kind, Score: 100}

		// Check image tags
		for _, c := range podSpec.Containers {
			img := c.Image
			iEntry := ImageTagEntry{Image: img, Workload: name, Namespace: ns}
			switch {
			case strings.Contains(img, "@sha256:"):
				iEntry.TagType = "digest"
				iEntry.IsReproducible = true
				result.Summary.WithDigestPin++
			case strings.HasSuffix(img, ":latest"):
				iEntry.TagType = "latest"
				iEntry.IsReproducible = false
				result.Summary.WithLatestTag++
				entry.Issues = append(entry.Issues, "container "+c.Name+" uses :latest tag")
				entry.Score -= 25
			case !strings.Contains(img, ":"):
				iEntry.TagType = "none"
				iEntry.IsReproducible = false
				result.Summary.WithNoTag++
				result.Summary.WithNoTag++
				entry.Issues = append(entry.Issues, "container "+c.Name+" has no tag")
				entry.Score -= 30
			default:
				iEntry.TagType = "version"
				iEntry.IsReproducible = true
			}
			result.ImageTags = append(result.ImageTags, iEntry)
		}

		// Check environment variable sources
		hasConfigRef := false
		for _, c := range podSpec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil {
					hasConfigRef = true
					result.Summary.WithConfigRef++
					source := "unknown"
					et := "literal"
					if env.ValueFrom.ConfigMapKeyRef != nil {
						source = "configmap:" + env.ValueFrom.ConfigMapKeyRef.Name
						et = "configmap"
					} else if env.ValueFrom.SecretKeyRef != nil {
						source = "secret:" + env.ValueFrom.SecretKeyRef.Name
						et = "secret"
					}
					result.EnvConsistency = append(result.EnvConsistency, EnvConsistencyEntry{
						Workload: name, Namespace: ns, EnvName: env.Name, EnvType: et, Source: source, RiskLevel: "low",
					})
				} else if env.Value != "" {
					// Literal env var - not reproducible across environments
					if isSensitiveEnvKey1894(env.Name) {
						result.EnvConsistency = append(result.EnvConsistency, EnvConsistencyEntry{
							Workload: name, Namespace: ns, EnvName: env.Name, EnvType: "literal", Source: "inline", RiskLevel: "high",
						})
						entry.Issues = append(entry.Issues, "sensitive env var "+env.Name+" is hardcoded")
						entry.Score -= 15
					}
				}
			}
		}
		if !hasConfigRef {
			result.Summary.WithoutConfigRef++
		}

		// Classify reproducibility
		switch {
		case entry.Score >= 90:
			result.Summary.FullyReproducible++
			entry.RiskLevel = "low"
		case entry.Score >= 60:
			result.Summary.PartiallyRepro++
			entry.RiskLevel = "medium"
			result.NonReproducible = append(result.NonReproducible, entry)
		default:
			result.Summary.NonReproducible++
			entry.RiskLevel = "high"
			result.NonReproducible = append(result.NonReproducible, entry)
		}
	}

	// Deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		analyze(dep.Name, dep.Namespace, "Deployment", &dep.Spec.Template.Spec)
	}
	// StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		analyze(ss.Name, ss.Namespace, "StatefulSet", &ss.Spec.Template.Spec)
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		reproPct := result.Summary.FullyReproducible * 100 / result.Summary.TotalWorkloads
		latestPenalty := result.Summary.WithLatestTag * 2
		result.HealthScore = reproPct - latestPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildReproducibilityRecs1894(&result)
	writeJSON(w, result)
}

func isSensitiveEnvKey1894(key string) bool {
	lower := strings.ToLower(key)
	sensitive := []string{"password", "passwd", "secret", "token", "key", "api_key", "apikey", "credential", "auth"}
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func buildReproducibilityRecs1894(result *DeployReproducibilityResult) []string {
	recs := []string{
		fmt.Sprintf("Reproducibility: %d workloads (%d fully reproducible, %d non-reproducible)",
			result.Summary.TotalWorkloads, result.Summary.FullyReproducible, result.Summary.NonReproducible),
	}
	if result.Summary.WithLatestTag > 0 {
		recs = append(recs, fmt.Sprintf("%d containers use :latest tag - pin specific version for reproducible builds", result.Summary.WithLatestTag))
	}
	if result.Summary.WithNoTag > 0 {
		recs = append(recs, fmt.Sprintf("%d containers have no tag at all - always specify explicit image tags", result.Summary.WithNoTag))
	}
	if result.Summary.NonReproducible > 0 {
		recs = append(recs, fmt.Sprintf("%d non-reproducible workloads - fix image tags and externalize hardcoded env vars", result.Summary.NonReproducible))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Update Compliance Deep
// ---------------------------------------------------------------

type UpdateComplianceResult struct {
	ScannedAt         time.Time               `json:"scannedAt"`
	HealthScore       int                     `json:"healthScore"`
	Grade             string                  `json:"grade"`
	Summary           UpdateComplianceSummary `json:"summary"`
	ByWorkload        []UpdateComplianceEntry `json:"byWorkload"`
	NonCompliant      []UpdateComplianceEntry `json:"nonCompliant"`
	StrategyBreakdown map[string]int          `json:"strategyBreakdown"`
	Recommendations   []string                `json:"recommendations"`
}

type UpdateComplianceSummary struct {
	TotalWorkloads          int `json:"totalWorkloads"`
	Compliant               int `json:"compliant"`
	NonCompliant            int `json:"nonCompliant"`
	WithProgressDeadline    int `json:"withProgressDeadline"`
	WithoutProgressDeadline int `json:"withoutProgressDeadline"`
	WithMaxSurge            int `json:"withMaxSurge"`
	WithMaxUnavailable      int `json:"withMaxUnavailable"`
	RecreateStrategy        int `json:"recreateStrategy"`
	RollingStrategy         int `json:"rollingStrategy"`
}

type UpdateComplianceEntry struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	Kind             string   `json:"kind"`
	Strategy         string   `json:"strategy"`
	MaxSurge         string   `json:"maxSurge"`
	MaxUnavailable   string   `json:"maxUnavailable"`
	ProgressDeadline int32    `json:"progressDeadlineSeconds"`
	Replicas         int32    `json:"replicas"`
	Issues           []string `json:"issues"`
	RiskLevel        string   `json:"riskLevel"`
}

func (s *Server) handleUpdateComplianceDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := UpdateComplianceResult{
		ScannedAt:         time.Now(),
		StrategyBreakdown: map[string]int{},
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := UpdateComplianceEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
		}
		if dep.Spec.Replicas != nil {
			entry.Replicas = *dep.Spec.Replicas
		}

		// Strategy
		strategy := string(dep.Spec.Strategy.Type)
		entry.Strategy = strategy
		result.StrategyBreakdown[strategy]++

		if strategy == "RollingUpdate" {
			result.Summary.RollingStrategy++
			ru := dep.Spec.Strategy.RollingUpdate
			if ru != nil {
				if ru.MaxSurge != nil {
					entry.MaxSurge = ru.MaxSurge.String()
					result.Summary.WithMaxSurge++
				}
				if ru.MaxUnavailable != nil {
					entry.MaxUnavailable = ru.MaxUnavailable.String()
					result.Summary.WithMaxUnavailable++
				}
			}
		} else if strategy == "Recreate" {
			result.Summary.RecreateStrategy++
			entry.Issues = append(entry.Issues, "Recreate strategy causes downtime during updates")
			entry.RiskLevel = "high"
		}

		// Progress deadline
		if dep.Spec.ProgressDeadlineSeconds != nil {
			entry.ProgressDeadline = *dep.Spec.ProgressDeadlineSeconds
			result.Summary.WithProgressDeadline++
		} else {
			result.Summary.WithoutProgressDeadline++
			entry.Issues = append(entry.Issues, "no progressDeadlineSeconds set (default 600s)")
		}

		// Check compliance rules
		compliant := true
		if strategy == "Recreate" {
			compliant = false
		}
		if dep.Spec.ProgressDeadlineSeconds == nil {
			entry.Issues = append(entry.Issues, "missing progress deadline for failure detection")
			compliant = false
		}
		// Check for revision history limit
		if dep.Spec.RevisionHistoryLimit == nil || *dep.Spec.RevisionHistoryLimit < 3 {
			entry.Issues = append(entry.Issues, "revision history < 3 - limited rollback capability")
			compliant = false
		}

		if len(entry.Issues) == 0 {
			entry.RiskLevel = "low"
		} else if entry.RiskLevel == "" {
			entry.RiskLevel = "medium"
		}

		if compliant {
			result.Summary.Compliant++
		} else {
			result.Summary.NonCompliant++
			result.NonCompliant = append(result.NonCompliant, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.Compliant * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildUpdateComplianceRecs1894(&result)
	writeJSON(w, result)
}

func buildUpdateComplianceRecs1894(result *UpdateComplianceResult) []string {
	recs := []string{
		fmt.Sprintf("Update compliance: %d workloads, %d compliant (%d%%), %d non-compliant",
			result.Summary.TotalWorkloads, result.Summary.Compliant,
			safePercent1891(result.Summary.Compliant, result.Summary.TotalWorkloads),
			result.Summary.NonCompliant),
	}
	if result.Summary.RecreateStrategy > 0 {
		recs = append(recs, fmt.Sprintf("%d Deployments use Recreate strategy (downtime) - switch to RollingUpdate", result.Summary.RecreateStrategy))
	}
	if result.Summary.WithoutProgressDeadline > 0 {
		recs = append(recs, fmt.Sprintf("%d Deployments missing progressDeadlineSeconds - set to detect stuck rollouts", result.Summary.WithoutProgressDeadline))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Container Restart Policy Deep
// ---------------------------------------------------------------

type RestartPolicyDeepResult struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         RestartPolicyDeepSummary `json:"summary"`
	ByWorkload      []RestartPolicyEntry     `json:"byWorkload"`
	Misconfigured   []RestartPolicyEntry     `json:"misconfigured"`
	RestartHistory  []RestartHistoryEntry    `json:"restartHistory"`
	PolicyBreakdown map[string]int           `json:"policyBreakdown"`
	Recommendations []string                 `json:"recommendations"`
}

type RestartPolicyDeepSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	AlwaysPolicy    int `json:"alwaysPolicy"`
	OnFailurePolicy int `json:"onFailurePolicy"`
	NeverPolicy     int `json:"neverPolicy"`
	Misconfigured   int `json:"misconfigured"`
	HighRestartRate int `json:"highRestartRate"`
	TotalRestarts   int `json:"totalRestarts"`
	WithLiveness    int `json:"withLivenessProbe"`
	WithoutLiveness int `json:"withoutLivenessProbe"`
}

type RestartPolicyEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	RestartPolicy string `json:"restartPolicy"`
	Replicas      int32  `json:"replicas"`
	HasLiveness   bool   `json:"hasLivenessProbe"`
	HasReadiness  bool   `json:"hasReadinessProbe"`
	HasPreStop    bool   `json:"hasPreStopHook"`
	RiskLevel     string `json:"riskLevel"`
	Issue         string `json:"issue,omitempty"`
}

type RestartHistoryEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	PodName      string `json:"podName"`
	RestartCount int    `json:"restartCount"`
	LastRestart  string `json:"lastRestartTime"`
	RiskLevel    string `json:"riskLevel"`
}

func (s *Server) handleRestartPolicyDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RestartPolicyDeepResult{
		ScannedAt:       time.Now(),
		PolicyBreakdown: map[string]int{},
	}

	// Analyze deployments for policy + probe configuration
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		policy := string(dep.Spec.Template.Spec.RestartPolicy)
		if policy == "" {
			policy = "Always"
		}
		result.PolicyBreakdown[policy]++
		switch policy {
		case "Always":
			result.Summary.AlwaysPolicy++
		case "OnFailure":
			result.Summary.OnFailurePolicy++
		case "Never":
			result.Summary.NeverPolicy++
		}

		entry := RestartPolicyEntry{
			Name:          dep.Name,
			Namespace:     dep.Namespace,
			Kind:          "Deployment",
			RestartPolicy: policy,
		}
		if dep.Spec.Replicas != nil {
			entry.Replicas = *dep.Spec.Replicas
		}

		// Check probes
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.LivenessProbe != nil {
				entry.HasLiveness = true
				result.Summary.WithLiveness++
			} else {
				result.Summary.WithoutLiveness++
			}
			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
			}
		}
		if hasPreStopHook1894(dep.Spec.Template.Spec.Containers) {
			entry.HasPreStop = true
		}

		// Compliance checks
		if policy == "OnFailure" || policy == "Never" {
			entry.Issue = "Deployment with " + policy + " restart policy - Deployments should use Always"
			entry.RiskLevel = "high"
			result.Summary.Misconfigured++
			result.Misconfigured = append(result.Misconfigured, entry)
		} else if !entry.HasLiveness {
			entry.Issue = "no liveness probe - restart policy cannot detect hung containers"
			entry.RiskLevel = "medium"
			result.Summary.Misconfigured++
			result.Misconfigured = append(result.Misconfigured, entry)
		} else {
			entry.RiskLevel = "low"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Analyze pod restart history
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 0 {
				result.Summary.TotalRestarts += int(cs.RestartCount)
				entry := RestartHistoryEntry{
					Name:         pod.Name,
					Namespace:    pod.Namespace,
					PodName:      pod.Name,
					RestartCount: int(cs.RestartCount),
				}
				if cs.LastTerminationState.Terminated != nil {
					entry.LastRestart = cs.LastTerminationState.Terminated.FinishedAt.Format(time.RFC3339)
				}
				switch {
				case cs.RestartCount >= 20:
					entry.RiskLevel = "critical"
					result.Summary.HighRestartRate++
				case cs.RestartCount >= 5:
					entry.RiskLevel = "high"
					result.Summary.HighRestartRate++
				case cs.RestartCount >= 2:
					entry.RiskLevel = "medium"
				default:
					entry.RiskLevel = "low"
				}
				result.RestartHistory = append(result.RestartHistory, entry)
			}
		}
	}

	// Sort restart history by count descending
	sort.Slice(result.RestartHistory, func(i, j int) bool {
		return result.RestartHistory[i].RestartCount > result.RestartHistory[j].RestartCount
	})
	if len(result.RestartHistory) > 30 {
		result.RestartHistory = result.RestartHistory[:30]
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		okCount := result.Summary.TotalWorkloads - result.Summary.Misconfigured
		result.HealthScore = okCount * 100 / result.Summary.TotalWorkloads
	}
	// Penalty for high restart rates
	if result.Summary.HighRestartRate > 0 {
		result.HealthScore -= 15
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildRestartPolicyRecs1894(&result)
	writeJSON(w, result)
}

func hasPreStopHook1894(containers []corev1.Container) bool {
	for _, c := range containers {
		if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
			return true
		}
	}
	return false
}

func buildRestartPolicyRecs1894(result *RestartPolicyDeepResult) []string {
	recs := []string{
		fmt.Sprintf("Restart policy health: %d workloads, %d misconfigured, %d high-restart pods, %d total restarts",
			result.Summary.TotalWorkloads, result.Summary.Misconfigured,
			result.Summary.HighRestartRate, result.Summary.TotalRestarts),
	}
	if result.Summary.WithoutLiveness > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without liveness probe - restart policy cannot detect and recover from hung states", result.Summary.WithoutLiveness))
	}
	if result.Summary.Misconfigured > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with misconfigured restart policy or missing probes", result.Summary.Misconfigured))
	}
	if result.Summary.HighRestartRate > 0 {
		recs = append(recs, fmt.Sprintf("%d pods with high restart rate (>=5 restarts) - investigate CrashLoopBackOff or OOM patterns", result.Summary.HighRestartRate))
	}
	return recs
}

// keep reference to avoid unused import
var _ = appsv1.SchemeGroupVersion
