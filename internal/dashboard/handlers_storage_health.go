package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCHealthStatus describes the health of a PVC from a storage perspective.
type PVCHealthStatus string

const (
	PVCHealthBound        PVCHealthStatus = "bound"         // PVC is bound to a PV
	PVCHealthPending      PVCHealthStatus = "pending"       // PVC is waiting for provisioning
	PVCHealthLost         PVCHealthStatus = "lost"          // PVC's underlying PV was deleted/reclaimed
	PVCHealthFailed       PVCHealthStatus = "failed"        // PVC provisioning failed
	PVCHealthOrphaned     PVCHealthStatus = "orphaned"      // PVC bound but no pod is using it
	PVCHealthNearCapacity PVCHealthStatus = "near-capacity" // PVC usage is approaching limit (informer-based)
)

// PVCHealth describes the health of a single PVC.
type PVCHealth struct {
	Name              string          `json:"name"`
	Namespace         string          `json:"namespace"`
	Status            PVCHealthStatus `json:"status"`
	Phase             string          `json:"phase"` // raw K8s phase
	StorageClass      string          `json:"storageClass"`
	AccessModes       []string        `json:"accessModes,omitempty"`
	CapacityGB        float64         `json:"capacityGB"`
	RequestedGB       float64         `json:"requestedGB"`
	VolumeName        string          `json:"volumeName,omitempty"`
	BoundPV           string          `json:"boundPV,omitempty"`
	UsedByPods        []string        `json:"usedByPods,omitempty"`
	PodCount          int             `json:"podCount"`
	AgeDays           float64         `json:"ageDays"`
	ReclaimPolicy     string          `json:"reclaimPolicy,omitempty"`
	VolumeBindingMode string          `json:"volumeBindingMode,omitempty"`
	Issues            []string        `json:"issues,omitempty"`
}

// PVHealth describes the health of a standalone PersistentVolume.
type PVHealth struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"` // Available, Bound, Released, Failed
	ClaimRef      string   `json:"claimRef,omitempty"`
	CapacityGB    float64  `json:"capacityGB"`
	StorageClass  string   `json:"storageClass,omitempty"`
	ReclaimPolicy string   `json:"reclaimPolicy"`
	AgeDays       float64  `json:"ageDays"`
	Orphaned      bool     `json:"orphaned"`
	Issues        []string `json:"issues,omitempty"`
}

// StorageHealthResult is the full scan output.
type StorageHealthResult struct {
	ScannedAt      time.Time            `json:"scannedAt"`
	Summary        StorageHealthSummary `json:"summary"`
	PVCs           []PVCHealth          `json:"pvcs"`
	OrphanedPVs    []PVHealth           `json:"orphanedPVs,omitempty"`
	StorageClasses []StorageClassInfo   `json:"storageClasses"`
}

// StorageHealthSummary aggregates storage health statistics.
type StorageHealthSummary struct {
	TotalPVCs       int            `json:"totalPVCs"`
	PVCsByStatus    map[string]int `json:"pvcByStatus"`
	PendingPVCs     int            `json:"pendingPVCs"`
	OrphanedPVCs    int            `json:"orphanedPVCs"`
	TotalPVs        int            `json:"totalPVs"`
	ReleasedPVs     int            `json:"releasedPVs"`
	OrphanedPVCount int            `json:"orphanedPVCount"`
	TotalCapacityGB float64        `json:"totalCapacityGB"`
	UsedCapacityGB  float64        `json:"usedCapacityGB"`
}

// StorageClassInfo summarizes a storage class and its PVC distribution.
type StorageClassInfo struct {
	Name            string `json:"name"`
	IsDefault       bool   `json:"isDefault"`
	Provisioner     string `json:"provisioner"`
	ReclaimPolicy   string `json:"reclaimPolicy"`
	BindingMode     string `json:"bindingMode"`
	VolumeExpansion bool   `json:"allowVolumeExpansion"`
	PVCCount        int    `json:"pvcCount"`
	PendingCount    int    `json:"pendingCount"`
}

// handleStorageHealth scans all PVCs and PVs for storage health issues.
// GET /api/storage/health
func (s *Server) handleStorageHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	// List PVCs
	pvcList, err := rc.clientset.CoreV1().PersistentVolumeClaims(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List PVs
	pvList, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List storage classes
	scList, err := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List pods to find PVC usage
	podList, err := rc.clientset.CoreV1().Pods(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build PVC usage index: ns/pvcName -> []podName
	pvcUsage := make(map[string][]string)
	for i := range podList.Items {
		pod := &podList.Items[i]
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				podName := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
				pvcUsage[key] = append(pvcUsage[key], podName)
			}
		}
	}

	// Build PV name -> PV index
	pvByName := make(map[string]*corev1.PersistentVolume)
	for i := range pvList.Items {
		pvByName[pvList.Items[i].Name] = &pvList.Items[i]
	}

	// Build storage class index
	scByName := make(map[string]*storagev1.StorageClass)
	defaultSC := ""
	for i := range scList.Items {
		sc := &scList.Items[i]
		scByName[sc.Name] = sc
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultSC = sc.Name
		}
	}

	var pvcs []PVCHealth
	var orphanedPVs []PVHealth
	summary := StorageHealthSummary{PVCsByStatus: make(map[string]int)}
	scPVCCount := make(map[string]int)
	scPendingCount := make(map[string]int)

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		h := analyzePVCHealth(pvc, pvByName, scByName, pvcUsage)

		summary.TotalPVCs++
		summary.PVCsByStatus[string(h.Status)]++
		if h.Status == PVCHealthPending || h.Status == PVCHealthFailed {
			summary.PendingPVCs++
		}
		if h.Status == PVCHealthOrphaned {
			summary.OrphanedPVCs++
		}
		summary.TotalCapacityGB += h.RequestedGB
		if h.Status == PVCHealthBound {
			summary.UsedCapacityGB += h.CapacityGB
		}

		scName := h.StorageClass
		if scName == "" {
			scName = defaultSC
		}
		scPVCCount[scName]++
		if h.Status == PVCHealthPending || h.Status == PVCHealthFailed {
			scPendingCount[scName]++
		}

		pvcs = append(pvcs, h)
	}

	// Find orphaned/released PVs
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		ph := analyzePVHealth(pv)
		summary.TotalPVs++

		if pv.Status.Phase == corev1.VolumeReleased {
			summary.ReleasedPVs++
		}
		if ph.Orphaned {
			summary.OrphanedPVCount++
			orphanedPVs = append(orphanedPVs, ph)
		}
	}

	// Build storage class info
	var scInfos []StorageClassInfo
	for i := range scList.Items {
		sc := &scList.Items[i]
		sci := StorageClassInfo{
			Name:            sc.Name,
			IsDefault:       sc.Name == defaultSC,
			Provisioner:     sc.Provisioner,
			ReclaimPolicy:   string(*sc.ReclaimPolicy),
			BindingMode:     string(*sc.VolumeBindingMode),
			VolumeExpansion: sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion,
			PVCCount:        scPVCCount[sc.Name],
			PendingCount:    scPendingCount[sc.Name],
		}
		scInfos = append(scInfos, sci)
	}

	// Sort PVCs: problematic first
	sort.Slice(pvcs, func(i, j int) bool {
		rankI := pvcStatusRank(pvcs[i].Status)
		rankJ := pvcStatusRank(pvcs[j].Status)
		if rankI != rankJ {
			return rankI < rankJ
		}
		return pvcs[i].Namespace+"/"+pvcs[i].Name < pvcs[j].Namespace+"/"+pvcs[j].Name
	})

	// Sort orphaned PVs by age (oldest first)
	sort.Slice(orphanedPVs, func(i, j int) bool {
		return orphanedPVs[i].AgeDays > orphanedPVs[j].AgeDays
	})

	// Sort storage classes: default first, then by PVC count desc
	sort.Slice(scInfos, func(i, j int) bool {
		if scInfos[i].IsDefault != scInfos[j].IsDefault {
			return scInfos[i].IsDefault
		}
		return scInfos[i].PVCCount > scInfos[j].PVCCount
	})

	writeJSON(w, StorageHealthResult{
		ScannedAt:      time.Now(),
		Summary:        summary,
		PVCs:           pvcs,
		OrphanedPVs:    orphanedPVs,
		StorageClasses: scInfos,
	})
}

// analyzePVCHealth evaluates the health of a single PVC.
func analyzePVCHealth(
	pvc *corev1.PersistentVolumeClaim,
	pvByName map[string]*corev1.PersistentVolume,
	scByName map[string]*storagev1.StorageClass,
	pvcUsage map[string][]string,
) PVCHealth {
	h := PVCHealth{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		Phase:     string(pvc.Status.Phase),
	}

	// Storage class (explicit or default)
	scName := ""
	if pvc.Spec.StorageClassName != nil {
		scName = *pvc.Spec.StorageClassName
	}
	if scName == "" {
		// Check annotations for default SC
		if ann, ok := pvc.Annotations["volume.beta.kubernetes.io/storage-class"]; ok {
			scName = ann
		}
	}
	if scName != "" {
		h.StorageClass = scName
	}

	// Access modes
	for _, am := range pvc.Spec.AccessModes {
		h.AccessModes = append(h.AccessModes, string(am))
	}

	// Requested storage
	if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		h.RequestedGB = float64(req.Value()) / 1024 / 1024 / 1024
	}

	// Actual capacity
	if pvc.Status.Capacity != nil {
		if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			h.CapacityGB = float64(storage.Value()) / 1024 / 1024 / 1024
		}
	}

	// Volume name
	if pvc.Spec.VolumeName != "" {
		h.VolumeName = pvc.Spec.VolumeName
		h.BoundPV = pvc.Spec.VolumeName
	}

	// Age
	h.AgeDays = time.Since(pvc.CreationTimestamp.Time).Hours() / 24

	// Pod usage
	key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
	h.UsedByPods = pvcUsage[key]
	h.PodCount = len(h.UsedByPods)

	// Determine health status
	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		if h.PodCount == 0 && h.AgeDays > 1 {
			h.Status = PVCHealthOrphaned
			h.Issues = append(h.Issues, fmt.Sprintf("PVC is bound but not mounted by any pod for %.0f days", h.AgeDays))
		} else {
			h.Status = PVCHealthBound
		}

		// Check reclaim policy from PV
		if pv, ok := pvByName[h.VolumeName]; ok {
			h.ReclaimPolicy = string(pv.Spec.PersistentVolumeReclaimPolicy)
		}

	case corev1.ClaimPending:
		h.Status = PVCHealthPending
		// Diagnose why it's pending
		if h.StorageClass == "" {
			h.Issues = append(h.Issues, "No storage class specified and no default storage class configured")
		} else {
			if sc, ok := scByName[h.StorageClass]; ok {
				h.VolumeBindingMode = string(*sc.VolumeBindingMode)
				if *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
					h.Issues = append(h.Issues, "Storage class uses WaitForFirstConsumer — waiting for a pod to schedule")
				} else {
					h.Issues = append(h.Issues, fmt.Sprintf("PVC pending in storage class '%s' — check provisioner logs and capacity", h.StorageClass))
				}
			} else {
				h.Issues = append(h.Issues, fmt.Sprintf("Storage class '%s' does not exist", h.StorageClass))
			}
		}
		// Check for provisioning failures in events
		for _, cond := range pvc.Status.Conditions {
			if cond.Type == corev1.PersistentVolumeClaimResizing && cond.Status == corev1.ConditionTrue {
				h.Issues = append(h.Issues, "PVC is being resized")
			}
		}

	case corev1.ClaimLost:
		h.Status = PVCHealthLost
		h.Issues = append(h.Issues, "Underlying PersistentVolume was lost or deleted")

	default:
		h.Status = PVCHealthFailed
		h.Issues = append(h.Issues, fmt.Sprintf("PVC in unknown phase: %s", pvc.Status.Phase))
	}

	return h
}

// analyzePVHealth evaluates the health of a standalone PersistentVolume.
func analyzePVHealth(pv *corev1.PersistentVolume) PVHealth {
	h := PVHealth{
		Name:          pv.Name,
		Status:        string(pv.Status.Phase),
		ReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
		AgeDays:       time.Since(pv.CreationTimestamp.Time).Hours() / 24,
	}

	// Capacity
	if cap, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
		h.CapacityGB = float64(cap.Value()) / 1024 / 1024 / 1024
	}

	// Storage class
	if pv.Spec.StorageClassName != "" {
		h.StorageClass = pv.Spec.StorageClassName
	}

	// Claim reference
	if pv.Spec.ClaimRef != nil {
		h.ClaimRef = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}

	// Detect orphaned/released PVs
	if pv.Status.Phase == corev1.VolumeReleased {
		h.Orphaned = true
		h.Issues = append(h.Issues, fmt.Sprintf("PV is Released (claim %s deleted) — reclaim policy: %s", h.ClaimRef, h.ReclaimPolicy))
		if h.ReclaimPolicy == string(corev1.PersistentVolumeReclaimRetain) {
			h.Issues = append(h.Issues, "Reclaim policy is Retain — PV must be manually cleaned up")
		}
	}

	// Detect Available PVs with Retain policy that are old (potential waste)
	if pv.Status.Phase == corev1.VolumeAvailable && h.AgeDays > 7 {
		h.Orphaned = true
		h.Issues = append(h.Issues, fmt.Sprintf("PV has been Available for %.0f days — potential wasted storage", h.AgeDays))
	}

	// Detect Failed PVs
	if pv.Status.Phase == corev1.VolumeFailed {
		h.Orphaned = true
		h.Issues = append(h.Issues, "PV is in Failed state — reclaim/recycle failed")
	}

	return h
}

// pvcStatusRank returns sort priority (lower = more problematic).
func pvcStatusRank(status PVCHealthStatus) int {
	switch status {
	case PVCHealthFailed:
		return 0
	case PVCHealthLost:
		return 1
	case PVCHealthPending:
		return 2
	case PVCHealthNearCapacity:
		return 3
	case PVCHealthOrphaned:
		return 4
	case PVCHealthBound:
		return 5
	default:
		return 6
	}
}

// parseStorageGB parses a resource.Quantity and returns GB as float64.
func parseStorageGB(q resource.Quantity) float64 {
	return float64(q.Value()) / 1024 / 1024 / 1024
}

// joinAccessModes joins access modes into a comma-separated string.
func joinAccessModes(modes []corev1.PersistentVolumeAccessMode) string {
	var parts []string
	for _, m := range modes {
		parts = append(parts, string(m))
	}
	return strings.Join(parts, ", ")
}
