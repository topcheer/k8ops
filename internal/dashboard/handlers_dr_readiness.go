package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DRResult is the disaster recovery readiness analysis.
type DRResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         DRSummary `json:"summary"`
	Findings        []DREntry `json:"findings"`
	ProtectedNS     []string  `json:"protectedNamespaces"`
	UnprotectedNS   []string  `json:"unprotectedNamespaces"`
	Issues          []DRIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// DRSummary aggregates DR readiness stats.
type DRSummary struct {
	TotalNamespaces    int  `json:"totalNamespaces"`
	ProtectedNS        int  `json:"protectedNamespaces"`
	HasBackupLabels    int  `json:"hasBackupLabels"`
	HasPVCs            int  `json:"hasPVCs"`
	PVCsNotSnapshotted int  `json:"pvsNotSnapshotted"`
	HasVelero          bool `json:"hasVelero"`
	HasSnapshotCtrl    bool `json:"hasSnapshotController"`
	MultiAZ            bool `json:"multiAZ"`        // nodes in multiple zones
	ReadinessScore     int  `json:"readinessScore"` // 0-100
}

// DREntry describes one DR finding.
type DREntry struct {
	Category string `json:"category"` // backup, snapshot, topology, recovery
	Status   string `json:"status"`   // pass, warning, fail
	Message  string `json:"message"`
}

// DRIssue is a detected DR problem.
type DRIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleDRReadiness audits cluster disaster recovery readiness.
// GET /api/scalability/dr-readiness
func (s *Server) handleDRReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DRResult{ScannedAt: time.Now()}

	// Check for Velero (backup controller)
	veleroDeployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
		LabelSelector: "kubernetes.io/created-by=velero",
	})
	if err == nil && veleroDeployments != nil && len(veleroDeployments.Items) > 0 {
		result.Summary.HasVelero = true
		result.Findings = append(result.Findings, DREntry{
			Category: "backup", Status: "pass",
			Message: "Velero backup controller detected in cluster",
		})
	} else {
		// Also check by name
		veleroNS, _ := rc.clientset.AppsV1().Deployments("velero").List(ctx, metav1.ListOptions{})
		if veleroNS != nil && len(veleroNS.Items) > 0 {
			result.Summary.HasVelero = true
			result.Findings = append(result.Findings, DREntry{
				Category: "backup", Status: "pass",
				Message: "Velero backup controller detected in velero namespace",
			})
		} else {
			result.Findings = append(result.Findings, DREntry{
				Category: "backup", Status: "warning",
				Message: "No Velero or backup controller detected — consider installing Velero for automated backups",
			})
			result.Issues = append(result.Issues, DRIssue{
				Severity: "warning", Type: "no-backup-controller",
				Resource: "cluster",
				Message:  "No backup controller (Velero/K8up/Stash) detected — no automated backup solution for disaster recovery",
			})
		}
	}

	// Check for VolumeSnapshotClass (snapshot controller)
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if scs != nil {
		// Can't list VSC directly (CRD), check CSI drivers for snapshot capability
		drivers, _ := rc.clientset.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
		if drivers != nil && len(drivers.Items) > 0 {
			result.Summary.HasSnapshotCtrl = true
			result.Findings = append(result.Findings, DREntry{
				Category: "snapshot", Status: "pass",
				Message: fmt.Sprintf("%d CSI driver(s) detected — volume snapshots may be available", len(drivers.Items)),
			})
		}
	}

	// Check namespace backup coverage
	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil && nss != nil {
		for _, ns := range nss.Items {
			// Skip system namespaces
			if strings.HasPrefix(ns.Name, "kube-") || ns.Name == "k8ops-system" {
				continue
			}
			result.Summary.TotalNamespaces++

			// Check for backup labels
			hasBackupLabel := false
			for k := range ns.Labels {
				if strings.Contains(k, "backup") || strings.Contains(k, "velero") {
					hasBackupLabel = true
					result.Summary.HasBackupLabels++
					break
				}
			}

			if hasBackupLabel {
				result.Summary.ProtectedNS++
				result.ProtectedNS = append(result.ProtectedNS, ns.Name)
			} else {
				result.UnprotectedNS = append(result.UnprotectedNS, ns.Name)
			}
		}
	}

	if result.Summary.TotalNamespaces > 0 && result.Summary.ProtectedNS < result.Summary.TotalNamespaces {
		unprotected := result.Summary.TotalNamespaces - result.Summary.ProtectedNS
		result.Findings = append(result.Findings, DREntry{
			Category: "backup", Status: "warning",
			Message: fmt.Sprintf("%d/%d application namespaces have no backup labels — data loss risk", unprotected, result.Summary.TotalNamespaces),
		})
		result.Issues = append(result.Issues, DRIssue{
			Severity: "warning", Type: "unprotected-namespaces",
			Resource: "cluster",
			Message:  fmt.Sprintf("%d namespaces have no backup configuration — workloads may not be recoverable after disaster", unprotected),
		})
	}

	// Check node topology (multi-AZ)
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil && nodes != nil {
		zones := make(map[string]int)
		for _, node := range nodes.Items {
			if zone, ok := node.Labels["topology.kubernetes.io/zone"]; ok {
				zones[zone]++
			}
		}
		if len(zones) >= 2 {
			result.Summary.MultiAZ = true
			result.Findings = append(result.Findings, DREntry{
				Category: "topology", Status: "pass",
				Message: fmt.Sprintf("Cluster spans %d availability zones — good fault tolerance", len(zones)),
			})
		} else if len(zones) == 1 {
			result.Findings = append(result.Findings, DREntry{
				Category: "topology", Status: "warning",
				Message: "Cluster is in a single availability zone — zone failure will cause total outage",
			})
			result.Issues = append(result.Issues, DRIssue{
				Severity: "info", Type: "single-zone",
				Resource: "cluster",
				Message:  "Single availability zone deployment — consider multi-zone for fault tolerance",
			})
		}
	}

	// Check PVCs for persistent data
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err == nil && pvcs != nil {
		result.Summary.HasPVCs = len(pvcs.Items)
		if len(pvcs.Items) > 0 {
			result.Findings = append(result.Findings, DREntry{
				Category: "snapshot", Status: "info",
				Message: fmt.Sprintf("%d PVC(s) with persistent data — ensure VolumeSnapshots are configured for backup", len(pvcs.Items)),
			})
		}
	}

	// Sort
	sort.Slice(result.Findings, func(i, j int) bool {
		return drStatusRank(result.Findings[i].Status) < drStatusRank(result.Findings[j].Status)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return drIssueRank(result.Issues[i].Severity) < drIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ReadinessScore = drScore(result.Summary)
	result.Recommendations = drGenRecs(result.Summary)

	writeJSON(w, result)
}

// drScore computes DR readiness score 0-100.
func drScore(s DRSummary) int {
	score := 0
	if s.HasVelero {
		score += 35
	}
	if s.HasSnapshotCtrl {
		score += 15
	}
	if s.MultiAZ {
		score += 15
	}
	if s.TotalNamespaces > 0 {
		protectedRatio := float64(s.ProtectedNS) / float64(s.TotalNamespaces)
		score += int(protectedRatio * 35)
	}
	return score
}

// drGenRecs produces actionable advice.
func drGenRecs(s DRSummary) []string {
	var recs []string

	if !s.HasVelero {
		recs = append(recs, "Install Velero or another backup controller for automated cluster backups and disaster recovery")
	}
	if s.TotalNamespaces > 0 && s.ProtectedNS < s.TotalNamespaces {
		unprotected := s.TotalNamespaces - s.ProtectedNS
		recs = append(recs, fmt.Sprintf("%d namespace(s) have no backup labels — add backup annotations (velero.io/backup) to enable automated backups", unprotected))
	}
	if s.HasPVCs > 0 && !s.HasSnapshotCtrl {
		recs = append(recs, fmt.Sprintf("%d PVC(s) but no snapshot controller — install CSI snapshot controller for volume backups", s.HasPVCs))
	}
	if !s.MultiAZ {
		recs = append(recs, "Single AZ deployment — distribute nodes across multiple zones for zone-failure resilience")
	}
	if s.HasPVCs > 0 {
		recs = append(recs, fmt.Sprintf("Verify %d PVC(s) have regular VolumeSnapshot schedules — test restore procedure quarterly", s.HasPVCs))
	}
	if s.ReadinessScore < 50 {
		recs = append(recs, fmt.Sprintf("DR readiness score is %d/100 — critical gaps in disaster recovery posture", s.ReadinessScore))
	}
	if s.HasVelero && s.ProtectedNS == s.TotalNamespaces && s.MultiAZ {
		recs = append(recs, fmt.Sprintf("Good DR posture (score: %d/100) — Velero + multi-AZ + backup coverage", s.ReadinessScore))
	}

	return recs
}

func drStatusRank(s string) int {
	switch s {
	case "fail":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	case "pass":
		return 3
	default:
		return 4
	}
}

func drIssueRank(s string) int {
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
