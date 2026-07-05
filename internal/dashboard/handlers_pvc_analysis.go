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

// PVCAnalysisResult is the full PVC binding and storage performance analysis.
type PVCAnalysisResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         PVCSummary         `json:"summary"`
	PVCs            []PVCEntry         `json:"pvcs"`
	ByStorageClass  []StorageClassStat `json:"byStorageClass"`
	StuckPVCs       []StuckPVCEntry    `json:"stuckPVCs"`
	Issues          []PVCIssue         `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

// PVCSummary aggregates cluster-wide PVC metrics.
type PVCSummary struct {
	TotalPVCs        int     `json:"totalPVCs"`
	BoundPVCs        int     `json:"boundPVCs"`
	PendingPVCs      int     `json:"pendingPVCs"`
	StuckPVCs        int     `json:"stuckPVCs"` // pending for >5min
	TotalSizeGB      float64 `json:"totalSizeGB"`
	BoundSizeGB      float64 `json:"boundSizeGB"`
	StorageClasses   int     `json:"storageClasses"`
	DefaultSC        string  `json:"defaultSC"`
	SlowBindingCount int     `json:"slowBindingCount"` // bind time >30s
	AvgBindTimeMs    int64   `json:"avgBindTimeMs"`
	HealthScore      int     `json:"healthScore"` // 0-100
}

// PVCEntry describes one PVC.
type PVCEntry struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	StorageClass string  `json:"storageClass"`
	SizeGB       float64 `json:"sizeGB"`
	AccessModes  string  `json:"accessModes"`
	Phase        string  `json:"phase"` // Bound / Pending / Lost
	BoundPV      string  `json:"boundPV"`
	BindTimeMs   int64   `json:"bindTimeMs"` // time from creation to bound
	Age          string  `json:"age"`
	Status       string  `json:"status"` // healthy / warning / critical
}

// StorageClassStat aggregates per-storage-class statistics.
type StorageClassStat struct {
	Name        string  `json:"name"`
	IsDefault   bool    `json:"isDefault"`
	PVCCount    int     `json:"pvcCount"`
	TotalSizeGB float64 `json:"totalSizeGB"`
	Provisioner string  `json:"provisioner"`
	BindingMode string  `json:"bindingMode"`
	AvgBindMs   int64   `json:"avgBindMs"`
}

// StuckPVCEntry is a PVC that's been pending too long.
type StuckPVCEntry struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	StorageClass string  `json:"storageClass"`
	SizeGB       float64 `json:"sizeGB"`
	PendingFor   string  `json:"pendingFor"`
	Reason       string  `json:"reason"`
}

// PVCIssue is a detected problem.
type PVCIssue struct {
	PVC       string `json:"pvc"`
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

// handlePVCAnalysis analyzes PVC binding health and storage performance.
// GET /api/scalability/pvc-analysis?namespace=xxx
func (s *Server) handlePVCAnalysis(w http.ResponseWriter, r *http.Request) {
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

	storageClasses, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	pvs, _ := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})

	// Build storage class lookup
	scMap := make(map[string]*storagev1.StorageClass)
	defaultSC := ""
	for i := range storageClasses.Items {
		sc := &storageClasses.Items[i]
		scMap[sc.Name] = sc
		// Check for default annotation
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			defaultSC = sc.Name
		}
	}

	// Build PV lookup for bind time calculation
	pvMap := make(map[string]*corev1.PersistentVolume)
	for i := range pvs.Items {
		pvMap[pvs.Items[i].Name] = &pvs.Items[i]
	}

	result := PVCAnalysisResult{ScannedAt: time.Now()}
	scStatMap := make(map[string]*StorageClassStat)

	var totalBindMs int64
	bindCount := 0

	for _, pvc := range pvcs.Items {
		entry := PVCEntry{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Phase:     string(pvc.Status.Phase),
			Age:       time.Since(pvc.CreationTimestamp.Time).Round(time.Second).String(),
		}

		// Storage class
		if pvc.Spec.StorageClassName != nil {
			entry.StorageClass = *pvc.Spec.StorageClassName
		} else {
			entry.StorageClass = defaultSC
		}

		// Size
		if r := pvc.Spec.Resources.Requests.Storage(); r != nil {
			entry.SizeGB = float64(r.Value()) / (1024 * 1024 * 1024)
		}

		// Access modes
		var modes []string
		for _, am := range pvc.Spec.AccessModes {
			modes = append(modes, string(am))
		}
		entry.AccessModes = strings.Join(modes, ",")

		// Bind time calculation
		if pvc.Status.Phase == corev1.ClaimBound {
			entry.BoundPV = pvc.Spec.VolumeName
			bindTime := calculatePVCBindTime(&pvc, pvMap)
			entry.BindTimeMs = bindTime.Milliseconds()
			if bindTime > 0 {
				totalBindMs += bindTime.Milliseconds()
				bindCount++
			}
			if bindTime > 30*time.Second {
				result.Summary.SlowBindingCount++
				result.Issues = append(result.Issues, PVCIssue{
					PVC: pvc.Name, Namespace: pvc.Namespace,
					Severity: "warning", Type: "slow-binding",
					Message: fmt.Sprintf("PVC took %s to bind (storage class: %s)", bindTime.Round(time.Second), entry.StorageClass),
				})
			}
		}

		// Status assessment
		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			entry.Status = "healthy"
			result.Summary.BoundPVCs++
			result.Summary.BoundSizeGB += entry.SizeGB
		case corev1.ClaimPending:
			pendingDuration := time.Since(pvc.CreationTimestamp.Time)
			if pendingDuration > 5*time.Minute {
				entry.Status = "critical"
				result.Summary.StuckPVCs++
				stuck := StuckPVCEntry{
					Name: pvc.Name, Namespace: pvc.Namespace,
					StorageClass: entry.StorageClass,
					SizeGB:       entry.SizeGB,
					PendingFor:   pendingDuration.Round(time.Second).String(),
					Reason:       determineStuckReason(&pvc, scMap),
				}
				result.StuckPVCs = append(result.StuckPVCs, stuck)
				result.Issues = append(result.Issues, PVCIssue{
					PVC: pvc.Name, Namespace: pvc.Namespace,
					Severity: "critical", Type: "stuck",
					Message: fmt.Sprintf("PVC pending for %s — %s", pendingDuration.Round(time.Minute), stuck.Reason),
				})
			} else {
				entry.Status = "warning"
			}
			result.Summary.PendingPVCs++
		case corev1.ClaimLost:
			entry.Status = "critical"
			result.Issues = append(result.Issues, PVCIssue{
				PVC: pvc.Name, Namespace: pvc.Namespace,
				Severity: "critical", Type: "lost",
				Message: "PVC is in Lost phase — underlying PV may have been deleted",
			})
		}

		// Storage class stats
		scStat := getOrCreateSCStat(scStatMap, entry.StorageClass)
		scStat.PVCCount++
		scStat.TotalSizeGB += entry.SizeGB
		if scMap[entry.StorageClass] != nil {
			scStat.Provisioner = scMap[entry.StorageClass].Provisioner
			if scMap[entry.StorageClass].VolumeBindingMode != nil {
				scStat.BindingMode = string(*scMap[entry.StorageClass].VolumeBindingMode)
			}
		}
		if entry.StorageClass == defaultSC {
			scStat.IsDefault = true
		}
		if entry.BindTimeMs > 0 {
			scStat.AvgBindMs += entry.BindTimeMs
		}

		result.Summary.TotalPVCs++
		result.Summary.TotalSizeGB += entry.SizeGB
		result.PVCs = append(result.PVCs, entry)
	}

	// Calculate avg bind times
	if bindCount > 0 {
		result.Summary.AvgBindTimeMs = totalBindMs / int64(bindCount)
	}

	// Build storage class stats
	for scName, stat := range scStatMap {
		if stat.PVCCount > 0 && stat.AvgBindMs > 0 {
			stat.AvgBindMs = stat.AvgBindMs / int64(stat.PVCCount)
		}
		result.ByStorageClass = append(result.ByStorageClass, *stat)
		_ = scName
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].PVCCount > result.ByStorageClass[j].PVCCount
	})

	// Sort PVCs by status
	sort.Slice(result.PVCs, func(i, j int) bool {
		return pvcAnalysisStatusRank(result.PVCs[i].Status) < pvcAnalysisStatusRank(result.PVCs[j].Status)
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return pvcAnalysisSeverityRank(result.Issues[i].Severity) < pvcAnalysisSeverityRank(result.Issues[j].Severity)
	})

	result.Summary.StorageClasses = len(storageClasses.Items)
	result.Summary.DefaultSC = defaultSC
	result.Summary.HealthScore = calculatePVCHealthScore(result.Summary)
	result.Recommendations = generatePVCRecommendations(result.Summary, result.ByStorageClass)

	writeJSON(w, result)
}

// calculatePVCBindTime computes the time from PVC creation to PV binding.
func calculatePVCBindTime(pvc *corev1.PersistentVolumeClaim, pvMap map[string]*corev1.PersistentVolume) time.Duration {
	if pvc.Status.Phase != corev1.ClaimBound || pvc.Spec.VolumeName == "" {
		return 0
	}

	pv, ok := pvMap[pvc.Spec.VolumeName]
	if !ok {
		return 0
	}

	// If PV was created after PVC, binding happened around PV creation time
	pvcCreated := pvc.CreationTimestamp.Time
	if pv.Status.Phase == corev1.VolumeBound && pv.CreationTimestamp.Time.After(pvcCreated) {
		return pv.CreationTimestamp.Time.Sub(pvcCreated)
	}

	return 0
}

// determineStuckReason identifies why a PVC is stuck in Pending.
func determineStuckReason(pvc *corev1.PersistentVolumeClaim, scMap map[string]*storagev1.StorageClass) string {
	// Check for missing storage class
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		if _, exists := scMap[*pvc.Spec.StorageClassName]; !exists {
			return fmt.Sprintf("storage class %q does not exist", *pvc.Spec.StorageClassName)
		}
	}

	// Check for waiting conditions in PVC status
	for _, cond := range pvc.Status.Conditions {
		if cond.Status == corev1.ConditionTrue && cond.Reason != "" {
			return fmt.Sprintf("%s: %s", cond.Reason, cond.Message)
		}
	}

	// Check access mode constraints
	if len(pvc.Spec.AccessModes) > 1 {
		for _, am := range pvc.Spec.AccessModes {
			if am == corev1.ReadWriteOncePod {
				return "ReadWriteOncePod access mode may be limiting provisioning"
			}
		}
	}

	// Check volume name is set (manual binding)
	if pvc.Spec.VolumeName != "" {
		return fmt.Sprintf("manually bound to PV %s but PV may be unavailable", pvc.Spec.VolumeName)
	}

	return "no available PV matching size/access mode/storage class — check provisioner and capacity"
}

// calculatePVCHealthScore computes 0-100.
func calculatePVCHealthScore(s PVCSummary) int {
	if s.TotalPVCs == 0 {
		return 100
	}
	score := 100
	score -= s.StuckPVCs * 15
	score -= s.PendingPVCs * 3
	score -= s.SlowBindingCount * 2
	if score < 0 {
		score = 0
	}
	return score
}

// generatePVCRecommendations produces actionable advice.
func generatePVCRecommendations(s PVCSummary, scStats []StorageClassStat) []string {
	var recs []string

	if s.StuckPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) stuck in Pending for >5min — check provisioner, storage class, and capacity", s.StuckPVCs))
	}
	if s.SlowBindingCount > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) took >30s to bind — consider WaitForFirstConsumer or faster storage", s.SlowBindingCount))
	}
	if s.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) currently Pending — verify storage provisioner is running", s.PendingPVCs))
	}
	if s.DefaultSC == "" {
		recs = append(recs, "No default StorageClass set — PVCs without explicit storageClassName will fail")
	}
	if s.TotalSizeGB > 0 {
		boundPct := s.BoundSizeGB / s.TotalSizeGB * 100
		if boundPct < 90 {
			recs = append(recs, fmt.Sprintf("Only %.0f%% of requested storage is bound — check for provisioning failures", boundPct))
		}
	}
	// Per-SC recommendations
	for _, sc := range scStats {
		if sc.BindingMode == "WaitForFirstConsumer" && sc.AvgBindMs > 5000 {
			recs = append(recs, fmt.Sprintf("StorageClass %s (WaitForFirstConsumer) has slow binding (%dms avg) — normal for topology-aware provisioning", sc.Name, sc.AvgBindMs))
		}
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Storage health score is %d/100 — investigate PVC provisioning issues", s.HealthScore))
	}

	return recs
}

func getOrCreateSCStat(m map[string]*StorageClassStat, name string) *StorageClassStat {
	displayName := name
	if name == "" {
		displayName = "<default>"
	}
	if e, ok := m[displayName]; ok {
		return e
	}
	e := &StorageClassStat{Name: displayName}
	m[displayName] = e
	return e
}

func pvcAnalysisStatusRank(status string) int {
	switch status {
	case "critical":
		return 0
	case "warning":
		return 1
	case "healthy":
		return 2
	default:
		return 3
	}
}

func pvcAnalysisSeverityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
