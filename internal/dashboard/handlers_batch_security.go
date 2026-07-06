package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BSResult is the CronJob & batch job security analysis.
type BSResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         BSSummary `json:"summary"`
	CronJobs        []BSEntry `json:"cronJobs"`
	OneShotJobs     []BSEntry `json:"oneShotJobs"`
	HighRisk        []BSEntry `json:"highRisk"`     // privileged or host access
	PrivilegedSA    []BSEntry `json:"privilegedSA"` // using SA with known wide RBAC
	Suspicious      []BSEntry `json:"suspicious"`   // every-minute schedule = persistence
	Issues          []BSIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// BSSummary aggregates batch security statistics.
type BSSummary struct {
	TotalCronJobs      int `json:"totalCronJobs"`
	TotalJobs          int `json:"totalJobs"`
	Privileged         int `json:"privileged"` // privileged containers
	HostPath           int `json:"hostPath"`   // hostPath mounts
	HostNetwork        int `json:"hostNetwork"`
	HostPID            int `json:"hostPID"`
	NoResourceLimits   int `json:"noResourceLimits"`
	NoSA               int `json:"noSA"` // uses default SA
	DefaultSA          int `json:"defaultSA"`
	SuspiciousSched    int `json:"suspiciousSchedule"` // every-minute or every-second
	NoConcurrencyLimit int `json:"noConcurrencyLimit"`
	MountsSecrets      int `json:"mountsSecrets"`
	SecurityScore      int `json:"securityScore"` // 0-100
}

// BSEntry describes one batch workload's security posture.
type BSEntry struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`               // CronJob / Job
	Schedule          string   `json:"schedule,omitempty"` // CronJob only
	Suspended         bool     `json:"suspended,omitempty"`
	ServiceAccount    string   `json:"serviceAccount"`
	IsDefaultSA       bool     `json:"isDefaultSA"`
	IsPrivileged      bool     `json:"isPrivileged"`
	HostPath          []string `json:"hostPathMounts,omitempty"`
	HasHostNetwork    bool     `json:"hasHostNetwork"`
	HasHostPID        bool     `json:"hasHostPID"`
	HasResourceLimits bool     `json:"hasResourceLimits"`
	ConcurrencyPolicy string   `json:"concurrencyPolicy,omitempty"`
	SecretCount       int      `json:"secretCount"`
	Violations        []string `json:"violations,omitempty"`
	RiskLevel         string   `json:"riskLevel"`
}

// BSIssue is a detected security problem.
type BSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleBatchSecurity audits CronJobs and Jobs for security risks.
// GET /api/security/batch-audit
func (s *Server) handleBatchSecurity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	cronJobs, err := rc.clientset.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	jobs, err := rc.clientset.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := BSResult{ScannedAt: time.Now()}

	// Analyze CronJobs
	for _, cj := range cronJobs.Items {
		result.Summary.TotalCronJobs++

		entry := bsAnalyzePodSpec(
			cj.Name,
			cj.Namespace,
			"CronJob",
			cj.Spec.JobTemplate.Spec.Template.Spec,
			cj.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName,
		)
		entry.Schedule = cj.Spec.Schedule
		entry.Suspended = cj.Spec.Suspend != nil && *cj.Spec.Suspend
		entry.ConcurrencyPolicy = string(cj.Spec.ConcurrencyPolicy)

		bsAnalyzeEntry(&entry, &result)
		result.CronJobs = append(result.CronJobs, entry)
	}

	// Analyze one-shot Jobs
	for _, job := range jobs.Items {
		// Skip Jobs owned by CronJobs (they're already analyzed)
		ownedByCronJob := false
		for _, ref := range job.OwnerReferences {
			if ref.Kind == "CronJob" {
				ownedByCronJob = true
				break
			}
		}
		if ownedByCronJob {
			continue
		}

		result.Summary.TotalJobs++

		entry := bsAnalyzePodSpec(
			job.Name,
			job.Namespace,
			"Job",
			job.Spec.Template.Spec,
			job.Spec.Template.Spec.ServiceAccountName,
		)

		bsAnalyzeEntry(&entry, &result)
		result.OneShotJobs = append(result.OneShotJobs, entry)
	}

	// Sort
	sort.Slice(result.HighRisk, func(i, j int) bool {
		return bsRiskRank(result.HighRisk[i].RiskLevel) < bsRiskRank(result.HighRisk[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return bsIssueRank(result.Issues[i].Severity) < bsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.SecurityScore = bsScore(result.Summary)
	result.Recommendations = bsGenRecs(result.Summary, result.HighRisk, result.Suspicious)

	writeJSON(w, result)
}

// bsAnalyzePodSpec extracts security-relevant info from a pod spec.
func bsAnalyzePodSpec(name, namespace, kind string, spec corev1.PodSpec, saName string) BSEntry {
	entry := BSEntry{
		Name:           name,
		Namespace:      namespace,
		Kind:           kind,
		ServiceAccount: saName,
		HasHostNetwork: spec.HostNetwork,
		HasHostPID:     spec.HostPID,
	}

	if saName == "" || saName == "default" {
		entry.IsDefaultSA = true
		entry.ServiceAccount = "default"
	}

	// Check containers for privileged, resource limits
	for _, c := range spec.Containers {
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			entry.IsPrivileged = true
		}
		if len(c.Resources.Limits) == 0 {
			// No limits
		} else {
			entry.HasResourceLimits = true
		}
	}

	// Check init containers too
	for _, c := range spec.InitContainers {
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			entry.IsPrivileged = true
		}
	}

	// Check volumes for hostPath and secrets
	for _, vol := range spec.Volumes {
		if vol.HostPath != nil {
			entry.HostPath = append(entry.HostPath, vol.HostPath.Path)
		}
		if vol.Secret != nil {
			entry.SecretCount++
		}
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.Secret != nil {
					entry.SecretCount++
				}
			}
		}
	}

	if !entry.HasResourceLimits && len(spec.Containers) > 0 {
		// Re-check: all containers must have limits for this to be true
		allHaveLimits := true
		for _, c := range spec.Containers {
			if len(c.Resources.Limits) == 0 {
				allHaveLimits = false
				break
			}
		}
		entry.HasResourceLimits = allHaveLimits
	}

	return entry
}

// bsAnalyzeEntry populates summary, violations, risk, and issues for one entry.
func bsAnalyzeEntry(entry *BSEntry, result *BSResult) {
	// Privileged
	if entry.IsPrivileged {
		result.Summary.Privileged++
		entry.Violations = append(entry.Violations, "Privileged container — full host access")
		result.HighRisk = append(result.HighRisk, *entry)
		result.Issues = append(result.Issues, BSIssue{
			Severity: "critical", Type: "privileged-batch",
			Resource: fmt.Sprintf("%s/%s", entry.Namespace, entry.Name),
			Message:  fmt.Sprintf("%s %s/%s runs privileged — full node compromise risk", entry.Kind, entry.Namespace, entry.Name),
		})
	}

	// HostPath
	if len(entry.HostPath) > 0 {
		result.Summary.HostPath++
		entry.Violations = append(entry.Violations, fmt.Sprintf("HostPath mounts: %s", strings.Join(entry.HostPath, ", ")))
		if !entry.IsPrivileged { // Don't double-add if already in highRisk
			result.HighRisk = append(result.HighRisk, *entry)
		}
		result.Issues = append(result.Issues, BSIssue{
			Severity: "critical", Type: "hostpath-batch",
			Resource: fmt.Sprintf("%s/%s", entry.Namespace, entry.Name),
			Message:  fmt.Sprintf("%s %s/%s mounts hostPath %s — can read/write node filesystem", entry.Kind, entry.Namespace, entry.Name, strings.Join(entry.HostPath, ", ")),
		})
	}

	// HostNetwork / HostPID
	if entry.HasHostNetwork {
		result.Summary.HostNetwork++
		entry.Violations = append(entry.Violations, "HostNetwork — shares node network namespace")
		result.Issues = append(result.Issues, BSIssue{
			Severity: "warning", Type: "hostnetwork-batch",
			Resource: fmt.Sprintf("%s/%s", entry.Namespace, entry.Name),
			Message:  fmt.Sprintf("%s %s/%s uses hostNetwork — can bind to node ports, sniff traffic", entry.Kind, entry.Namespace, entry.Name),
		})
	}
	if entry.HasHostPID {
		result.Summary.HostPID++
		entry.Violations = append(entry.Violations, "HostPID — shares node process namespace")
	}

	// Default SA
	if entry.IsDefaultSA {
		result.Summary.DefaultSA++
		entry.Violations = append(entry.Violations, "Uses default ServiceAccount — may inherit unintended RBAC permissions")
		result.PrivilegedSA = append(result.PrivilegedSA, *entry)
		result.Issues = append(result.Issues, BSIssue{
			Severity: "warning", Type: "default-sa-batch",
			Resource: fmt.Sprintf("%s/%s", entry.Namespace, entry.Name),
			Message:  fmt.Sprintf("%s %s/%s uses default ServiceAccount — create dedicated SA with minimal RBAC", entry.Kind, entry.Namespace, entry.Name),
		})
	}

	// No resource limits
	if !entry.HasResourceLimits {
		result.Summary.NoResourceLimits++
		entry.Violations = append(entry.Violations, "No resource limits — batch job can exhaust node resources")
	}

	// Secrets
	if entry.SecretCount > 0 {
		result.Summary.MountsSecrets++
		if entry.SecretCount > 3 {
			entry.Violations = append(entry.Violations, fmt.Sprintf("Mounts %d secrets — high credential exposure for a batch job", entry.SecretCount))
		}
	}

	// Suspicious schedule (every minute or more frequent)
	if entry.Schedule != "" && bsIsSuspiciousSchedule(entry.Schedule) {
		result.Summary.SuspiciousSched++
		entry.Violations = append(entry.Violations, fmt.Sprintf("Suspicious schedule '%s' — potential persistence mechanism", entry.Schedule))
		result.Suspicious = append(result.Suspicious, *entry)
		result.Issues = append(result.Issues, BSIssue{
			Severity: "warning", Type: "suspicious-schedule",
			Resource: fmt.Sprintf("%s/%s", entry.Namespace, entry.Name),
			Message:  fmt.Sprintf("CronJob %s/%s runs every minute or more frequently — verify this is legitimate, not persistence", entry.Namespace, entry.Name),
		})
	}

	// No concurrency limit for CronJobs (Allow = default, unlimited)
	if entry.Kind == "CronJob" && (entry.ConcurrencyPolicy == "" || entry.ConcurrencyPolicy == "Allow") {
		result.Summary.NoConcurrencyLimit++
		entry.Violations = append(entry.Violations, "No concurrency limit — overlapping runs can fork-bomb the cluster")
	}

	entry.RiskLevel = bsAssessRisk(*entry)
}

// bsIsSuspiciousSchedule checks if schedule runs too frequently.
func bsIsSuspiciousSchedule(schedule string) bool {
	// Check for every-minute patterns: "* * * * *" or "*/1 * * * *"
	schedule = strings.TrimSpace(schedule)
	if schedule == "* * * * *" || schedule == "*/1 * * * *" {
		return true
	}
	// Check for sub-minute or every-second (shouldn't exist in CronJob but check)
	if strings.Contains(schedule, "*/1 ") && strings.HasPrefix(schedule, "*/1") {
		return true
	}
	return false
}

// bsAssessRisk determines overall risk for a batch workload.
func bsAssessRisk(entry BSEntry) string {
	if entry.IsPrivileged || len(entry.HostPath) > 0 {
		return "critical"
	}
	if entry.HasHostNetwork || entry.HasHostPID {
		return "high"
	}
	if entry.IsDefaultSA {
		return "medium"
	}
	if !entry.HasResourceLimits {
		return "medium"
	}
	return "low"
}

// bsScore computes 0-100.
func bsScore(s BSSummary) int {
	total := s.TotalCronJobs + s.TotalJobs
	if total == 0 {
		return 100
	}
	score := 100
	score -= s.Privileged * 20
	score -= s.HostPath * 15
	score -= s.HostNetwork * 8
	score -= s.HostPID * 8
	score -= s.DefaultSA * 4
	score -= s.NoResourceLimits * 3
	score -= s.SuspiciousSched * 6
	if score < 0 {
		score = 0
	}
	return score
}

// bsGenRecs produces actionable advice.
func bsGenRecs(s BSSummary, highRisk []BSEntry, suspicious []BSEntry) []string {
	var recs []string

	if s.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) run privileged — remove privileged flag, use SecurityContextConstraints", s.Privileged))
	}
	if s.HostPath > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) mount hostPath — use PVCs or projected volumes instead", s.HostPath))
	}
	if s.HostNetwork > 0 || s.HostPID > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) use hostNetwork/hostPID — isolate from node, use network policies", s.HostNetwork+s.HostPID))
	}
	if s.DefaultSA > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) use default ServiceAccount — create dedicated SA with minimal RBAC per job", s.DefaultSA))
	}
	if s.NoResourceLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) have no resource limits — batch jobs can exhaust node CPU/memory", s.NoResourceLimits))
	}
	if s.SuspiciousSched > 0 {
		recs = append(recs, fmt.Sprintf("%d CronJob(s) have suspicious (every-minute) schedules — verify these are legitimate, not attacker persistence", s.SuspiciousSched))
	}
	if s.NoConcurrencyLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d CronJob(s) have no concurrency limit — set concurrencyPolicy: Forbid or Replace", s.NoConcurrencyLimit))
	}
	if s.MountsSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d batch workload(s) mount secrets — verify each secret is necessary and uses minimal scope", s.MountsSecrets))
	}
	if s.SecurityScore < 60 {
		recs = append(recs, fmt.Sprintf("Batch security score is %d/100 — multiple high-risk batch workloads detected", s.SecurityScore))
	}
	if s.Privileged == 0 && s.HostPath == 0 && s.SuspiciousSched == 0 {
		recs = append(recs, "No high-risk batch workloads detected — good security posture")
	}

	return recs
}

func bsRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func bsIssueRank(s string) int {
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

var _ = batchv1.CronJobSpec{}
