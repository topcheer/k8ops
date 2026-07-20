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

// PVCIOHealthResult monitors PVC read/write health and I/O anomalies.
type PVCIOHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         PVCIOHealthSummary `json:"summary"`
	ByPVC           []PVCIOEntry       `json:"byPVC"`
	AtRiskPVCs      []PVCIOEntry       `json:"atRiskPVCs"`
	ByStorageClass  []PVCSCEntry       `json:"byStorageClass"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type PVCIOHealthSummary struct {
	TotalPVCs    int     `json:"totalPVCs"`
	BoundPVCs    int     `json:"boundPVCs"`
	PendingPVCs  int     `json:"pendingPVCs"`
	LostPVCs     int     `json:"lostPVCs"`
	TotalSizeGB  float64 `json:"totalSizeGB"`
	OrphanedPVCs int     `json:"orphanedPVCs"`
	LargePVCs    int     `json:"largePVCs"` // > 100GB
	NoBackupPVCs int     `json:"noBackupPVCs"`
}

type PVCIOEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	SizeGB       float64  `json:"sizeGB"`
	StorageClass string   `json:"storageClass"`
	Status       string   `json:"status"`
	AccessMode   string   `json:"accessMode"`
	MountedByPod string   `json:"mountedByPod"`
	IsOrphaned   bool     `json:"isOrphaned"`
	HasBackup    bool     `json:"hasBackupAnnotation"`
	RiskLevel    string   `json:"riskLevel"`
	RiskFactors  []string `json:"riskFactors"`
}

type PVCSCEntry struct {
	StorageClass string  `json:"storageClass"`
	Count        int     `json:"count"`
	TotalSizeGB  float64 `json:"totalSizeGB"`
}

// handlePVCIOHealth handles GET /api/product/pvc-io-health
func (s *Server) handlePVCIOHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PVCIOHealthResult{ScannedAt: time.Now()}

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build PVC to pod mount map
	pvcMountMap := make(map[string]string) // ns/pvc -> podName
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := pod.Namespace + "/" + vol.PersistentVolumeClaim.ClaimName
				pvcMountMap[key] = pod.Name
			}
		}
	}

	scMap := make(map[string]*PVCSCEntry)

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		entry := PVCIOEntry{
			Name:         pvc.Name,
			Namespace:    pvc.Namespace,
			StorageClass: "<none>",
			Status:       string(pvc.Status.Phase),
		}

		if pvc.Spec.StorageClassName != nil {
			entry.StorageClass = *pvc.Spec.StorageClassName
		}

		// Size
		if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			entry.SizeGB = float64(req.Value()) / (1024 * 1024 * 1024)
			result.Summary.TotalSizeGB += entry.SizeGB
		}

		// Access mode
		if len(pvc.Spec.AccessModes) > 0 {
			entry.AccessMode = string(pvc.Spec.AccessModes[0])
		}

		// Mount info
		key := pvc.Namespace + "/" + pvc.Name
		entry.MountedByPod = pvcMountMap[key]

		// Status
		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
		case corev1.ClaimLost:
			result.Summary.LostPVCs++
		}

		// Risk factors
		var risks []string
		entry.IsOrphaned = entry.MountedByPod == ""
		if entry.IsOrphaned {
			risks = append(risks, "orphaned-no-mount")
			result.Summary.OrphanedPVCs++
		}
		if entry.SizeGB > 100 {
			risks = append(risks, "large-volume")
			result.Summary.LargePVCs++
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			risks = append(risks, "not-bound")
		}

		// Check backup annotation
		for k := range pvc.Annotations {
			if strings.Contains(k, "backup") || strings.Contains(k, "velero") || strings.Contains(k, "snapshot") {
				entry.HasBackup = true
				break
			}
		}
		if !entry.HasBackup {
			risks = append(risks, "no-backup")
			result.Summary.NoBackupPVCs++
		}

		entry.RiskFactors = risks
		switch {
		case len(risks) >= 3:
			entry.RiskLevel = "critical"
		case len(risks) >= 2:
			entry.RiskLevel = "high"
		case len(risks) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.AtRiskPVCs = append(result.AtRiskPVCs, entry)
		}
		result.ByPVC = append(result.ByPVC, entry)

		// Storage class aggregation
		if scMap[entry.StorageClass] == nil {
			scMap[entry.StorageClass] = &PVCSCEntry{StorageClass: entry.StorageClass}
		}
		scMap[entry.StorageClass].Count++
		scMap[entry.StorageClass].TotalSizeGB += entry.SizeGB
	}

	for _, sc := range scMap {
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}
	sort.Slice(result.ByPVC, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByPVC[i].RiskLevel] < rank[result.ByPVC[j].RiskLevel]
	})

	if result.Summary.TotalPVCs > 0 {
		healthy := result.Summary.TotalPVCs - result.Summary.OrphanedPVCs - result.Summary.LostPVCs - result.Summary.PendingPVCs
		result.HealthScore = healthy * 100 / result.Summary.TotalPVCs
		result.HealthScore -= result.Summary.NoBackupPVCs * 3
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("PVC 健康: %d 总计, %d 已绑定, %d 孤立, %.1fGB 总量, %d 无备份",
			result.Summary.TotalPVCs, result.Summary.BoundPVCs,
			result.Summary.OrphanedPVCs, result.Summary.TotalSizeGB, result.Summary.NoBackupPVCs),
	}
	if result.Summary.OrphanedPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个孤立 PVC 未被任何 Pod 挂载, 建议清理", result.Summary.OrphanedPVCs))
	}
	if result.Summary.NoBackupPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 PVC 没有备份标注, 数据丢失风险", result.Summary.NoBackupPVCs))
	}
	writeJSON(w, result)
}
