package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OrphanCleanupResult identifies orphaned resources across the cluster
// and generates safe cleanup commands. It cross-references ConfigMaps,
// Secrets, PVCs, and ConfigMap/Secret volume mounts to find resources
// that no workload references — wasted storage and potential security risk.
type OrphanCleanupResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         OrphanCleanupSummary `json:"summary"`
	Orphans         []OrphanItem         `json:"orphans"`
	CleanupBatches  []OrphanBatch        `json:"cleanupBatches"`
	PotentialSave   OrphanSave           `json:"potentialSave"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type OrphanCleanupSummary struct {
	TotalResources int `json:"totalResources"`
	OrphanedCount  int `json:"orphanedCount"`
	ConfigMaps     int `json:"configMaps"`
	Secrets        int `json:"secrets"`
	PVCs           int `json:"pvcs"`
	SafeToDelete   int `json:"safeToDelete"`
	NeedsReview    int `json:"needsReview"`
}

type OrphanItem struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Age          string `json:"age"`
	Size         string `json:"size"`
	SafeToDelete bool   `json:"safeToDelete"`
	Reason       string `json:"reason"`
	DeleteCmd    string `json:"deleteCmd"`
}

type OrphanBatch struct {
	Title    string   `json:"title"`
	Kind     string   `json:"kind"`
	Commands []string `json:"commands"`
	Count    int      `json:"count"`
}

type OrphanSave struct {
	EstimatedStorageGB   float64 `json:"estimatedStorageGB"`
	EstimatedMonthlyCost float64 `json:"estimatedMonthlyCostUSD"`
	SecretCount          int     `json:"secretCount"`
}

// handleOrphanCleanup handles GET /api/scalability/orphan-cleanup
func (s *Server) handleOrphanCleanup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := OrphanCleanupResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Build usage set: namespace/name -> used
	usedCM := make(map[string]bool)
	usedSec := make(map[string]bool)
	usedPVC := make(map[string]bool)

	scanVolumes := func(vols []corev1.Volume, ns string) {
		for _, v := range vols {
			if v.ConfigMap != nil {
				usedCM[ns+"/"+v.ConfigMap.Name] = true
			}
			if v.Secret != nil {
				usedSec[ns+"/"+v.Secret.SecretName] = true
			}
			if v.PersistentVolumeClaim != nil {
				usedPVC[ns+"/"+v.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	scanEnv := func(containers []corev1.Container, ns string) {
		for _, c := range containers {
			for _, e := range c.Env {
				if e.ValueFrom != nil {
					if e.ValueFrom.ConfigMapKeyRef != nil {
						usedCM[ns+"/"+e.ValueFrom.ConfigMapKeyRef.Name] = true
					}
					if e.ValueFrom.SecretKeyRef != nil {
						usedSec[ns+"/"+e.ValueFrom.SecretKeyRef.Name] = true
					}
				}
			}
			for _, e := range c.EnvFrom {
				if e.ConfigMapRef != nil {
					usedCM[ns+"/"+e.ConfigMapRef.Name] = true
				}
				if e.SecretRef != nil {
					usedSec[ns+"/"+e.SecretRef.Name] = true
				}
			}
		}
	}

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		scanVolumes(d.Spec.Template.Spec.Volumes, d.Namespace)
		scanEnv(d.Spec.Template.Spec.Containers, d.Namespace)
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		scanVolumes(ss.Spec.Template.Spec.Volumes, ss.Namespace)
		scanEnv(ss.Spec.Template.Spec.Containers, ss.Namespace)
		// StatefulSet volumeClaimTemplates
		for _, vct := range ss.Spec.VolumeClaimTemplates {
			usedPVC[ss.Namespace+"/"+vct.Name] = true
		}
	}

	// Find orphaned ConfigMaps
	cmBatch := []string{}
	secBatch := []string{}
	pvcBatch := []string{}
	var totalStorageGB float64

	for _, cm := range configmaps.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		// Skip system-created CMs
		if cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"] != "" && !usedCM[cm.Namespace+"/"+cm.Name] {
			// still check usage
		}
		if !usedCM[cm.Namespace+"/"+cm.Name] {
			result.Summary.OrphanedCount++
			result.Summary.ConfigMaps++
			item := OrphanItem{
				Kind: "ConfigMap", Name: cm.Name, Namespace: cm.Namespace,
				Age:          svcAge(cm.CreationTimestamp.Time),
				SafeToDelete: true,
				Reason:       "No workload references this ConfigMap",
				DeleteCmd:    fmt.Sprintf("kubectl delete cm %s -n %s", cm.Name, cm.Namespace),
			}
			result.Orphans = append(result.Orphans, item)
			cmBatch = append(cmBatch, item.DeleteCmd)
		}
		result.Summary.TotalResources++
	}

	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		// Skip service-account tokens
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}
		if !usedSec[sec.Namespace+"/"+sec.Name] {
			result.Summary.OrphanedCount++
			result.Summary.Secrets++
			item := OrphanItem{
				Kind: "Secret", Name: sec.Name, Namespace: sec.Namespace,
				Age:          svcAge(sec.CreationTimestamp.Time),
				SafeToDelete: false,
				Reason:       "No workload references — verify before deleting (may be used by external tools)",
				DeleteCmd:    fmt.Sprintf("kubectl delete secret %s -n %s", sec.Name, sec.Namespace),
			}
			result.Orphans = append(result.Orphans, item)
			secBatch = append(secBatch, item.DeleteCmd)
		}
		result.Summary.TotalResources++
	}

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		if !usedPVC[pvc.Namespace+"/"+pvc.Name] {
			// Check if bound (has storage cost)
			storageGB := 0.0
			if pvc.Spec.Resources.Requests.Storage() != nil {
				storageGB = pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
			}
			totalStorageGB += storageGB

			result.Summary.OrphanedCount++
			result.Summary.PVCs++
			safeDelete := pvc.Status.Phase == corev1.ClaimLost || pvc.Status.Phase == corev1.ClaimPending
			item := OrphanItem{
				Kind: "PVC", Name: pvc.Name, Namespace: pvc.Namespace,
				Age:          svcAge(pvc.CreationTimestamp.Time),
				Size:         fmt.Sprintf("%.1fGB", storageGB),
				SafeToDelete: safeDelete,
			}
			if safeDelete {
				item.Reason = fmt.Sprintf("PVC phase=%s, safe to delete", pvc.Status.Phase)
			} else {
				item.Reason = "Bound PVC not referenced by any workload — verify data before deleting"
			}
			item.DeleteCmd = fmt.Sprintf("kubectl delete pvc %s -n %s", pvc.Name, pvc.Namespace)
			result.Orphans = append(result.Orphans, item)
			if safeDelete {
				pvcBatch = append(pvcBatch, item.DeleteCmd)
			}
		}
		result.Summary.TotalResources++
	}

	result.Summary.SafeToDelete = len(cmBatch) + len(pvcBatch)
	result.Summary.NeedsReview = result.Summary.Secrets + (result.Summary.PVCs - len(pvcBatch))
	if result.Summary.NeedsReview < 0 {
		result.Summary.NeedsReview = 0
	}

	// Build batch commands
	if len(cmBatch) > 0 {
		if len(cmBatch) > 20 {
			cmBatch = cmBatch[:20]
		}
		result.CleanupBatches = append(result.CleanupBatches, OrphanBatch{
			Title: "ConfigMap 清理", Kind: "ConfigMap", Commands: cmBatch, Count: len(cmBatch),
		})
	}
	if len(secBatch) > 0 {
		if len(secBatch) > 20 {
			secBatch = secBatch[:20]
		}
		result.CleanupBatches = append(result.CleanupBatches, OrphanBatch{
			Title: "Secret 清理 (需确认)", Kind: "Secret", Commands: secBatch, Count: len(secBatch),
		})
	}
	if len(pvcBatch) > 0 {
		result.CleanupBatches = append(result.CleanupBatches, OrphanBatch{
			Title: "PVC 清理", Kind: "PVC", Commands: pvcBatch, Count: len(pvcBatch),
		})
	}

	result.PotentialSave = OrphanSave{
		EstimatedStorageGB:   totalStorageGB,
		EstimatedMonthlyCost: totalStorageGB * 0.10, // ~$0.10/GB-month
		SecretCount:          result.Summary.Secrets,
	}

	// Score
	if result.Summary.TotalResources > 0 {
		result.HealthScore = (result.Summary.TotalResources - result.Summary.OrphanedCount) * 100 / result.Summary.TotalResources
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	// Sort orphans by age descending
	sort.Slice(result.Orphans, func(i, j int) bool {
		return result.Orphans[i].Age > result.Orphans[j].Age
	})

	result.Recommendations = buildOrphanRecs(&result)
	writeJSON(w, result)
}

func buildOrphanRecs(r *OrphanCleanupResult) []string {
	recs := []string{}
	if r.Summary.OrphanedCount == 0 {
		recs = append(recs, "没有孤立资源，集群资源利用率良好")
		return recs
	}
	if r.Summary.ConfigMaps > 0 {
		recs = append(recs, fmt.Sprintf("%d 个孤立 ConfigMap 可安全删除", r.Summary.ConfigMaps))
	}
	if r.Summary.Secrets > 0 {
		recs = append(recs, fmt.Sprintf("%d 个孤立 Secret 需确认后删除（可能是安全隐患）", r.Summary.Secrets))
	}
	if r.Summary.PVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d 个孤立 PVC 占用 %.1f GB 存储，约 $%.2f/月", r.Summary.PVCs, r.PotentialSave.EstimatedStorageGB, r.PotentialSave.EstimatedMonthlyCost))
	}
	if r.Summary.SafeToDelete > 0 {
		recs = append(recs, fmt.Sprintf("%d 个资源可立即安全删除，使用 cleanupBatches 中的命令", r.Summary.SafeToDelete))
	}
	return recs
}
