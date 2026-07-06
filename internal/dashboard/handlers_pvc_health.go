package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVHResult is the PV/PVC storage health analysis.
type PVHResult struct {
	ScannedAt       time.Time    `json:"scannedAt"`
	Summary         PVHSummary   `json:"summary"`
	PVCs            []PVHEntry   `json:"pvcs"`
	PendingPVCs     []PVHEntry   `json:"pendingPVCs"` // stuck in Pending
	LostPVCs        []PVHEntry   `json:"lostPVCs"`    // Bound but PV is Lost/Failed
	PVs             []PVHEntry   `json:"pvs"`         // standalone PVs
	ReleasedPVs     []PVHEntry   `json:"releasedPVs"` // PV released but not reclaimed
	StorageClasses  []PVHSCEntry `json:"storageClasses"`
	ByNamespace     []PVHNSEntry `json:"byNamespace"`
	Issues          []PVHIssue   `json:"issues"`
	Recommendations []string     `json:"recommendations"`
}

// PVHSummary aggregates storage health statistics.
type PVHSummary struct {
	TotalPVCs           int `json:"totalPVCs"`
	BoundPVCs           int `json:"boundPVCs"`
	PendingPVCs         int `json:"pendingPVCs"`
	LostPVCs            int `json:"lostPVCs"`
	TotalPVs            int `json:"totalPVs"`
	ReleasedPVs         int `json:"releasedPVs"`
	FailedPVs           int `json:"failedPVs"`
	TotalStorageClasses int `json:"totalStorageClasses"`
	DefaultSCCount      int `json:"defaultSCCount"`
	NoExpandingSC       int `json:"noExpandingSC"`    // SCs without allowVolumeExpansion
	ReclaimDeletePVs    int `json:"reclaimDeletePVs"` // PV with Reclaim Delete
	ReclaimRetainPVs    int `json:"reclaimRetainPVs"` // PV with Reclaim Retain
	HealthScore         int `json:"healthScore"`      // 0-100
}

// PVHEntry describes one PVC or PV.
type PVHEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace,omitempty"`
	Kind          string `json:"kind"`  // PVC / PV
	Phase         string `json:"phase"` // Bound / Pending / Lost / Available / Released / Failed
	StorageClass  string `json:"storageClass"`
	AccessModes   string `json:"accessModes"`
	Capacity      string `json:"capacity,omitempty"`
	BoundVolume   string `json:"boundVolume,omitempty"` // PVC → PV name or PV → PVC name
	ReclaimPolicy string `json:"reclaimPolicy,omitempty"`
	Age           string `json:"age"`
	RiskLevel     string `json:"riskLevel"`
	Reason        string `json:"reason,omitempty"`
}

// PVHSCEntry describes one StorageClass.
type PVHSCEntry struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	AllowExpansion    bool   `json:"allowVolumeExpansion"`
	IsDefault         bool   `json:"isDefault"`
	PVCCount          int    `json:"pvcCount"`
}

// PVHNSEntry per-namespace PVC stats.
type PVHNSEntry struct {
	Namespace    string `json:"namespace"`
	PVCCount     int    `json:"pvcCount"`
	BoundCount   int    `json:"boundCount"`
	PendingCount int    `json:"pendingCount"`
}

// PVHIssue is a detected storage problem.
type PVHIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handlePVCHealth audits PV/PVC storage health and capacity.
// GET /api/product/pvc-health
func (s *Server) handlePVCHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pvs, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	scs, err := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := PVHResult{ScannedAt: time.Now()}
	now := time.Now()
	nsMap := make(map[string]*PVHNSEntry)

	// Build SC → PVC count and default SC
	scPVCCount := make(map[string]int)
	defaultSC := ""
	for _, sc := range scs.Items {
		if isDefaultSC(sc) {
			defaultSC = sc.Name
		}
	}

	// Analyze PVCs
	pvMap := make(map[string]*corev1.PersistentVolume)
	for i := range pvs.Items {
		pvMap[pvs.Items[i].Name] = &pvs.Items[i]
	}

	for _, pvc := range pvcs.Items {
		result.Summary.TotalPVCs++

		entry := PVHEntry{
			Name:         pvc.Name,
			Namespace:    pvc.Namespace,
			Kind:         "PVC",
			Phase:        string(pvc.Status.Phase),
			StorageClass: *pvc.Spec.StorageClassName,
			Age:          now.Sub(pvc.CreationTimestamp.Time).Round(time.Hour).String(),
		}

		if len(pvc.Spec.AccessModes) > 0 {
			entry.AccessModes = pvhAccessModes(pvc.Spec.AccessModes)
		}
		if pvc.Spec.VolumeName != "" {
			entry.BoundVolume = pvc.Spec.VolumeName
		}
		if cap, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			entry.Capacity = cap.String()
		}

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
			// Check if PVC is Lost (underlying PV is gone)
			if pvc.Status.Phase == corev1.ClaimLost {
				result.Summary.LostPVCs++
				entry.RiskLevel = "critical"
				entry.Reason = "PVC is Lost — underlying PV is gone, data may be lost"
				result.LostPVCs = append(result.LostPVCs, entry)
				result.Issues = append(result.Issues, PVHIssue{
					Severity: "critical", Type: "lost-pvc",
					Resource: fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
					Message:  fmt.Sprintf("PVC %s/%s is Lost — underlying PV is gone, data may be lost", pvc.Namespace, pvc.Name),
				})
			} else if pv, ok := pvMap[pvc.Spec.VolumeName]; ok {
				if pv.Status.Phase == corev1.VolumeFailed {
					result.Summary.LostPVCs++
					entry.RiskLevel = "critical"
					entry.Reason = fmt.Sprintf("Bound PV %s is Failed", pv.Name)
					result.LostPVCs = append(result.LostPVCs, entry)
					result.Issues = append(result.Issues, PVHIssue{
						Severity: "critical", Type: "lost-pvc",
						Resource: fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
						Message:  fmt.Sprintf("PVC %s/%s bound to PV %s which is Failed — data at risk", pvc.Namespace, pvc.Name, pv.Name),
					})
				} else {
					entry.RiskLevel = "low"
				}
			} else {
				entry.RiskLevel = "low"
			}
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
			entry.RiskLevel = "high"
			entry.Reason = "Pending — storage provisioning may be stuck"
			result.PendingPVCs = append(result.PendingPVCs, entry)
			result.Issues = append(result.Issues, PVHIssue{
				Severity: "warning", Type: "pending-pvc",
				Resource: fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
				Message:  fmt.Sprintf("PVC %s/%s is Pending — check StorageClass provisioner and capacity", pvc.Namespace, pvc.Name),
			})
		default:
			entry.RiskLevel = "medium"
		}

		scPVCCount[entry.StorageClass]++
		result.PVCs = append(result.PVCs, entry)

		// Namespace tracking
		nsStat := pvhGetOrCreateNS(nsMap, pvc.Namespace)
		nsStat.PVCCount++
		if pvc.Status.Phase == corev1.ClaimBound {
			nsStat.BoundCount++
		} else if pvc.Status.Phase == corev1.ClaimPending {
			nsStat.PendingCount++
		}
	}

	// Analyze PVs
	pvcBindMap := make(map[string]bool) // PV names that are bound to a PVC
	for _, pvc := range pvcs.Items {
		if pvc.Spec.VolumeName != "" {
			pvcBindMap[pvc.Spec.VolumeName] = true
		}
	}

	for _, pv := range pvs.Items {
		result.Summary.TotalPVs++

		entry := PVHEntry{
			Name:  pv.Name,
			Kind:  "PV",
			Phase: string(pv.Status.Phase),
			Age:   now.Sub(pv.CreationTimestamp.Time).Round(time.Hour).String(),
		}
		if pv.Spec.StorageClassName != "" {
			entry.StorageClass = pv.Spec.StorageClassName
		}
		if pv.Spec.ClaimRef != nil {
			entry.BoundVolume = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		if len(pv.Spec.AccessModes) > 0 {
			entry.AccessModes = pvhAccessModes(pv.Spec.AccessModes)
		}
		if cap, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			entry.Capacity = cap.String()
		}
		entry.ReclaimPolicy = string(pv.Spec.PersistentVolumeReclaimPolicy)

		if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
			result.Summary.ReclaimDeletePVs++
		} else if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
			result.Summary.ReclaimRetainPVs++
		}

		switch pv.Status.Phase {
		case corev1.VolumeReleased:
			result.Summary.ReleasedPVs++
			entry.RiskLevel = "medium"
			entry.Reason = "Released — PV retained but not reclaimed, wasting storage"
			result.ReleasedPVs = append(result.ReleasedPVs, entry)
			result.Issues = append(result.Issues, PVHIssue{
				Severity: "info", Type: "released-pv",
				Resource: pv.Name,
				Message:  fmt.Sprintf("PV %s is Released — manually reclaim or delete to free storage", pv.Name),
			})
		case corev1.VolumeFailed:
			result.Summary.FailedPVs++
			entry.RiskLevel = "critical"
			entry.Reason = "Failed — storage backend error"
			result.Issues = append(result.Issues, PVHIssue{
				Severity: "critical", Type: "failed-pv",
				Resource: pv.Name,
				Message:  fmt.Sprintf("PV %s is Failed — storage backend error, data at risk", pv.Name),
			})
		case corev1.VolumeAvailable:
			if !pvcBindMap[pv.Name] {
				entry.RiskLevel = "low"
				entry.Reason = "Available — not bound to any PVC"
			} else {
				entry.RiskLevel = "low"
			}
		default:
			entry.RiskLevel = "low"
		}

		result.PVs = append(result.PVs, entry)
	}

	// Analyze StorageClasses
	for _, sc := range scs.Items {
		result.Summary.TotalStorageClasses++

		entry := PVHSCEntry{
			Name:              sc.Name,
			Provisioner:       sc.Provisioner,
			ReclaimPolicy:     string(*sc.ReclaimPolicy),
			VolumeBindingMode: string(*sc.VolumeBindingMode),
			AllowExpansion:    sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion,
			PVCCount:          scPVCCount[sc.Name],
		}
		if sc.Name == defaultSC {
			entry.IsDefault = true
			result.Summary.DefaultSCCount++
		}
		if !entry.AllowExpansion {
			result.Summary.NoExpandingSC++
		}

		result.StorageClasses = append(result.StorageClasses, entry)

		if !entry.AllowExpansion && entry.PVCCount > 0 {
			result.Issues = append(result.Issues, PVHIssue{
				Severity: "info", Type: "no-expansion",
				Resource: sc.Name,
				Message:  fmt.Sprintf("StorageClass %s does not allow volume expansion — PVCs cannot grow dynamically", sc.Name),
			})
		}
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.PendingPVCs, func(i, j int) bool {
		return result.PendingPVCs[i].Namespace < result.PendingPVCs[j].Namespace
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PVCCount > result.ByNamespace[j].PVCCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return pvhIssueRank(result.Issues[i].Severity) < pvhIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = pvhScore(result.Summary)
	result.Recommendations = pvhGenRecs(result.Summary, result.PendingPVCs, result.ReleasedPVs)

	writeJSON(w, result)
}

// isDefaultSC checks if a StorageClass is the default.
func isDefaultSC(sc storagev1.StorageClass) bool {
	if sc.Annotations == nil {
		return false
	}
	return sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
		sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true"
}

// pvhAccessModes joins access modes for display.
func pvhAccessModes(modes []corev1.PersistentVolumeAccessMode) string {
	var parts []string
	for _, m := range modes {
		parts = append(parts, string(m))
	}
	return strings.Join(parts, ", ")
}

// pvhScore computes 0-100.
func pvhScore(s PVHSummary) int {
	if s.TotalPVCs == 0 {
		return 100
	}
	score := 100
	score -= s.PendingPVCs * 8
	score -= s.LostPVCs * 20
	score -= s.FailedPVs * 15
	score -= s.ReleasedPVs * 3
	if score < 0 {
		score = 0
	}
	return score
}

// pvhGenRecs produces actionable advice.
func pvhGenRecs(s PVHSummary, pending []PVHEntry, released []PVHEntry) []string {
	var recs []string

	if s.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) are Pending — check StorageClass provisioner, zone availability, and storage capacity", s.PendingPVCs))
	}
	if s.LostPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) bound to Lost/Failed PVs — data may be permanently lost, check storage backend", s.LostPVCs))
	}
	if s.FailedPVs > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) are in Failed state — storage backend errors, investigate immediately", s.FailedPVs))
	}
	if s.ReleasedPVs > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) are Released but not reclaimed — manually delete or reclaim to free storage capacity", s.ReleasedPVs))
	}
	if s.NoExpandingSC > 0 {
		recs = append(recs, fmt.Sprintf("%d StorageClass(es) don't allow volume expansion — PVCs cannot grow, enable allowVolumeExpansion", s.NoExpandingSC))
	}
	if s.DefaultSCCount == 0 {
		recs = append(recs, "No default StorageClass — PVCs without explicit storageClassName will fail to provision")
	}
	if s.ReclaimRetainPVs > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) use Reclaim Retain — deleted PVCs leave orphaned PVs, consider Reclaim Delete for dynamic provisioning", s.ReclaimRetainPVs))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Storage health score is %d/100 — multiple PVC/PV issues detected", s.HealthScore))
	}
	if s.PendingPVCs == 0 && s.LostPVCs == 0 && s.FailedPVs == 0 {
		recs = append(recs, "All PVCs are healthy and properly bound — good storage posture")
	}

	return recs
}

func pvhGetOrCreateNS(m map[string]*PVHNSEntry, ns string) *PVHNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &PVHNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func pvhIssueRank(s string) int {
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
