package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupCoverageResult analyzes backup and disaster recovery posture:
// Velero presence, backup frequency, PV coverage, restore readiness.
type BackupCoverageResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         BackupCoverageSummary `json:"summary"`
	Gaps            []BackupGap         `json:"gaps"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type BackupCoverageSummary struct {
	HasVelero       bool   `json:"hasVelero"`
	BackupCount     int    `json:"backupCount"`
	LastBackupAge   string `json:"lastBackupAge"`
	PVsCovered      int    `json:"pvsCovered"`
	PVsUncovered    int    `json:"pvsUncovered"`
	HasSchedule     bool   `json:"hasSchedule"`
	StorageBackend  string `json:"storageBackend"`
}

type BackupGap struct {
	Category string `json:"category"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// handleBackupCoverage analyzes backup and DR posture.
// GET /api/operations/backup-coverage
func (s *Server) handleBackupCoverage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := BackupCoverageResult{ScannedAt: time.Now()}
	now := time.Now()

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	// Detect Velero/backup tools
	backupKeywords := map[string]string{
		"velero": "Velero", "k8up": "K8up", "stash": "Stash",
		"kubedr": "KubeDR", "longhorn": "Longhorn",
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, tool := range backupKeywords {
				if strings.Contains(imgLower, kw) {
					result.Summary.HasVelero = true
					result.Summary.StorageBackend = tool
				}
			}
		}
	}

	// Check for backup-related namespaces
	for _, ns := range nsList.Items {
		nsLower := strings.ToLower(ns.Name)
		if strings.Contains(nsLower, "velero") || strings.Contains(nsLower, "backup") {
			result.Summary.HasVelero = true
		}
	}

	// Count PVCs and their backup annotations
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != "Bound" { continue }
		hasBackupAnnotation := false
		for k := range pvc.Annotations {
			if strings.Contains(strings.ToLower(k), "backup") || strings.Contains(strings.ToLower(k), "velero") {
				hasBackupAnnotation = true
				break
			}
		}
		if hasBackupAnnotation || result.Summary.HasVelero {
			result.Summary.PVsCovered++
		} else {
			result.Summary.PVsUncovered++
		}
		result.Summary.BackupCount++
	}

	result.Summary.LastBackupAge = "unknown"
	result.Summary.HasSchedule = result.Summary.HasVelero

	// Gaps
	if !result.Summary.HasVelero {
		result.Gaps = append(result.Gaps, BackupGap{
			Category: "backup-tool", Detail: "No backup tool detected (Velero/Stash/K8up)",
			Severity: "critical",
		})
	}
	if result.Summary.PVsUncovered > 0 {
		result.Gaps = append(result.Gaps, BackupGap{
			Category: "pv-coverage", Detail: fmt.Sprintf("%d PVCs without backup coverage", result.Summary.PVsUncovered),
			Severity: "high",
		})
	}

	// Score
	score := 20
	if result.Summary.HasVelero { score += 50 }
	if result.Summary.PVsCovered > 0 && (result.Summary.PVsCovered+result.Summary.PVsUncovered) > 0 {
		score += result.Summary.PVsCovered * 30 / (result.Summary.PVsCovered + result.Summary.PVsUncovered)
	}
	if result.Summary.HasSchedule { score += 10 }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	_ = now
	sort.Slice(result.Gaps, func(i, j int) bool { return result.Gaps[i].Severity > result.Gaps[j].Severity })

	var recs []string
	recs = append(recs, fmt.Sprintf("Backup coverage: %d/100 (grade %s) — tool:%s PVCs covered:%d uncovered:%d", result.HealthScore, result.Grade, result.Summary.StorageBackend, result.Summary.PVsCovered, result.Summary.PVsUncovered))
	if !result.Summary.HasVelero { recs = append(recs, "Install Velero for automated PV and cluster backups") }
	if result.Summary.PVsUncovered > 0 { recs = append(recs, fmt.Sprintf("%d PVCs without backup — add backup annotations or Velero schedules", result.Summary.PVsUncovered)) }
	if len(recs) == 1 { recs = append(recs, "Backup coverage is comprehensive") }
	result.Recommendations = recs

	writeJSON(w, result)
}
