package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageOrphanResult analyzes orphaned PVCs, unused PVs, storage class waste.
type StorageOrphanResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         StorageOrphanSummary `json:"summary"`
	OrphanPVCs      []OrphanPVCInfo      `json:"orphanPVCs"`
	WasteEstimate   float64              `json:"wasteEstimate"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type StorageOrphanSummary struct {
	TotalPVCs      int     `json:"totalPVCs"`
	BoundPVCs      int     `json:"boundPVCs"`
	PendingPVCs    int     `json:"pendingPVCs"`
	OrphanedPVCs   int     `json:"orphanedPVCs"`
	TotalPVCGB     float64 `json:"totalPVCGB"`
	OrphanGB       float64 `json:"orphanGB"`
	StorageClasses int     `json:"storageClasses"`
	Snapshots      int     `json:"snapshots"`
}

type OrphanPVCInfo struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	SizeGB    float64 `json:"sizeGB"`
	Status    string  `json:"status"`
	Age       string  `json:"age"`
	Severity  string  `json:"severity"`
}

// handleStorageOrphan analyzes orphaned PVCs and storage waste.
// GET /api/scalability/storage-orphan
func (s *Server) handleStorageOrphan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := StorageOrphanResult{ScannedAt: time.Now()}
	now := time.Now()

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	result.Summary.StorageClasses = len(scs.Items)

	// Build PVC usage map from pods
	usedPVCs := map[string]bool{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				usedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}

	for _, pvc := range pvcs.Items {
		result.Summary.TotalPVCs++

		// Get size
		sizeGB := 0.0
		if q, ok := pvc.Spec.Resources.Requests["storage"]; ok {
			sizeGB = float64(q.Value()) / (1024 * 1024 * 1024)
		}
		result.Summary.TotalPVCGB += sizeGB

		switch pvc.Status.Phase {
		case "Bound":
			result.Summary.BoundPVCs++
			key := pvc.Namespace + "/" + pvc.Name
			if !usedPVCs[key] {
				result.Summary.OrphanedPVCs++
				result.Summary.OrphanGB += sizeGB
				ageDays := int(now.Sub(pvc.CreationTimestamp.Time).Hours() / 24)
				severity := "medium"
				if ageDays > 90 {
					severity = "high"
				}
				result.OrphanPVCs = append(result.OrphanPVCs, OrphanPVCInfo{
					Name: pvc.Name, Namespace: pvc.Namespace, SizeGB: sizeGB,
					Status: "orphaned", Age: fmt.Sprintf("%dd", ageDays),
					Severity: severity,
				})
			}
		case "Pending":
			result.Summary.PendingPVCs++
			result.OrphanPVCs = append(result.OrphanPVCs, OrphanPVCInfo{
				Name: pvc.Name, Namespace: pvc.Namespace, SizeGB: sizeGB,
				Status: "pending", Age: "n/a", Severity: "high",
			})
		}
	}

	// Waste cost estimate: $0.10/GB/month
	result.WasteEstimate = result.Summary.OrphanGB * 0.10

	// Score
	score := 100
	orphanRatio := 0.0
	if result.Summary.TotalPVCs > 0 {
		orphanRatio = float64(result.Summary.OrphanedPVCs) / float64(result.Summary.TotalPVCs)
	}
	score -= int(orphanRatio * 60)
	score -= result.Summary.PendingPVCs * 5
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.OrphanPVCs, func(i, j int) bool {
		return result.OrphanPVCs[i].Severity > result.OrphanPVCs[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Storage orphan: %d/100 (grade %s) — %d PVCs, %d orphaned (%.1f GB), $%.2f/mo waste", result.HealthScore, result.Grade, result.Summary.TotalPVCs, result.Summary.OrphanedPVCs, result.Summary.OrphanGB, result.WasteEstimate))
	if result.Summary.OrphanedPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned PVCs — delete or reattach to workloads", result.Summary.OrphanedPVCs))
	}
	if result.Summary.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d pending PVCs — check storage class provisioning", result.Summary.PendingPVCs))
	}
	if len(recs) == 1 {
		recs = append(recs, "Storage utilization is efficient — no orphaned PVCs")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}

func init() { _ = strings.ToLower }
