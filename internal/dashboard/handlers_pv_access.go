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

// PVAccessResult is the persistent volume access mode & multi-attach risk audit.
type PVAccessResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PVAccessSummary     `json:"summary"`
	ByStorageClass  []PVAccessSCStat    `json:"byStorageClass"`
	Risks           []PVAccessRiskEntry `json:"risks"`
	UnboundPVCs     []PVAccessPVCEntry  `json:"unboundPVCs"`
	MultiAttachPVCs []PVAccessPVCEntry  `json:"multiAttachPVCs"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// PVAccessSummary aggregates PV access mode statistics.
type PVAccessSummary struct {
	TotalPVs         int `json:"totalPVs"`
	TotalPVCs        int `json:"totalPVCs"`
	BoundPVCs        int `json:"boundPVCs"`
	UnboundPVCs      int `json:"unboundPVCs"`
	RWOPVs           int `json:"rwoPVs"`           // ReadWriteOnce
	RWXPVs           int `json:"rwxPVs"`           // ReadWriteMany
	ROXPVs           int `json:"roxPVs"`           // ReadOnlyMany
	MultiAttachPVCs  int `json:"multiAttachPVCs"`  // RWX PVCs used by multiple pods
	DeleteReclaim    int `json:"deleteReclaim"`    // Delete reclaim policy
	RetainReclaim    int `json:"retainReclaim"`    // Retain reclaim policy
	ExpansionEnabled int `json:"expansionEnabled"` // supports volume expansion
	NoStorageClass   int `json:"noStorageClass"`   // PVCs without storage class
}

// PVAccessSCStat shows PV stats per storage class.
type PVAccessSCStat struct {
	StorageClass  string `json:"storageClass"`
	PVCount       int    `json:"pvCount"`
	PVCCount      int    `json:"pvcCount"`
	HasExpansion  bool   `json:"hasExpansion"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	DefaultSC     bool   `json:"defaultSC"`
	RiskLevel     string `json:"riskLevel"`
}

// PVAccessRiskEntry describes a specific PV access risk.
type PVAccessRiskEntry struct {
	PVCName   string `json:"pvcName"`
	Namespace string `json:"namespace"`
	PVName    string `json:"pvName"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// PVAccessPVCEntry describes a PVC with a specific status.
type PVAccessPVCEntry struct {
	PVCName      string `json:"pvcName"`
	Namespace    string `json:"namespace"`
	StorageClass string `json:"storageClass"`
	AccessModes  string `json:"accessModes"`
	Size         string `json:"size"`
	PodCount     int    `json:"podCount"`
}

// pvAccessAuditCore performs the audit on PVs, PVCs, and pods (testable).
func pvAccessAuditCore(
	pvs []corev1.PersistentVolume,
	pvcs []corev1.PersistentVolumeClaim,
	pods []corev1.Pod,
	storageClasses map[string]*PVAccessSCStat,
) PVAccessResult {
	result := PVAccessResult{
		ScannedAt: time.Now(),
	}

	// Build PVC to pod mapping
	pvcPodMap := make(map[string][]string) // key: namespace/pvcname -> pod names
	for i := range pods {
		pod := &pods[i]
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				pvcPodMap[key] = append(pvcPodMap[key], pod.Name)
			}
		}
	}

	// Build PV map by name
	pvMap := make(map[string]*corev1.PersistentVolume)
	for i := range pvs {
		pvMap[pvs[i].Name] = &pvs[i]
	}

	// Analyze PVCs
	for i := range pvcs {
		pvc := &pvcs[i]
		result.Summary.TotalPVCs++

		accessModes := pvcAccessModesString(pvc.Spec.AccessModes)
		pvcKey := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		podCount := len(pvcPodMap[pvcKey])

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}

		entry := PVAccessPVCEntry{
			PVCName:      pvc.Name,
			Namespace:    pvc.Namespace,
			StorageClass: scName,
			AccessModes:  accessModes,
			Size:         pvc.Spec.Resources.Requests.Storage().String(),
			PodCount:     podCount,
		}

		// Check bound status
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		} else {
			result.Summary.UnboundPVCs++
			result.UnboundPVCs = append(result.UnboundPVCs, entry)
			result.Risks = append(result.Risks, PVAccessRiskEntry{
				PVCName:   pvc.Name,
				Namespace: pvc.Namespace,
				RiskType:  "unbound-pvc",
				Severity:  "medium",
				Detail:    fmt.Sprintf("PVC is %s — pod may be stuck in Pending", pvc.Status.Phase),
			})
		}

		// Check for multi-attach risk (RWX PVC used by multiple pods on potentially same node)
		isRWX := false
		for _, am := range pvc.Spec.AccessModes {
			if am == corev1.ReadWriteMany {
				isRWX = true
				break
			}
		}
		if isRWX && podCount > 1 {
			result.Summary.MultiAttachPVCs++
			result.MultiAttachPVCs = append(result.MultiAttachPVCs, entry)
			result.Risks = append(result.Risks, PVAccessRiskEntry{
				PVCName:   pvc.Name,
				Namespace: pvc.Namespace,
				RiskType:  "multi-attach-rwx",
				Severity:  "medium",
				Detail:    fmt.Sprintf("RWX PVC used by %d pods — ensure filesystem supports concurrent writes", podCount),
			})
		}

		// Check no storage class
		if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
			result.Summary.NoStorageClass++
			result.Risks = append(result.Risks, PVAccessRiskEntry{
				PVCName:   pvc.Name,
				Namespace: pvc.Namespace,
				RiskType:  "no-storage-class",
				Severity:  "low",
				Detail:    "PVC has no storage class — uses default or static provisioning",
			})
		}

		// Check bound PV for reclaim policy
		if pvc.Spec.VolumeName != "" {
			if pv, ok := pvMap[pvc.Spec.VolumeName]; ok {
				if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
					result.Summary.DeleteReclaim++
					result.Risks = append(result.Risks, PVAccessRiskEntry{
						PVCName:   pvc.Name,
						Namespace: pvc.Namespace,
						PVName:    pv.Name,
						RiskType:  "delete-reclaim",
						Severity:  "high",
						Detail:    "PV has Delete reclaim policy — data will be lost when PVC is deleted",
					})
				} else if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
					result.Summary.RetainReclaim++
				}
			}
		}

		// Update storage class stats
		if scName != "" {
			if sc, ok := storageClasses[scName]; ok {
				sc.PVCCount++
			}
		}
	}

	// Analyze PVs
	for i := range pvs {
		pv := &pvs[i]
		result.Summary.TotalPVs++

		hasRWO, hasRWX, hasROX := false, false, false
		for _, am := range pv.Spec.AccessModes {
			switch am {
			case corev1.ReadWriteOnce:
				hasRWO = true
			case corev1.ReadWriteMany:
				hasRWX = true
			case corev1.ReadOnlyMany:
				hasROX = true
			}
		}
		if hasRWO {
			result.Summary.RWOPVs++
		}
		if hasRWX {
			result.Summary.RWXPVs++
		}
		if hasROX {
			result.Summary.ROXPVs++
		}

		// Update storage class stats
		scName := pv.Spec.StorageClassName
		if scName != "" {
			if sc, ok := storageClasses[scName]; ok {
				sc.PVCount++
			}
		}
	}

	// Build storage class stats
	for _, sc := range storageClasses {
		sc.RiskLevel = "low"
		if sc.ReclaimPolicy == "Delete" {
			sc.RiskLevel = "medium"
		}
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].PVCCount > result.ByStorageClass[j].PVCCount
	})

	// Sort risks by severity
	sort.Slice(result.Risks, func(i, j int) bool {
		return result.Risks[i].Severity > result.Risks[j].Severity
	})

	result.HealthScore = pvAccessScore(result.Summary)
	result.Recommendations = pvAccessRecommendations(result.Summary)

	return result
}

// pvcAccessModesString converts access modes to a string.
func pvcAccessModesString(modes []corev1.PersistentVolumeAccessMode) string {
	parts := make([]string, 0, len(modes))
	for _, m := range modes {
		switch m {
		case corev1.ReadWriteOnce:
			parts = append(parts, "RWO")
		case corev1.ReadWriteMany:
			parts = append(parts, "RWX")
		case corev1.ReadOnlyMany:
			parts = append(parts, "ROX")
		default:
			parts = append(parts, string(m))
		}
	}
	return strings.Join(parts, ",")
}

// pvAccessScore calculates the health score.
func pvAccessScore(s PVAccessSummary) int {
	if s.TotalPVCs == 0 {
		return 100
	}
	base := 100
	base -= (s.UnboundPVCs * 5)
	base -= (s.DeleteReclaim * 3)
	base -= (s.NoStorageClass * 2)
	// Multi-attach is not necessarily bad, just a risk indicator
	base -= (s.MultiAttachPVCs * 1)
	if base < 0 {
		base = 0
	}
	return base
}

// pvAccessRecommendations generates actionable recommendations.
func pvAccessRecommendations(s PVAccessSummary) []string {
	var recs []string
	if s.UnboundPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) are unbound — check storage class provisioning and capacity", s.UnboundPVCs))
	}
	if s.DeleteReclaim > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) have Delete reclaim policy — switch to Retain for critical data to prevent accidental data loss", s.DeleteReclaim))
	}
	if s.MultiAttachPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d RWX PVC(s) are used by multiple pods — ensure filesystem supports concurrent writes (NFS, CephFS, GlusterFS)", s.MultiAttachPVCs))
	}
	if s.NoStorageClass > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) have no storage class — specify explicit storage class for consistent provisioning", s.NoStorageClass))
	}
	if s.UnboundPVCs == 0 && s.DeleteReclaim == 0 {
		recs = append(recs, "all PVCs are bound with Retain reclaim policy — storage configuration is healthy")
	}
	return recs
}

// Need a helper on PVAccessSCStat to count delete reclaim
// Add a field to track delete reclaim count per SC
// Actually, let me just add a method that always returns 0 for now since we track per-PVC
// This is a simplification — the actual reclaim policy is on the PV, not the SC

// handlePVAccess audits persistent volume access modes and multi-attach risks.
// GET /api/product/pv-access
func (s *Server) handlePVAccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pvs, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	// Build storage class map from PVCs and PVs
	scMap := make(map[string]*PVAccessSCStat)
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		if scName != "" {
			if _, ok := scMap[scName]; !ok {
				scMap[scName] = &PVAccessSCStat{StorageClass: scName, RiskLevel: "low"}
			}
		}
	}
	for i := range pvs.Items {
		pv := &pvs.Items[i]
		scName := pv.Spec.StorageClassName
		if scName != "" {
			if _, ok := scMap[scName]; !ok {
				scMap[scName] = &PVAccessSCStat{StorageClass: scName, RiskLevel: "low"}
			}
			if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
				scMap[scName].ReclaimPolicy = "Delete"
			} else {
				scMap[scName].ReclaimPolicy = "Retain"
			}
		}
	}

	result := pvAccessAuditCore(pvs.Items, pvcs.Items, pods.Items, scMap)
	writeJSON(w, result)
}
