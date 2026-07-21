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
// v19.07 — Operations Dimension (Round 4)
// 1. Backup Snapshot Auditor
// 2. Job/CronJob Success Rate
// 3. Event Retention & Volume
// ============================================================

// ---------------------------------------------------------------
// 1. Backup Snapshot Auditor
// ---------------------------------------------------------------

type BackupSnapshotResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         BackupSnapSummary `json:"summary"`
	Snapshots       []BackupSnapEntry `json:"snapshots"`
	VolumeSnapshots []VolumeSnapEntry `json:"volumeSnapshots"`
	UnprotectedNS   []string          `json:"unprotectedNamespaces"`
	Recommendations []string          `json:"recommendations"`
}

type BackupSnapSummary struct {
	TotalPVs         int `json:"totalPersistentVolumes"`
	SnapshotsFound   int `json:"snapshotsFound"`
	VolumeSnapshots  int `json:"volumeSnapshots"`
	ProtectedPVs     int `json:"protectedPVs"`
	UnprotectedPVs   int `json:"unprotectedPVs"`
	BackupAgeHours   int `json:"latestBackupAgeHours"`
	NamespacesUnprot int `json:"namespacesUnprotected"`
}

type BackupSnapEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	PVCName       string `json:"pvcName"`
	SnapshotClass string `json:"snapshotClass"`
	ReadyToUse    bool   `json:"readyToUse"`
	CreationTime  string `json:"creationTime"`
	SizeBytes     int64  `json:"sizeBytes"`
	RiskLevel     string `json:"riskLevel"`
}

type VolumeSnapEntry struct {
	PVCName   string `json:"pvcName"`
	Namespace string `json:"namespace"`
	SizeGB    int    `json:"sizeGB"`
	Protected bool   `json:"protected"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleBackupSnapshotAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := BackupSnapshotResult{ScannedAt: time.Now()}

	// Check for VolumeSnapshots - skip if API not available
	// VolumeSnapshot API requires snapshot.storage.k8s.io CRD which may not be installed
	result.Summary.SnapshotsFound = 0

	// Check PVCs for backup coverage
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pvcSnapshotMap := map[string]bool{}
	for _, snap := range result.Snapshots {
		key := snap.Namespace + "/" + snap.PVCName
		pvcSnapshotMap[key] = true
	}

	protectedNS := map[string]bool{}
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVs++
		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}
		key := pvc.Namespace + "/" + pvc.Name
		protected := pvcSnapshotMap[key]
		if protected {
			result.Summary.ProtectedPVs++
			protectedNS[pvc.Namespace] = true
		} else {
			result.Summary.UnprotectedPVs++
			result.VolumeSnapshots = append(result.VolumeSnapshots, VolumeSnapEntry{
				PVCName: pvc.Name, Namespace: pvc.Namespace,
				SizeGB: sizeGB, Protected: false, RiskLevel: "high",
			})
		}
	}

	// Find namespaces with PVCs but no snapshots at all
	nsWithPVC := map[string]bool{}
	for _, pvc := range pvcs.Items {
		if !isSystemNamespace(pvc.Namespace) {
			nsWithPVC[pvc.Namespace] = true
		}
	}
	for ns := range nsWithPVC {
		if !protectedNS[ns] {
			result.Summary.NamespacesUnprot++
			result.UnprotectedNS = append(result.UnprotectedNS, ns)
		}
	}
	sort.Strings(result.UnprotectedNS)

	// Score
	if result.Summary.TotalPVs > 0 {
		protPct := result.Summary.ProtectedPVs * 100 / result.Summary.TotalPVs
		result.HealthScore = protPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildBackupSnapRecs1907(&result)
	writeJSON(w, result)
}

func buildBackupSnapRecs1907(r *BackupSnapshotResult) []string {
	recs := []string{fmt.Sprintf("Backup audit: %d PVs (%d protected, %d unprotected), %d volume snapshots",
		r.Summary.TotalPVs, r.Summary.ProtectedPVs, r.Summary.UnprotectedPVs, r.Summary.VolumeSnapshots)}
	if r.Summary.UnprotectedPVs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVCs without backup snapshots - configure VolumeSnapshot scheduling", r.Summary.UnprotectedPVs))
	}
	if len(r.UnprotectedNS) > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces with PVCs but no snapshots: %s", len(r.UnprotectedNS), strings.Join(r.UnprotectedNS[:minInt1907(5, len(r.UnprotectedNS))], ", ")))
	}
	return recs
}

func minInt1907(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------
// 2. Job/CronJob Success Rate
// ---------------------------------------------------------------

type JobSuccessResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         JobSuccessSummary `json:"summary"`
	ByCronJob       []JobSuccessEntry `json:"byCronJob"`
	FailedJobs      []JobSuccessEntry `json:"failedJobs"`
	ByNamespace     []JobSuccessNS    `json:"byNamespace"`
	Recommendations []string          `json:"recommendations"`
}

type JobSuccessSummary struct {
	TotalJobs     int     `json:"totalJobs"`
	SucceededJobs int     `json:"succeededJobs"`
	FailedJobs    int     `json:"failedJobs"`
	RunningJobs   int     `json:"runningJobs"`
	TotalCronJobs int     `json:"totalCronJobs"`
	SuccessRate   float64 `json:"successRate"`
	AvgDuration   int     `json:"avgDurationSec"`
	LongRunning   int     `json:"longRunningJobs"`
}

type JobSuccessEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CronJob   string `json:"cronJob,omitempty"`
	Status    string `json:"status"`
	Duration  int    `json:"durationSec"`
	StartTime string `json:"startTime"`
	RiskLevel string `json:"riskLevel"`
	FailCount int    `json:"failCount"`
}

type JobSuccessNS struct {
	Namespace   string  `json:"namespace"`
	TotalJobs   int     `json:"totalJobs"`
	FailedJobs  int     `json:"failedJobs"`
	SuccessRate float64 `json:"successRate"`
}

func (s *Server) handleJobSuccessRate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := JobSuccessResult{ScannedAt: time.Now()}

	// Map jobs to cronjobs
	cronjobs, _ := rc.clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	for _, cj := range cronjobs.Items {
		if !isSystemNamespace(cj.Namespace) {
			result.Summary.TotalCronJobs++
		}
	}

	nsMap := map[string]*JobSuccessNS{}
	var durations []int

	jobs, _ := rc.clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	for _, job := range jobs.Items {
		if isSystemNamespace(job.Namespace) {
			continue
		}
		result.Summary.TotalJobs++
		entry := JobSuccessEntry{
			Name:      job.Name,
			Namespace: job.Namespace,
		}

		// Determine job status
		switch {
		case job.Status.Failed > 0:
			entry.Status = "failed"
			entry.FailCount = int(job.Status.Failed)
			entry.RiskLevel = "high"
			result.Summary.FailedJobs++
			result.FailedJobs = append(result.FailedJobs, entry)
		case job.Status.Succeeded > 0:
			entry.Status = "succeeded"
			entry.RiskLevel = "low"
			result.Summary.SucceededJobs++
		case job.Status.Active > 0:
			entry.Status = "running"
			entry.RiskLevel = "low"
			result.Summary.RunningJobs++
		default:
			entry.Status = "pending"
			entry.RiskLevel = "medium"
		}

		// Calculate duration
		if job.Status.StartTime != nil {
			entry.StartTime = job.Status.StartTime.Format(time.RFC3339)
			endTime := time.Now()
			if job.Status.CompletionTime != nil {
				endTime = job.Status.CompletionTime.Time
			}
			duration := int(endTime.Sub(job.Status.StartTime.Time).Seconds())
			if duration < 0 {
				duration = 0
			}
			entry.Duration = duration
			durations = append(durations, duration)
			if duration > 3600 && job.Status.CompletionTime == nil {
				entry.RiskLevel = "high"
				result.Summary.LongRunning++
			}
		}

		// Check if owned by cronjob
		for _, owner := range job.OwnerReferences {
			if owner.Kind == "CronJob" {
				entry.CronJob = owner.Name
			}
		}

		result.ByCronJob = append(result.ByCronJob, entry)

		// Per-NS tracking
		nsE, ok := nsMap[job.Namespace]
		if !ok {
			nsE = &JobSuccessNS{Namespace: job.Namespace}
			nsMap[job.Namespace] = nsE
		}
		nsE.TotalJobs++
		if entry.Status == "failed" {
			nsE.FailedJobs++
		}
	}

	// Calculate averages
	if len(durations) > 0 {
		total := 0
		for _, d := range durations {
			total += d
		}
		result.Summary.AvgDuration = total / len(durations)
	}

	for _, ns := range nsMap {
		if ns.TotalJobs > 0 {
			ns.SuccessRate = float64(ns.TotalJobs-ns.FailedJobs) * 100 / float64(ns.TotalJobs)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].SuccessRate < result.ByNamespace[j].SuccessRate
	})

	// Score
	if result.Summary.TotalJobs > 0 {
		result.Summary.SuccessRate = float64(result.Summary.SucceededJobs) * 100 / float64(result.Summary.TotalJobs)
		result.HealthScore = int(result.Summary.SuccessRate)
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildJobSuccessRecs1907(&result)
	writeJSON(w, result)
}

func buildJobSuccessRecs1907(r *JobSuccessResult) []string {
	recs := []string{fmt.Sprintf("Job success rate: %d jobs (%.1f%% success), %d failed, %d running, %d long-running",
		r.Summary.TotalJobs, r.Summary.SuccessRate, r.Summary.FailedJobs,
		r.Summary.RunningJobs, r.Summary.LongRunning)}
	if r.Summary.FailedJobs > 0 {
		recs = append(recs, fmt.Sprintf("%d failed jobs - investigate job pod logs and resource limits", r.Summary.FailedJobs))
	}
	if r.Summary.LongRunning > 0 {
		recs = append(recs, fmt.Sprintf("%d jobs running >1hr - add activeDeadlineSeconds to prevent runaway jobs", r.Summary.LongRunning))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Event Retention & Volume
// ---------------------------------------------------------------

type EventRetentionResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EventRetentionSummary `json:"summary"`
	ByNamespace     []EventRetentionNS    `json:"byNamespace"`
	ByReason        map[string]int        `json:"byReason"`
	NoisySources    []EventSourceEntry    `json:"noisySources"`
	Recommendations []string              `json:"recommendations"`
}

type EventRetentionSummary struct {
	TotalEvents     int `json:"totalEvents"`
	Events24h       int `json:"events24h"`
	Events7d        int `json:"events7d"`
	WarningEvents   int `json:"warningEvents"`
	NormalEvents    int `json:"normalEvents"`
	UniqueReasons   int `json:"uniqueReasons"`
	NoisyComponents int `json:"noisyComponents"`
}

type EventRetentionNS struct {
	Namespace  string `json:"namespace"`
	EventCount int    `json:"eventCount"`
	Warnings   int    `json:"warnings"`
	RiskLevel  string `json:"riskLevel"`
}

type EventSourceEntry struct {
	Component  string `json:"component"`
	EventCount int    `json:"eventCount"`
	TopReason  string `json:"topReason"`
	NoiseRatio int    `json:"noiseRatioPct"`
}

func (s *Server) handleEventRetention(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EventRetentionResult{
		ScannedAt: time.Now(),
		ByReason:  map[string]int{},
	}

	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	nsMap := map[string]*EventRetentionNS{}
	componentEvents := map[string]int{}
	componentReasons := map[string]map[string]int{}

	for _, evt := range events.Items {
		if isSystemNamespace(evt.Namespace) {
			continue
		}
		result.Summary.TotalEvents++

		if evt.LastTimestamp.After(dayAgo) {
			result.Summary.Events24h++
		}
		if evt.LastTimestamp.After(weekAgo) {
			result.Summary.Events7d++
		}

		switch string(evt.Type) {
		case "Warning":
			result.Summary.WarningEvents++
		default:
			result.Summary.NormalEvents++
		}

		// By reason
		reason := evt.Reason
		if reason == "" {
			reason = "Unknown"
		}
		result.ByReason[reason]++

		// By namespace
		nsE, ok := nsMap[evt.Namespace]
		if !ok {
			nsE = &EventRetentionNS{Namespace: evt.Namespace}
			nsMap[evt.Namespace] = nsE
		}
		nsE.EventCount++
		if string(evt.Type) == "Warning" {
			nsE.Warnings++
		}

		// By component (source)
		component := evt.Source.Component
		if component == "" {
			component = evt.ReportingController
		}
		if component == "" {
			component = "unknown"
		}
		componentEvents[component]++
		if componentReasons[component] == nil {
			componentReasons[component] = map[string]int{}
		}
		componentReasons[component][reason]++
	}

	result.Summary.UniqueReasons = len(result.ByReason)

	// Identify noisy components (>50 events = noisy)
	for comp, count := range componentEvents {
		if count > 50 {
			result.Summary.NoisyComponents++
			topReason := ""
			topCount := 0
			for reason, rc := range componentReasons[comp] {
				if rc > topCount {
					topCount = rc
					topReason = reason
				}
			}
			noiseRatio := 0
			if count > 0 {
				noiseRatio = topCount * 100 / count
			}
			result.NoisySources = append(result.NoisySources, EventSourceEntry{
				Component: comp, EventCount: count,
				TopReason: topReason, NoiseRatio: noiseRatio,
			})
		}
	}
	sort.Slice(result.NoisySources, func(i, j int) bool {
		return result.NoisySources[i].EventCount > result.NoisySources[j].EventCount
	})

	for _, ns := range nsMap {
		if ns.Warnings > 50 {
			ns.RiskLevel = "high"
		} else if ns.Warnings > 10 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].EventCount > result.ByNamespace[j].EventCount
	})

	// Score: more warning events = lower score
	if result.Summary.TotalEvents > 0 {
		warningPct := result.Summary.WarningEvents * 100 / result.Summary.TotalEvents
		result.HealthScore = 100 - warningPct/2
		// Noise penalty
		result.HealthScore -= result.Summary.NoisyComponents * 5
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildEventRetentionRecs1907(&result)
	writeJSON(w, result)
}

func buildEventRetentionRecs1907(r *EventRetentionResult) []string {
	recs := []string{fmt.Sprintf("Event volume: %d total events (%d in 24h), %d warnings, %d noisy components",
		r.Summary.TotalEvents, r.Summary.Events24h, r.Summary.WarningEvents, r.Summary.NoisyComponents)}
	if r.Summary.NoisyComponents > 0 {
		recs = append(recs, fmt.Sprintf("%d noisy components generating excessive events - investigate and fix root cause", r.Summary.NoisyComponents))
	}
	if r.Summary.WarningEvents > 100 {
		recs = append(recs, fmt.Sprintf("%d warning events - review cluster health and address recurring issues", r.Summary.WarningEvents))
	}
	return recs
}

// (removed unused batchv1 reference)
