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

// DeployAuditSeverity ranks the importance of a configuration finding.
type DeployAuditSeverity string

const (
	DeployAuditCritical DeployAuditSeverity = "critical" // will cause outages or security incidents
	DeployAuditWarning  DeployAuditSeverity = "warning"  // best-practice violation, should fix
	DeployAuditInfo     DeployAuditSeverity = "info"     // minor or informational
)

// DeployAuditCategory groups related checks.
type DeployAuditCategory string

const (
	CatRevisionHistory DeployAuditCategory = "revision-history"
	CatImagePolicy     DeployAuditCategory = "image-policy"
	CatResources       DeployAuditCategory = "resources"
	CatProbes          DeployAuditCategory = "probes"
	CatSecurity        DeployAuditCategory = "security-context"
	CatStrategy        DeployAuditCategory = "update-strategy"
	CatLifecycle       DeployAuditCategory = "lifecycle"
	CatConfigDrift     DeployAuditCategory = "config-drift"
)

// DeployAuditFinding describes a single configuration issue in a workload.
type DeployAuditFinding struct {
	Category   DeployAuditCategory `json:"category"`
	Severity   DeployAuditSeverity `json:"severity"`
	Check      string              `json:"check"`      // short check name
	Message    string              `json:"message"`    // human-readable description
	Suggestion string              `json:"suggestion"` // recommended fix
}

// DeployAuditWorkload represents one workload with its audit findings.
type DeployAuditWorkload struct {
	Kind            string               `json:"kind"` // Deployment, StatefulSet, DaemonSet
	Name            string               `json:"name"`
	Namespace       string               `json:"namespace"`
	Images          []string             `json:"images"`
	Replicas        int32                `json:"replicas"`
	ReadyReplicas   int32                `json:"readyReplicas"`
	AgeHours        float64              `json:"ageHours"`
	SinceUpdateHrs  float64              `json:"sinceUpdateHours"`
	RevisionHistory int32                `json:"revisionHistoryLimit"`
	Findings        []DeployAuditFinding `json:"findings"`
	Score           int                  `json:"score"` // 0 (perfect) to 100 (worst)
}

// DeployAuditResult is the full scan output.
type DeployAuditResult struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     DeployAuditSummary          `json:"summary"`
	Workloads   []DeployAuditWorkload       `json:"workloads"`
	TopFindings []DeployAuditFindingSummary `json:"topFindings"`
}

// DeployAuditSummary aggregates statistics.
type DeployAuditSummary struct {
	Total         int `json:"total"`
	Deployments   int `json:"deployments"`
	StatefulSets  int `json:"statefulSets"`
	DaemonSets    int `json:"daemonSets"`
	WithFindings  int `json:"withFindings"`
	CriticalCount int `json:"criticalCount"`
	WarningCount  int `json:"warningCount"`
	InfoCount     int `json:"infoCount"`
	AvgScore      int `json:"avgScore"` // average health score (lower = worse)
}

// DeployAuditFindingSummary aggregates counts per finding type.
type DeployAuditFindingSummary struct {
	Category DeployAuditCategory `json:"category"`
	Check    string              `json:"check"`
	Severity DeployAuditSeverity `json:"severity"`
	Count    int                 `json:"count"`
}

// handleDeployAudit scans all Deployments, StatefulSets, and DaemonSets for
// configuration best-practice violations that affect deployment reliability.
// GET /api/deployments/audit?namespace=xxx&severity=critical
func (s *Server) handleDeployAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ns := r.URL.Query().Get("namespace")
	severityFilter := r.URL.Query().Get("severity")

	result := DeployAuditResult{
		ScannedAt: time.Now(),
	}

	// --- Deployments ---
	depList, err := rc.clientset.AppsV1().Deployments(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range depList.Items {
		wa := auditDeployment(&depList.Items[i])
		result.Workloads = append(result.Workloads, wa)
	}

	// --- StatefulSets ---
	stsList, err := rc.clientset.AppsV1().StatefulSets(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range stsList.Items {
		wa := auditStatefulSet(&stsList.Items[i])
		result.Workloads = append(result.Workloads, wa)
	}

	// --- DaemonSets ---
	dsList, err := rc.clientset.AppsV1().DaemonSets(ns).List(r.Context(), metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	for i := range dsList.Items {
		wa := auditDaemonSet(&dsList.Items[i])
		result.Workloads = append(result.Workloads, wa)
	}

	// Compute summaries
	totalScore := 0
	findingCounts := make(map[string]*DeployAuditFindingSummary)
	for _, wa := range result.Workloads {
		result.Summary.Total++
		switch wa.Kind {
		case "Deployment":
			result.Summary.Deployments++
		case "StatefulSet":
			result.Summary.StatefulSets++
		case "DaemonSet":
			result.Summary.DaemonSets++
		}
		if len(wa.Findings) > 0 {
			result.Summary.WithFindings++
		}
		for _, f := range wa.Findings {
			switch f.Severity {
			case DeployAuditCritical:
				result.Summary.CriticalCount++
			case DeployAuditWarning:
				result.Summary.WarningCount++
			case DeployAuditInfo:
				result.Summary.InfoCount++
			}
			key := fmt.Sprintf("%s|%s", f.Category, f.Check)
			if existing, ok := findingCounts[key]; ok {
				existing.Count++
			} else {
				findingCounts[key] = &DeployAuditFindingSummary{
					Category: f.Category,
					Check:    f.Check,
					Severity: f.Severity,
					Count:    1,
				}
			}
		}
		totalScore += wa.Score
	}
	if result.Summary.Total > 0 {
		result.Summary.AvgScore = totalScore / result.Summary.Total
	}

	// Collect top findings
	for _, fc := range findingCounts {
		result.TopFindings = append(result.TopFindings, *fc)
	}
	sort.Slice(result.TopFindings, func(i, j int) bool {
		if result.TopFindings[i].Severity != result.TopFindings[j].Severity {
			return deployAuditSevRank(result.TopFindings[i].Severity) < deployAuditSevRank(result.TopFindings[j].Severity)
		}
		return result.TopFindings[i].Count > result.TopFindings[j].Count
	})

	// Sort workloads by score descending (worst first), then by name
	sort.Slice(result.Workloads, func(i, j int) bool {
		if result.Workloads[i].Score != result.Workloads[j].Score {
			return result.Workloads[i].Score > result.Workloads[j].Score
		}
		return result.Workloads[i].Name < result.Workloads[j].Name
	})

	// Apply severity filter
	if severityFilter != "" {
		filtered := make([]DeployAuditWorkload, 0, len(result.Workloads))
		for _, wa := range result.Workloads {
			for _, f := range wa.Findings {
				if string(f.Severity) == severityFilter {
					filtered = append(filtered, wa)
					break
				}
			}
		}
		result.Workloads = filtered
	}

	writeJSON(w, result)
}

func deployAuditSevRank(s DeployAuditSeverity) int {
	switch s {
	case DeployAuditCritical:
		return 0
	case DeployAuditWarning:
		return 1
	case DeployAuditInfo:
		return 2
	}
	return 9
}

// auditDeployment evaluates a Deployment for configuration issues.
func auditDeployment(dep *appsv1.Deployment) DeployAuditWorkload {
	wa := DeployAuditWorkload{
		Kind:            "Deployment",
		Name:            dep.Name,
		Namespace:       dep.Namespace,
		Images:          extractImages(dep.Spec.Template.Spec.Containers),
		Replicas:        derefInt32(dep.Spec.Replicas),
		ReadyReplicas:   dep.Status.ReadyReplicas,
		AgeHours:        hoursSince(dep.CreationTimestamp),
		SinceUpdateHrs:  deploymentSinceUpdate(dep),
		RevisionHistory: derefInt32(dep.Spec.RevisionHistoryLimit),
	}
	podSpec := &dep.Spec.Template.Spec
	auditContainerSpec(&wa, podSpec)
	auditRevisionHistory(&wa, dep.Spec.RevisionHistoryLimit)
	auditUpdateStrategy(&wa, string(dep.Spec.Strategy.Type), dep.Spec.Strategy.RollingUpdate)
	auditLifecycle(&wa, podSpec)
	auditPodSecurityContext(&wa, podSpec)
	return wa
}

// auditStatefulSet evaluates a StatefulSet for configuration issues.
func auditStatefulSet(sts *appsv1.StatefulSet) DeployAuditWorkload {
	wa := DeployAuditWorkload{
		Kind:            "StatefulSet",
		Name:            sts.Name,
		Namespace:       sts.Namespace,
		Images:          extractImages(sts.Spec.Template.Spec.Containers),
		Replicas:        derefInt32(sts.Spec.Replicas),
		ReadyReplicas:   sts.Status.ReadyReplicas,
		AgeHours:        hoursSince(sts.CreationTimestamp),
		SinceUpdateHrs:  hoursSince(sts.CreationTimestamp),
		RevisionHistory: derefInt32(sts.Spec.RevisionHistoryLimit),
	}
	podSpec := &sts.Spec.Template.Spec
	auditContainerSpec(&wa, podSpec)
	auditRevisionHistory(&wa, sts.Spec.RevisionHistoryLimit)
	auditStatefulSetStrategy(&wa, sts.Spec.UpdateStrategy)
	auditLifecycle(&wa, podSpec)
	auditPodSecurityContext(&wa, podSpec)
	return wa
}

// auditDaemonSet evaluates a DaemonSet for configuration issues.
func auditDaemonSet(ds *appsv1.DaemonSet) DeployAuditWorkload {
	wa := DeployAuditWorkload{
		Kind:            "DaemonSet",
		Name:            ds.Name,
		Namespace:       ds.Namespace,
		Images:          extractImages(ds.Spec.Template.Spec.Containers),
		Replicas:        ds.Status.DesiredNumberScheduled,
		ReadyReplicas:   ds.Status.NumberReady,
		AgeHours:        hoursSince(ds.CreationTimestamp),
		SinceUpdateHrs:  hoursSince(ds.CreationTimestamp),
		RevisionHistory: derefInt32(ds.Spec.RevisionHistoryLimit),
	}
	podSpec := &ds.Spec.Template.Spec
	auditContainerSpec(&wa, podSpec)
	auditRevisionHistory(&wa, ds.Spec.RevisionHistoryLimit)
	auditLifecycle(&wa, podSpec)
	auditPodSecurityContext(&wa, podSpec)
	return wa
}

// --- Individual audit checks ---

// auditContainerSpec checks probes, resources, image policies, and security contexts.
func auditContainerSpec(wa *DeployAuditWorkload, spec *corev1.PodSpec) {
	for _, c := range spec.Containers {
		// Missing probes
		if c.LivenessProbe == nil {
			wa.addFinding(DeployAuditWarning, CatProbes, "missing-liveness-probe",
				fmt.Sprintf("Container %q has no liveness probe", c.Name),
				"Add a liveness probe so Kubernetes can detect and restart unhealthy containers")
		}
		if c.ReadinessProbe == nil {
			wa.addFinding(DeployAuditWarning, CatProbes, "missing-readiness-probe",
				fmt.Sprintf("Container %q has no readiness probe", c.Name),
				"Add a readiness probe so traffic is only sent to containers that are ready to serve")
		}
		if c.StartupProbe == nil {
			wa.addFinding(DeployAuditInfo, CatProbes, "missing-startup-probe",
				fmt.Sprintf("Container %q has no startup probe", c.Name),
				"Add a startup probe for slow-starting containers to avoid premature liveness-probe kills")
		}

		// Resource limits and requests
		if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
			wa.addFinding(DeployAuditCritical, CatResources, "missing-resource-limits",
				fmt.Sprintf("Container %q has no resource limits", c.Name),
				"Set CPU and memory limits to prevent a single container from exhausting node resources")
		}
		if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
			wa.addFinding(DeployAuditWarning, CatResources, "missing-resource-requests",
				fmt.Sprintf("Container %q has no resource requests", c.Name),
				"Set resource requests to ensure proper scheduling and QoS class")
		}

		// Image pull policy
		auditImagePullPolicy(wa, c)

		// Security context
		if c.SecurityContext == nil {
			wa.addFinding(DeployAuditWarning, CatSecurity, "missing-security-context",
				fmt.Sprintf("Container %q has no security context", c.Name),
				"Set securityContext to runAsNonRoot, drop capabilities, and set readOnlyRootFilesystem")
			wa.addFinding(DeployAuditWarning, CatSecurity, "runs-as-root",
				fmt.Sprintf("Container %q has no security context and defaults to running as root", c.Name),
				"Set runAsNonRoot: true and specify a non-zero runAsUser")
		} else {
			sc := c.SecurityContext
			if sc.Privileged != nil && *sc.Privileged {
				wa.addFinding(DeployAuditCritical, CatSecurity, "privileged-container",
					fmt.Sprintf("Container %q is running in privileged mode", c.Name),
					"Remove privileged mode — it grants full host access")
			}
			if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				wa.addFinding(DeployAuditWarning, CatSecurity, "runs-as-root",
					fmt.Sprintf("Container %q may run as root (runAsNonRoot not set or false)", c.Name),
					"Set runAsNonRoot: true and specify a non-zero runAsUser")
			}
			if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				wa.addFinding(DeployAuditInfo, CatSecurity, "writable-root-filesystem",
					fmt.Sprintf("Container %q has a writable root filesystem", c.Name),
					"Set readOnlyRootFilesystem: true and mount writable paths as volumes")
			}
			if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
				wa.addFinding(DeployAuditWarning, CatSecurity, "privilege-escalation",
					fmt.Sprintf("Container %q allows privilege escalation", c.Name),
					"Set allowPrivilegeEscalation: false to prevent gaining more privileges")
			}
		}
	}

	// Also check init containers (less strict on probes)
	for _, c := range spec.InitContainers {
		auditImagePullPolicy(wa, c)
		if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
			wa.addFinding(DeployAuditInfo, CatResources, "missing-init-resource-limits",
				fmt.Sprintf("Init container %q has no resource limits", c.Name),
				"Set resource limits on init containers to prevent resource contention during startup")
		}
	}
}

// auditImagePullPolicy checks that image pull policy matches the image tag.
func auditImagePullPolicy(wa *DeployAuditWorkload, c corev1.Container) {
	image := c.Image
	if image == "" {
		return
	}
	// Determine if the image uses :latest or no tag (defaults to :latest)
	parts := strings.Split(image, ":")
	tag := ""
	if len(parts) > 1 {
		tag = parts[len(parts)-1]
		// Strip digest (sha256:...)
		if strings.Contains(tag, "@") {
			tag = strings.Split(tag, "@")[0]
		}
	}
	isLatest := tag == "" || tag == "latest"

	policy := c.ImagePullPolicy
	if policy == "" {
		// Default: Always for :latest, IfNotPresent otherwise
		if isLatest {
			policy = corev1.PullAlways
		} else {
			policy = corev1.PullIfNotPresent
		}
	}

	if isLatest && policy != corev1.PullAlways {
		wa.addFinding(DeployAuditWarning, CatImagePolicy, "latest-tag-without-always",
			fmt.Sprintf("Container %q uses :latest tag but imagePullPolicy is %q", c.Name, policy),
			"Either pin the image to a specific tag, or set imagePullPolicy: Always for :latest tags")
	}
	if !isLatest && policy == corev1.PullAlways {
		wa.addFinding(DeployAuditInfo, CatImagePolicy, "pinned-tag-with-always",
			fmt.Sprintf("Container %q uses pinned tag but imagePullPolicy is Always", c.Name),
			"For pinned image tags, use imagePullPolicy: IfNotPresent to reduce registry load")
	}
}

// auditRevisionHistory checks the revision history limit is reasonable.
func auditRevisionHistory(wa *DeployAuditWorkload, limit *int32) {
	l := int32(10) // Kubernetes default
	if limit != nil {
		l = *limit
	}
	if l < 2 {
		wa.addFinding(DeployAuditCritical, CatRevisionHistory, "insufficient-revision-history",
			fmt.Sprintf("Revision history limit is %d — cannot rollback more than %d revision(s)", l, l),
			"Set revisionHistoryLimit to at least 3 to enable safe rollback")
	}
	if l > 20 {
		wa.addFinding(DeployAuditInfo, CatRevisionHistory, "excessive-revision-history",
			fmt.Sprintf("Revision history limit is %d — old ReplicaSets consume resources", l),
			"Reduce revisionHistoryLimit to 10 (the default) unless you need extensive rollback history")
	}
}

// auditUpdateStrategy checks the Deployment rolling update parameters.
func auditUpdateStrategy(wa *DeployAuditWorkload, strategyType string, ru *appsv1.RollingUpdateDeployment) {
	if strategyType == "Recreate" {
		wa.addFinding(DeployAuditWarning, CatStrategy, "recreate-strategy",
			"Deployment uses Recreate strategy — causes downtime during updates",
			"Use RollingUpdate strategy for zero-downtime deployments")
		return
	}
	if ru != nil {
		if ru.MaxUnavailable != nil {
			maxUnavail := ru.MaxUnavailable.String()
			if maxUnavail == "100%" || maxUnavail == "1" {
				if wa.Replicas <= 2 {
					wa.addFinding(DeployAuditWarning, CatStrategy, "aggressive-max-unavailable",
						fmt.Sprintf("maxUnavailable=%s with only %d replicas may cause downtime", maxUnavail, wa.Replicas),
						"Reduce maxUnavailable or increase replicas to maintain availability during rollouts")
				}
			}
		}
	}
}

// auditStatefulSetStrategy checks the StatefulSet update strategy.
func auditStatefulSetStrategy(wa *DeployAuditWorkload, strategy appsv1.StatefulSetUpdateStrategy) {
	if strategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		wa.addFinding(DeployAuditWarning, CatStrategy, "ondelete-strategy",
			"StatefulSet uses OnDelete strategy — pods must be manually deleted to apply updates",
			"Use RollingUpdate strategy for automatic pod updates")
	}
	if strategy.RollingUpdate != nil && strategy.RollingUpdate.Partition != nil {
		partition := *strategy.RollingUpdate.Partition
		if partition > 0 {
			wa.addFinding(DeployAuditInfo, CatStrategy, "partitioned-rollout",
				fmt.Sprintf("StatefulSet has partition=%d — only pods with ordinal >= %d will be updated", partition, partition),
				"Reset partition to 0 to complete the rolling update")
		}
	}
}

// auditLifecycle checks termination grace period and preStop hooks.
func auditLifecycle(wa *DeployAuditWorkload, spec *corev1.PodSpec) {
	grpc := int64(30)
	if spec.TerminationGracePeriodSeconds != nil {
		grpc = *spec.TerminationGracePeriodSeconds
	}
	if grpc < 10 {
		wa.addFinding(DeployAuditCritical, CatLifecycle, "short-grace-period",
			fmt.Sprintf("terminationGracePeriodSeconds is %d — too short for graceful shutdown", grpc),
			"Increase to at least 30s to allow in-flight requests to complete")
	}
	if grpc > 300 {
		wa.addFinding(DeployAuditInfo, CatLifecycle, "long-grace-period",
			fmt.Sprintf("terminationGracePeriodSeconds is %d — excessively long", grpc),
			"Reduce to 30-60s unless the workload needs extended cleanup time")
	}

	// Check for preStop hook (important for zero-downtime)
	for _, c := range spec.Containers {
		if c.Lifecycle == nil || c.Lifecycle.PreStop == nil {
			wa.addFinding(DeployAuditInfo, CatLifecycle, "missing-prestop-hook",
				fmt.Sprintf("Container %q has no preStop hook", c.Name),
				"Add a preStop hook to deregister from load balancers before shutdown")
			break // report once
		}
	}
}

// auditPodSecurityContext checks the pod-level security context.
func auditPodSecurityContext(wa *DeployAuditWorkload, spec *corev1.PodSpec) {
	if spec.SecurityContext == nil {
		wa.addFinding(DeployAuditInfo, CatSecurity, "missing-pod-security-context",
			"No pod-level security context defined",
			"Set pod-level securityContext with runAsNonRoot, seccompProfile, and fsGroup")
		return
	}
	sc := spec.SecurityContext
	if sc.SeccompProfile == nil {
		wa.addFinding(DeployAuditInfo, CatSecurity, "missing-seccomp-profile",
			"No seccomp profile defined at pod level",
			"Set seccompProfile type: RuntimeDefault to restrict system calls")
	}
}

// addFinding adds a finding and updates the health score.
func (wa *DeployAuditWorkload) addFinding(severity DeployAuditSeverity, category DeployAuditCategory, check, msg, suggestion string) {
	wa.Findings = append(wa.Findings, DeployAuditFinding{
		Severity:   severity,
		Category:   category,
		Check:      check,
		Message:    msg,
		Suggestion: suggestion,
	})
	switch severity {
	case DeployAuditCritical:
		wa.Score += 20
	case DeployAuditWarning:
		wa.Score += 8
	case DeployAuditInfo:
		wa.Score += 2
	}
}

// derefInt32 safely dereferences an *int32, returning 0 if nil.
func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// deploymentSinceUpdate estimates hours since the last deployment update.
func deploymentSinceUpdate(dep *appsv1.Deployment) float64 {
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionTrue {
			if !c.LastUpdateTime.IsZero() {
				return time.Since(c.LastUpdateTime.Time).Hours()
			}
		}
	}
	return hoursSince(dep.CreationTimestamp)
}
