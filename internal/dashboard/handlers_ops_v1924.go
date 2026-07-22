package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.24 — Operations Dimension (Round 7)
// 1. Ingress Health Monitor — rule conflicts, TLS coverage
// 2. Job Lifecycle Tracker — completion/failure/staleness
// 3. Leader Election Health — lease monitoring & failover readiness
// ============================================================

// ---------------------------------------------------------------
// 1. Ingress Health Monitor — rule conflicts, TLS coverage
// ---------------------------------------------------------------

type IngressHealthResult1924 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         IngressHealthSummary1924 `json:"summary"`
	Ingresses       []IngressEntry1924       `json:"ingresses"`
	Conflicts       []IngressConflict1924    `json:"conflicts"`
	Issues          []IngressIssue1924       `json:"issues"`
	Recommendations []string                 `json:"recommendations"`
}

type IngressHealthSummary1924 struct {
	TotalIngresses    int  `json:"totalIngresses"`
	WithTLS           int  `json:"withTLS"`
	WithoutTLS        int  `json:"withoutTLS"`
	HostConflicts     int  `json:"hostConflicts"`
	PathConflicts     int  `json:"pathConflicts"`
	OrphanedIngress   int  `json:"orphanedIngress"`
	HasDefaultBackend bool `json:"hasDefaultBackend"`
}

type IngressEntry1924 struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Hosts      []string `json:"hosts"`
	Paths      []string `json:"paths"`
	HasTLS     bool     `json:"hasTLS"`
	TLSSecrets []string `json:"tlsSecrets"`
	Backend    string   `json:"backend"`
	Age        string   `json:"age"`
}

type IngressConflict1924 struct {
	Type       string `json:"type"`
	Host       string `json:"host"`
	Path       string `json:"path"`
	Ingress1   string `json:"ingress1"`
	Ingress2   string `json:"ingress2"`
	Namespace1 string `json:"namespace1"`
	Namespace2 string `json:"namespace2"`
}

type IngressIssue1924 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleIngressHealthMonitor(w http.ResponseWriter, r *http.Request) {
	result := IngressHealthResult1924{
		ScannedAt: time.Now(),
	}
	score := 100

	ingList, err := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Build host/path map for conflict detection
	type hostPathKey struct{ host, path string }
	hostPathOwners := make(map[hostPathKey][]IngressEntry1924)

	for _, ing := range ingList.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		hosts := make([]string, 0)
		paths := make([]string, 0)
		hasTLS := len(ing.Spec.TLS) > 0
		tlsSecrets := make([]string, 0)
		for _, tls := range ing.Spec.TLS {
			tlsSecrets = append(tlsSecrets, tls.SecretName)
			hosts = append(hosts, tls.Hosts...)
		}
		backend := ""
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hosts = append(hosts, rule.Host)
			}
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					p := path.Path
					if p == "" {
						p = "/"
					}
					paths = append(paths, p)
					if path.Backend.Service != nil {
						backend = fmt.Sprintf("%s/%s:%d", ing.Namespace, path.Backend.Service.Name, path.Backend.Service.Port.Number)
					}
					hk := hostPathKey{host: rule.Host, path: p}
					entry := IngressEntry1924{
						Name: ing.Name, Namespace: ing.Namespace,
						Hosts: hosts, Paths: paths,
						HasTLS: hasTLS, TLSSecrets: tlsSecrets,
						Backend: backend,
						Age:     fmt.Sprintf("%.0fd", time.Since(ing.CreationTimestamp.Time).Hours()/24),
					}
					hostPathOwners[hk] = append(hostPathOwners[hk], entry)
				}
			}
		}

		entry := IngressEntry1924{
			Name:       ing.Name,
			Namespace:  ing.Namespace,
			Hosts:      hosts,
			Paths:      paths,
			HasTLS:     hasTLS,
			TLSSecrets: tlsSecrets,
			Backend:    backend,
			Age:        fmt.Sprintf("%.0fd", time.Since(ing.CreationTimestamp.Time).Hours()/24),
		}
		result.Ingresses = append(result.Ingresses, entry)
		result.Summary.TotalIngresses++

		if hasTLS {
			result.Summary.WithTLS++
		} else {
			result.Summary.WithoutTLS++
			result.Issues = append(result.Issues, IngressIssue1924{
				Name: ing.Name, Namespace: ing.Namespace,
				IssueType: "no-tls", Severity: "warning",
				Detail: "Ingress has no TLS termination configured",
			})
		}

		// Check for orphan ingress (no backend service)
		if backend == "" {
			result.Summary.OrphanedIngress++
			result.Issues = append(result.Issues, IngressIssue1924{
				Name: ing.Name, Namespace: ing.Namespace,
				IssueType: "no-backend", Severity: "high",
				Detail: "Ingress has no backend service reference",
			})
			score -= 5
		}
	}

	// Detect host/path conflicts
	for hk, owners := range hostPathOwners {
		if len(owners) > 1 {
			// Check if from different namespaces or same namespace
			nsSet := make(map[string]bool)
			for _, o := range owners {
				nsSet[o.Namespace] = true
			}
			if len(nsSet) > 1 {
				result.Conflicts = append(result.Conflicts, IngressConflict1924{
					Type: "cross-namespace-host", Host: hk.host, Path: hk.path,
					Ingress1: owners[0].Name, Ingress2: owners[1].Name,
					Namespace1: owners[0].Namespace, Namespace2: owners[1].Namespace,
				})
				result.Summary.HostConflicts++
				score -= 3
			}
		}
	}

	// Score
	if result.Summary.WithoutTLS > result.Summary.WithTLS && result.Summary.TotalIngresses > 0 {
		score -= 10
	}
	if result.Summary.OrphanedIngress > 0 {
		score -= result.Summary.OrphanedIngress * 3
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutTLS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses without TLS — add TLS termination", result.Summary.WithoutTLS))
	}
	if result.Summary.HostConflicts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d host/path conflicts across namespaces — consolidate or use IngressClass", result.Summary.HostConflicts))
	}
	if result.Summary.OrphanedIngress > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d orphan ingresses without backend service", result.Summary.OrphanedIngress))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Job Lifecycle Tracker — completion/failure/staleness
// ---------------------------------------------------------------

type JobLifecycleResult1924 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         JobLifecycleSummary1924 `json:"summary"`
	Jobs            []JobEntry1924          `json:"jobs"`
	CronJobs        []CronJobEntry1924      `json:"cronJobs"`
	StaleJobs       []StaleJobEntry1924     `json:"staleJobs"`
	Recommendations []string                `json:"recommendations"`
}

type JobLifecycleSummary1924 struct {
	TotalJobs      int     `json:"totalJobs"`
	SucceededJobs  int     `json:"succeededJobs"`
	FailedJobs     int     `json:"failedJobs"`
	RunningJobs    int     `json:"runningJobs"`
	PendingJobs    int     `json:"pendingJobs"`
	TotalCronJobs  int     `json:"totalCronJobs"`
	SuspendedCron  int     `json:"suspendedCronJobs"`
	StaleJobCount  int     `json:"staleJobCount"`
	AvgJobDuration string  `json:"avgJobDuration"`
	SuccessRate    float64 `json:"successRate"`
}

type JobEntry1924 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Status      string `json:"status"`
	Completions int32  `json:"completions"`
	Succeeded   int32  `json:"succeeded"`
	Failed      int32  `json:"failed"`
	Age         string `json:"age"`
}

type CronJobEntry1924 struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Schedule     string `json:"schedule"`
	Suspended    bool   `json:"suspended"`
	LastSchedule string `json:"lastSchedule"`
	Active       int32  `json:"active"`
}

type StaleJobEntry1924 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
}

func (s *Server) handleJobLifecycle(w http.ResponseWriter, r *http.Request) {
	result := JobLifecycleResult1924{
		ScannedAt: time.Now(),
	}
	score := 100

	jobList, err := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, job := range jobList.Items {
		if isSystemNamespace(job.Namespace) {
			continue
		}
		status := "pending"
		succeeded := int32(0)
		failed := int32(0)
		if job.Status.Succeeded > 0 {
			status = "succeeded"
			succeeded = job.Status.Succeeded
		}
		if job.Status.Failed > 0 {
			status = "failed"
			failed = job.Status.Failed
		}
		if job.Status.Active > 0 {
			status = "running"
		}

		completions := int32(1)
		if job.Spec.Completions != nil {
			completions = *job.Spec.Completions
		}

		age := fmt.Sprintf("%.0fd", time.Since(job.CreationTimestamp.Time).Hours()/24)

		entry := JobEntry1924{
			Name:        job.Name,
			Namespace:   job.Namespace,
			Status:      status,
			Completions: completions,
			Succeeded:   succeeded,
			Failed:      failed,
			Age:         age,
		}
		result.Jobs = append(result.Jobs, entry)
		result.Summary.TotalJobs++

		switch status {
		case "succeeded":
			result.Summary.SucceededJobs++
		case "failed":
			result.Summary.FailedJobs++
			score -= 3
		case "running":
			result.Summary.RunningJobs++
		case "pending":
			result.Summary.PendingJobs++
		}

		// Stale: completed jobs older than 7 days without TTL cleanup
		if (status == "succeeded" || status == "failed") && time.Since(job.CreationTimestamp.Time).Hours() > 168 {
			if job.Spec.TTLSecondsAfterFinished == nil {
				result.StaleJobs = append(result.StaleJobs, StaleJobEntry1924{
					Name:      job.Name,
					Namespace: job.Namespace,
					Age:       age,
					Reason:    "Completed job >7 days old without TTL cleanup",
				})
				result.Summary.StaleJobCount++
			}
		}
	}

	// CronJobs
	cronList, err := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, cj := range cronList.Items {
			if isSystemNamespace(cj.Namespace) {
				continue
			}
			suspended := false
			if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
				suspended = true
				result.Summary.SuspendedCron++
			}
			lastSchedule := "never"
			if cj.Status.LastScheduleTime != nil {
				lastSchedule = fmt.Sprintf("%.0fd ago", time.Since(cj.Status.LastScheduleTime.Time).Hours()/24)
			}
			active := int32(len(cj.Status.Active))
			result.CronJobs = append(result.CronJobs, CronJobEntry1924{
				Name:         cj.Name,
				Namespace:    cj.Namespace,
				Schedule:     cj.Spec.Schedule,
				Suspended:    suspended,
				LastSchedule: lastSchedule,
				Active:       active,
			})
			result.Summary.TotalCronJobs++

			if suspended {
				result.StaleJobs = append(result.StaleJobs, StaleJobEntry1924{
					Name:      cj.Name,
					Namespace: cj.Namespace,
					Reason:    "CronJob is suspended — not running scheduled jobs",
				})
			}
		}
	}

	// Success rate
	if result.Summary.TotalJobs > 0 {
		result.Summary.SuccessRate = float64(result.Summary.SucceededJobs) * 100 / float64(result.Summary.TotalJobs)
	}

	// Score
	if result.Summary.FailedJobs > 5 {
		score -= 10
	}
	if result.Summary.StaleJobCount > 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.FailedJobs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d failed jobs — investigate failure logs", result.Summary.FailedJobs))
	}
	if result.Summary.StaleJobCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale jobs — set TTLSecondsAfterFinished for auto-cleanup", result.Summary.StaleJobCount))
	}
	if result.Summary.SuspendedCron > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d suspended CronJobs — resume or delete if no longer needed", result.Summary.SuspendedCron))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Leader Election Health — lease monitoring & failover readiness
// ---------------------------------------------------------------

type LeaderElectionResult1924 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         LeaderElectionSummary1924 `json:"summary"`
	Leases          []LeaseEntry1924          `json:"leases"`
	Controllers     []ControllerEntry1924     `json:"controllers"`
	Risks           []LeaderElectionRisk1924  `json:"risks"`
	Recommendations []string                  `json:"recommendations"`
}

type LeaderElectionSummary1924 struct {
	TotalLeases      int    `json:"totalLeases"`
	ActiveHolders    int    `json:"activeHolders"`
	AvgLeaseDuration string `json:"avgLeaseDuration"`
	WithoutLease     int    `json:"withoutLease"`
	StaleLeases      int    `json:"staleLeases"`
	ControllerCount  int    `json:"controllerCount"`
}

type LeaseEntry1924 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Holder      string `json:"holder"`
	Age         string `json:"age"`
	RenewTime   string `json:"renewTime"`
	DurationSec int32  `json:"durationSec"`
}

type ControllerEntry1924 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HasLease  bool   `json:"hasLease"`
	Replicas  int    `json:"replicas"`
}

type LeaderElectionRisk1924 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleLeaderElection(w http.ResponseWriter, r *http.Request) {
	result := LeaderElectionResult1924{
		ScannedAt: time.Now(),
	}
	score := 100

	// Check coordination leases (leader election)
	leaseList, err := s.clientset.CoordinationV1().Leases("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		// Fallback to checking controller deployments
		writeJSON(w, result)
		return
	}

	for _, lease := range leaseList.Items {
		holder := ""
		if lease.Spec.HolderIdentity != nil {
			holder = *lease.Spec.HolderIdentity
		}
		durationSec := int32(0)
		if lease.Spec.LeaseDurationSeconds != nil {
			durationSec = *lease.Spec.LeaseDurationSeconds
		}
		renewTime := "unknown"
		if lease.Spec.RenewTime != nil {
			age := time.Since(lease.Spec.RenewTime.Time)
			renewTime = fmt.Sprintf("%.0fs ago", age.Seconds())
			// Stale lease: not renewed in 10x duration
			if durationSec > 0 && age.Seconds() > float64(durationSec)*10 {
				result.Risks = append(result.Risks, LeaderElectionRisk1924{
					Name: lease.Name, Namespace: lease.Namespace,
					RiskType: "stale-lease", Severity: "high",
					Detail: fmt.Sprintf("Lease not renewed in %.0fs (10x duration)", age.Seconds()),
				})
				result.Summary.StaleLeases++
				score -= 5
			}
		}
		age := "unknown"
		if !lease.CreationTimestamp.IsZero() {
			age = fmt.Sprintf("%.0fd", time.Since(lease.CreationTimestamp.Time).Hours()/24)
		}

		result.Leases = append(result.Leases, LeaseEntry1924{
			Name:        lease.Name,
			Namespace:   lease.Namespace,
			Holder:      holder,
			Age:         age,
			RenewTime:   renewTime,
			DurationSec: durationSec,
		})
		result.Summary.TotalLeases++
		if holder != "" {
			result.Summary.ActiveHolders++
		}
	}

	// Check controller deployments for leader election
	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		leaseSet := make(map[string]bool)
		for _, l := range result.Leases {
			leaseSet[l.Namespace+"/"+l.Name] = true
		}
		for _, dep := range depList.Items {
			if isSystemNamespace(dep.Namespace) {
				continue
			}
			// Check if deployment looks like a controller (has leader election annotations)
			isController := false
			for key := range dep.Annotations {
				if strings.Contains(key, "leader-elect") || strings.Contains(key, "leader-elect-rewrite") {
					isController = true
					break
				}
			}
			// Also check by name patterns
			if !isController {
				nameLower := strings.ToLower(dep.Name)
				if strings.Contains(nameLower, "controller") || strings.Contains(nameLower, "operator") ||
					strings.Contains(nameLower, "scheduler") || strings.Contains(nameLower, "manager") {
					isController = true
				}
			}
			if !isController {
				continue
			}
			replicas := 1
			if dep.Spec.Replicas != nil {
				replicas = int(*dep.Spec.Replicas)
			}
			hasLease := leaseSet[dep.Namespace+"/"+dep.Name]
			result.Controllers = append(result.Controllers, ControllerEntry1924{
				Name: dep.Name, Namespace: dep.Namespace,
				HasLease: hasLease, Replicas: replicas,
			})
			result.Summary.ControllerCount++

			if !hasLease && replicas > 1 {
				result.Risks = append(result.Risks, LeaderElectionRisk1924{
					Name: dep.Name, Namespace: dep.Namespace,
					RiskType: "no-leader-election", Severity: "high",
					Detail: fmt.Sprintf("Controller with %d replicas but no leader election lease — split brain risk", replicas),
				})
				score -= 10
			}
			if !hasLease {
				result.Summary.WithoutLease++
			}
		}
	}

	// Score
	if result.Summary.StaleLeases > 0 {
		score -= result.Summary.StaleLeases * 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.StaleLeases > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale leader election leases — controller may be stuck", result.Summary.StaleLeases))
	}
	if result.Summary.WithoutLease > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d controllers without leader election — add for HA failover", result.Summary.WithoutLease))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// Suppress unused import warning
var _ networkingv1.Ingress = networkingv1.Ingress{}
var _ corev1.Pod = corev1.Pod{}
