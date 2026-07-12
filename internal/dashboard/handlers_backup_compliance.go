package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupResult is the volume snapshot & PVC backup compliance audit.
type BackupResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         BackupSummary    `json:"summary"`
	UnprotectedPVCs []PVCBackupEntry `json:"unprotectedPVCs"`
	ByNamespace     []BackupNSStat   `json:"byNamespace"`
	ByStorageClass  []BackupSCStat   `json:"byStorageClass"`
	Recommendations []string         `json:"recommendations"`
}

// BackupSummary aggregates backup compliance statistics.
type BackupSummary struct {
	TotalPVCs       int  `json:"totalPVCs"`
	ProtectedPVCs   int  `json:"protectedPVCs"` // PVCs with snapshot annotation
	UnprotectedPVCs int  `json:"unprotectedPVCs"`
	CriticalPVCs    int  `json:"criticalPVCs"` // large PVCs without backup
	HasVelero       bool `json:"hasVelero"`
	VeleroBackups   int  `json:"veleroBackups"`
	HealthScore     int  `json:"healthScore"`
}

// PVCBackupEntry describes an unprotected PVC.
type PVCBackupEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Size         string `json:"size"`
	StorageClass string `json:"storageClass"`
	AccessMode   string `json:"accessMode"`
	Age          string `json:"age"`
	Severity     string `json:"severity"`
}

// BackupNSStat shows backup compliance per namespace.
type BackupNSStat struct {
	Namespace   string `json:"namespace"`
	TotalPVCs   int    `json:"totalPVCs"`
	Unprotected int    `json:"unprotected"`
	Protected   int    `json:"protected"`
	IsSystem    bool   `json:"isSystem"`
}

// BackupSCStat shows backup compliance per storage class.
type BackupSCStat struct {
	StorageClass string `json:"storageClass"`
	TotalPVCs    int    `json:"totalPVCs"`
	Unprotected  int    `json:"unprotected"`
}

// handleBackupCompliance audits PVC backup and snapshot compliance.
// GET /api/product/backup-compliance
func (s *Server) handleBackupCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Check for Velero
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	hasVelero := false
	for _, ns := range namespaces.Items {
		if ns.Name == "velero" {
			hasVelero = true
			break
		}
	}

	// Build PVC usage map: pvc name -> mounted by pods
	pvcUsage := map[string]bool{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				pvcUsage[key] = true
			}
		}
	}

	now := time.Now()
	result := BackupResult{
		ScannedAt: now,
		Summary:   BackupSummary{HasVelero: hasVelero},
	}
	result.Summary.TotalPVCs = len(pvcs.Items)

	nsStats := map[string]*BackupNSStat{}
	scStats := map[string]*BackupSCStat{}

	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound && pvc.Status.Phase != corev1.ClaimPending {
			continue
		}

		nsStat, ok := nsStats[pvc.Namespace]
		if !ok {
			nsStat = &BackupNSStat{Namespace: pvc.Namespace, IsSystem: isSystemNamespace(pvc.Namespace)}
			nsStats[pvc.Namespace] = nsStat
		}
		nsStat.TotalPVCs++

		scName := *pvc.Spec.StorageClassName
		if scName == "" {
			scName = "<default>"
		}
		scStat, ok := scStats[scName]
		if !ok {
			scStat = &BackupSCStat{StorageClass: scName}
			scStats[scName] = scStat
		}
		scStat.TotalPVCs++

		// Check if PVC has backup annotation or snapshot
		hasBackup := false
		if pvc.Annotations != nil {
			for key := range pvc.Annotations {
				if contains(key, "backup") || contains(key, "snapshot") || contains(key, "velero.io/backup") {
					hasBackup = true
					break
				}
			}
		}

		// Check if PVC is actually in use
		pvcKey := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		inUse := pvcUsage[pvcKey]

		if hasBackup {
			result.Summary.ProtectedPVCs++
			nsStat.Protected++
		} else if inUse {
			// Only flag PVCs that are actually in use
			result.Summary.UnprotectedPVCs++
			nsStat.Unprotected++
			scStat.Unprotected++

			// Determine size
			size := "unknown"
			if pvc.Spec.Resources.Requests != nil {
				if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
					size = qty.String()
				}
			}

			// Severity based on size
			severity := "medium"
			if size != "unknown" {
				// Parse size in Gi roughly
				if contains(size, "Gi") || contains(size, "Ti") {
					severity = "high"
					result.Summary.CriticalPVCs++
				}
			}

			accessMode := ""
			if len(pvc.Spec.AccessModes) > 0 {
				accessMode = string(pvc.Spec.AccessModes[0])
			}

			result.UnprotectedPVCs = append(result.UnprotectedPVCs, PVCBackupEntry{
				Name:         pvc.Name,
				Namespace:    pvc.Namespace,
				Size:         size,
				StorageClass: scName,
				AccessMode:   accessMode,
				Age:          formatDuration(now.Sub(pvc.CreationTimestamp.Time)),
				Severity:     severity,
			})
		}
	}

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Unprotected > result.ByNamespace[j].Unprotected
	})

	// Build storage class stats
	for _, sc := range scStats {
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].Unprotected > result.ByStorageClass[j].Unprotected
	})

	// Sort unprotected by severity
	sort.Slice(result.UnprotectedPVCs, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.UnprotectedPVCs[i].Severity] < sevOrder[result.UnprotectedPVCs[j].Severity]
	})
	if len(result.UnprotectedPVCs) > 30 {
		result.UnprotectedPVCs = result.UnprotectedPVCs[:30]
	}

	result.Summary.HealthScore = backupScore(result.Summary)
	result.Recommendations = backupRecommendations(&result)

	writeJSON(w, result)
}

// backupScore computes a 0-100 backup compliance score.
func backupScore(s BackupSummary) int {
	if s.TotalPVCs == 0 {
		return 100
	}

	score := 100

	// Penalize unprotected PVCs
	unprotRatio := float64(s.UnprotectedPVCs) / float64(s.TotalPVCs)
	score -= int(unprotRatio * 50)

	// Extra penalty for critical unprotected PVCs
	if s.CriticalPVCs > 0 {
		score -= min(20, s.CriticalPVCs*5)
	}

	// Small penalty for no Velero
	if !s.HasVelero && s.TotalPVCs > 5 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	return score
}

// backupRecommendations generates actionable recommendations.
func backupRecommendations(r *BackupResult) []string {
	var recs []string

	if r.Summary.UnprotectedPVCs > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d PVC(s) are in use without backup protection — create VolumeSnapshots or use Velero for disaster recovery",
			r.Summary.UnprotectedPVCs,
		))
	}

	if r.Summary.CriticalPVCs > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d large PVC(s) (>=1Gi) are unprotected — prioritize backup for critical data volumes",
			r.Summary.CriticalPVCs,
		))
	}

	if !r.Summary.HasVelero && r.Summary.TotalPVCs > 5 {
		recs = append(recs, "Velero is not installed — consider installing it for automated PVC backups and cluster disaster recovery")
	}

	if r.Summary.TotalPVCs > 0 {
		protRatio := float64(r.Summary.ProtectedPVCs) / float64(r.Summary.TotalPVCs) * 100
		if protRatio < 50 {
			recs = append(recs, fmt.Sprintf(
				"Only %.0f%% of PVCs have backup protection — establish a backup policy for all critical volumes",
				protRatio,
			))
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "All PVCs are protected with backups — backup compliance is healthy")
	}

	return recs
}
