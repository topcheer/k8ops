package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageTierResult analyzes storage classes, PVC performance tiers, and
// volume binding modes to identify cost optimization opportunities and
// configuration inconsistencies.
type StorageTierResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         StorageTierSummary     `json:"summary"`
	ByStorageClass  []StorageTierClassStat `json:"byStorageClass"`
	Mismatches      []StorageTierMismatch  `json:"mismatches"`
	Optimization    []StorageOptimize      `json:"optimization"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type StorageTierSummary struct {
	TotalPVCs        int     `json:"totalPVCs"`
	TotalSizeGB      float64 `json:"totalSizeGB"`
	StorageClasses   int     `json:"storageClasses"`
	BoundPVCs        int     `json:"boundPVCs"`
	PendingPVCs      int     `json:"pendingPVCs"`
	OrphanedPVCs     int     `json:"orphanedPVCs"`
	DefaultSC        string  `json:"defaultStorageClass"`
	WritableOncePVCs int     `json:"writableOncePVCs"`
	ReadOnlyManyPVCs int     `json:"readOnlyManyPVCs"`
}

type StorageTierClassStat struct {
	Name        string  `json:"name"`
	PVCCount    int     `json:"pvcCount"`
	TotalSizeGB float64 `json:"totalSizeGB"`
	IsDefault   bool    `json:"isDefault"`
	Provisioner string  `json:"provisioner"`
	BindingMode string  `json:"bindingMode"`
}

type StorageTierMismatch struct {
	PVC       string `json:"pvc"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type StorageOptimize struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Save     string `json:"estimatedSave"`
	Action   string `json:"action"`
}

// handleStorageTier handles GET /api/scalability/storage-tier
func (s *Server) handleStorageTier(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := StorageTierResult{ScannedAt: time.Now()}

	storageClasses, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Find default storage class
	defaultSC := ""
	scMap := make(map[string]*StorageTierClassStat)
	for _, sc := range storageClasses.Items {
		stat := &StorageTierClassStat{
			Name:        sc.Name,
			Provisioner: sc.Provisioner,
			BindingMode: "Immediate",
		}
		if sc.VolumeBindingMode != nil {
			stat.BindingMode = string(*sc.VolumeBindingMode)
		}
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			stat.IsDefault = true
			defaultSC = sc.Name
		}
		scMap[sc.Name] = stat
	}
	result.Summary.StorageClasses = len(storageClasses.Items)
	result.Summary.DefaultSC = defaultSC

	// Analyze PVCs
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		sizeGB := 0.0
		if pvc.Spec.Resources.Requests.Storage() != nil {
			sizeGB = pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
		}
		result.Summary.TotalSizeGB += sizeGB

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
		case corev1.ClaimLost:
			result.Summary.OrphanedPVCs++
		}

		switch pvc.Spec.AccessModes[0] {
		case corev1.ReadWriteOnce:
			result.Summary.WritableOncePVCs++
		case corev1.ReadOnlyMany, corev1.ReadWriteMany:
			result.Summary.ReadOnlyManyPVCs++
		}

		// Storage class stats
		scName := pvc.Spec.StorageClassName
		if scName == nil || *scName == "" {
			scName = &defaultSC
		}
		if scName != nil && *scName != "" {
			if stat, ok := scMap[*scName]; ok {
				stat.PVCCount++
				stat.TotalSizeGB += sizeGB
			}
		}

		// Check for mismatches
		if pvc.Status.Phase == corev1.ClaimPending {
			result.Mismatches = append(result.Mismatches, StorageTierMismatch{
				PVC: pvc.Name, Namespace: pvc.Namespace,
				Issue: "PVC stuck in Pending", Severity: "high",
				Detail: "StorageClass may not exist or provisioner failed",
			})
		}
		if pvc.Status.Phase == corev1.ClaimLost {
			result.Mismatches = append(result.Mismatches, StorageTierMismatch{
				PVC: pvc.Name, Namespace: pvc.Namespace,
				Issue: "PVC lost its PV", Severity: "critical",
				Detail: "Underlying PersistentVolume is lost or deleted",
			})
		}
		// Oversized PVC check (> 100GB)
		if sizeGB > 100 {
			result.Mismatches = append(result.Mismatches, StorageTierMismatch{
				PVC: pvc.Name, Namespace: pvc.Namespace,
				Issue: fmt.Sprintf("Large PVC %.0fGB", sizeGB), Severity: "low",
				Detail: "Verify actual usage, may be over-provisioned",
			})
		}
	}

	// Build storage class stats
	for _, stat := range scMap {
		result.ByStorageClass = append(result.ByStorageClass, *stat)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].PVCCount > result.ByStorageClass[j].PVCCount
	})

	// Optimization suggestions
	if result.Summary.PendingPVCs > 0 {
		result.Optimization = append(result.Optimization, StorageOptimize{
			Category: "Pending PVCs", Count: result.Summary.PendingPVCs,
			Action: "Check StorageClass provisioner and capacity",
		})
	}
	if result.Summary.OrphanedPVCs > 0 {
		result.Optimization = append(result.Optimization, StorageOptimize{
			Category: "Lost PVCs", Count: result.Summary.OrphanedPVCs,
			Save:   fmt.Sprintf("%.1f GB", result.Summary.TotalSizeGB/float64(maxInt(result.Summary.TotalPVCs, 1))*float64(result.Summary.OrphanedPVCs)),
			Action: "Delete lost PVCs to free storage",
		})
	}

	// Score
	if result.Summary.TotalPVCs > 0 {
		result.HealthScore = result.Summary.BoundPVCs * 100 / result.Summary.TotalPVCs
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 90:
		result.Grade = "A"
	case result.HealthScore >= 75:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildStorageTierRecs(&result)
	writeJSON(w, result)
}

func buildStorageTierRecs(r *StorageTierResult) []string {
	recs := []string{}
	if r.Summary.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 PVC 卡在 Pending，检查 StorageClass 和 provisioner", r.Summary.PendingPVCs))
	}
	if r.Summary.OrphanedPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 PVC 状态为 Lost，建议清理", r.Summary.OrphanedPVCs))
	}
	if r.Summary.TotalSizeGB > 500 {
		recs = append(recs, fmt.Sprintf("总存储 %.0f GB，建议审查是否有过度分配", r.Summary.TotalSizeGB))
	}
	if r.Summary.StorageClasses == 0 {
		recs = append(recs, "没有定义 StorageClass，所有 PVC 使用默认或无法绑定")
	}
	if len(recs) == 0 {
		recs = append(recs, "存储配置健康")
	}
	return recs
}
