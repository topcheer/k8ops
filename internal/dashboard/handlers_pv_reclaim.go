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

// PVReclaimResult is the PV reclaim policy & storage class waste audit.
type PVReclaimResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         PVReclaimSummary  `json:"summary"`
	ByNamespace     []PVReclaimNSStat `json:"byNamespace"`
	ByStorageClass  []PVSCStat        `json:"byStorageClass"`
	Volumes         []PVEntry         `json:"volumes"`
	Issues          []PVIssue         `json:"issues"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// PVReclaimSummary aggregates PV reclaim statistics.
type PVReclaimSummary struct {
	TotalPVs       int `json:"totalPVs"`
	BoundPVs       int `json:"boundPVs"`
	ReleasedPVs    int `json:"releasedPVs"`
	PendingPVs     int `json:"pendingPVs"`
	FailedPVs      int `json:"failedPVs"`
	DeleteReclaim  int `json:"deleteReclaimPolicy"`
	RetainReclaim  int `json:"retainReclaimPolicy"`
	OrphanedPVs    int `json:"orphanedPVs"`
	NoStorageClass int `json:"noStorageClass"`
	TotalPVCs      int `json:"totalPVCs"`
	PendingPVCs    int `json:"pendingPVCs"`
}

// PVReclaimNSStat per-namespace PV stats.
type PVReclaimNSStat struct {
	Namespace  string `json:"namespace"`
	PVCCount   int    `json:"pvcCount"`
	PendingPVC int    `json:"pendingPVC"`
}

// PVSCStat per-storage-class stats.
type PVSCStat struct {
	StorageClass  string `json:"storageClass"`
	PVCount       int    `json:"pvCount"`
	BoundCount    int    `json:"boundCount"`
	ReleasedCount int    `json:"releasedCount"`
	ReclaimPolicy string `json:"reclaimPolicy"`
}

// PVEntry describes one PV.
type PVEntry struct {
	Name          string `json:"name"`
	Phase         string `json:"phase"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	StorageClass  string `json:"storageClass"`
	Capacity      string `json:"capacity"`
	ClaimRef      string `json:"claimRef,omitempty"`
	Age           string `json:"age"`
	Orphaned      bool   `json:"orphaned"`
	RiskLevel     string `json:"riskLevel"`
}

// PVIssue is a detected PV problem.
type PVIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handlePVReclaim audits PV reclaim policy & storage class waste.
// GET /api/scalability/pv-reclaim
func (s *Server) handlePVReclaim(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &PVReclaimResult{
		ScannedAt: time.Now(),
	}

	// List PVs
	pvs, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List PVCs
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List StorageClasses
	scs, err := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build SC map: name -> reclaim policy
	scReclaim := make(map[string]string)
	for _, sc := range scs.Items {
		policy := string(corev1.PersistentVolumeReclaimRetain)
		if sc.ReclaimPolicy != nil {
			policy = string(*sc.ReclaimPolicy)
		}
		scReclaim[sc.Name] = policy
	}

	// Build PVC claim map: pvcUID -> namespace/name
	pvcMap := make(map[string]string)
	for _, pvc := range pvcs.Items {
		uid := string(pvc.UID)
		pvcMap[uid] = fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
	}

	var entries []PVEntry
	var issues []PVIssue
	scStats := make(map[string]*PVSCStat)
	nsStats := make(map[string]*PVReclaimNSStat)

	deleteReclaim := 0
	retainReclaim := 0
	releasedPVs := 0
	pendingPVs := 0
	failedPVs := 0
	boundPVs := 0
	orphanedPVs := 0
	noSC := 0

	for i := range pvs.Items {
		pv := &pvs.Items[i]

		entry := PVEntry{
			Name:          pv.Name,
			Phase:         string(pv.Status.Phase),
			ReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
			StorageClass:  pv.Spec.StorageClassName,
			RiskLevel:     "healthy",
		}

		// Capacity
		if pv.Spec.Capacity != nil {
			if cap, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
				entry.Capacity = cap.String()
			}
		}

		// Age
		entry.Age = time.Since(pv.CreationTimestamp.Time).Round(time.Hour * 24).String()

		// Reclaim policy stats
		if entry.ReclaimPolicy == "Delete" {
			deleteReclaim++
		} else if entry.ReclaimPolicy == "Retain" {
			retainReclaim++
		}

		// No storage class
		if entry.StorageClass == "" {
			noSC++
		}

		// Phase stats
		switch pv.Status.Phase {
		case corev1.VolumeBound:
			boundPVs++
		case corev1.VolumeReleased:
			releasedPVs++
			// Released PVs with Retain policy are orphaned
			if entry.ReclaimPolicy == "Retain" {
				orphanedPVs++
				entry.Orphaned = true
				entry.RiskLevel = "warning"
				issues = append(issues, PVIssue{
					Severity: "warning",
					Type:     "released-pv-retain",
					Resource: pv.Name,
					Message:  fmt.Sprintf("PV '%s' is Released with Retain policy — manual cleanup needed, storage is wasted", pv.Name),
				})
			}
		case corev1.VolumePending:
			pendingPVs++
			entry.RiskLevel = "info"
		case corev1.VolumeFailed:
			failedPVs++
			entry.RiskLevel = "critical"
			issues = append(issues, PVIssue{
				Severity: "critical",
				Type:     "pv-failed",
				Resource: pv.Name,
				Message:  fmt.Sprintf("PV '%s' is in Failed state — storage may be corrupted or unavailable", pv.Name),
			})
		}

		// Claim reference
		if pv.Spec.ClaimRef != nil {
			entry.ClaimRef = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}

		// Delete reclaim with bound PVC warning
		if entry.ReclaimPolicy == "Delete" && pv.Status.Phase == corev1.VolumeBound {
			issues = append(issues, PVIssue{
				Severity: "info",
				Type:     "delete-reclaim-bound",
				Resource: pv.Name,
				Message:  fmt.Sprintf("PV '%s' uses Delete reclaim — data will be lost when PVC is deleted", pv.Name),
			})
		}

		entries = append(entries, entry)

		// SC stats
		scName := entry.StorageClass
		if scName == "" {
			scName = "(none)"
		}
		if _, ok := scStats[scName]; !ok {
			scStats[scName] = &PVSCStat{
				StorageClass:  scName,
				ReclaimPolicy: scReclaim[entry.StorageClass],
			}
		}
		scStats[scName].PVCount++
		switch pv.Status.Phase {
		case corev1.VolumeBound:
			scStats[scName].BoundCount++
		case corev1.VolumeReleased:
			scStats[scName].ReleasedCount++
		}
	}

	// PVC stats
	totalPVCs := 0
	pendingPVCs := 0
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		totalPVCs++
		if pvc.Status.Phase == corev1.ClaimPending {
			pendingPVCs++
		}

		if _, ok := nsStats[pvc.Namespace]; !ok {
			nsStats[pvc.Namespace] = &PVReclaimNSStat{Namespace: pvc.Namespace}
		}
		nsStats[pvc.Namespace].PVCCount++
		if pvc.Status.Phase == corev1.ClaimPending {
			nsStats[pvc.Namespace].PendingPVC++
		}

		if pvc.Status.Phase == corev1.ClaimPending {
			issues = append(issues, PVIssue{
				Severity: "warning",
				Type:     "pvc-pending",
				Resource: fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
				Message:  fmt.Sprintf("PVC '%s' is Pending — no PV available or storage class provisioning failed", pvc.Name),
			})
		}
	}

	// Convert stats to slices
	for _, sc := range scStats {
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].PVCount > result.ByStorageClass[j].PVCount
	})

	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PendingPVC > result.ByNamespace[j].PendingPVC
	})

	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if orphanedPVs > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d released PV(s) with Retain policy are orphaned — manually delete or re-bind to save storage costs", orphanedPVs))
	}
	if failedPVs > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d PV(s) are in Failed state — investigate storage backend health", failedPVs))
	}
	if pendingPVCs > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d PVC(s) are Pending — check storage class provisioning and capacity", pendingPVCs))
	}
	if deleteReclaim > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d PV(s) use Delete reclaim — ensure this is intentional, data will be lost on PVC deletion", deleteReclaim))
	}
	if noSC > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d PV(s) have no storage class — may cause provisioning issues", noSC))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Storage management is healthy — PVs are properly configured and bound")
	}

	result.Volumes = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = PVReclaimSummary{
		TotalPVs:       len(entries),
		BoundPVs:       boundPVs,
		ReleasedPVs:    releasedPVs,
		PendingPVs:     pendingPVs,
		FailedPVs:      failedPVs,
		DeleteReclaim:  deleteReclaim,
		RetainReclaim:  retainReclaim,
		OrphanedPVs:    orphanedPVs,
		NoStorageClass: noSC,
		TotalPVCs:      totalPVCs,
		PendingPVCs:    pendingPVCs,
	}
	result.HealthScore = computePVReclaimScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// computePVReclaimScore computes a 0-100 health score.
func computePVReclaimScore(s PVReclaimSummary, issueCount int) int {
	if s.TotalPVs == 0 {
		return 100
	}
	score := 100
	score -= s.FailedPVs * 15
	score -= s.PendingPVCs * 5
	score -= s.OrphanedPVs * 3
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
