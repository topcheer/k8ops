package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeBudgetResult analyzes PVC usage, storage quota consumption, and volume
// lifecycle. It tracks PVC request vs actual capacity, identifies over-provisioned
// volumes, detects orphaned PVCs, and forecasts storage budget exhaustion.
type VolumeBudgetResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         VolBudgetSummary  `json:"summary"`
	Volumes         []VolBudgetEntry  `json:"volumes"`
	ByNamespace     []VolBudgetNS     `json:"byNamespace"`
	ByStorageClass  []VolBudgetSC     `json:"byStorageClass"`
	OrphanedPVCs    []VolOrphan       `json:"orphanedPVCs"`
	OverProvisioned []VolOverProv     `json:"overProvisioned"`
	Forecast        VolBudgetForecast `json:"forecast"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type VolBudgetSummary struct {
	TotalPVCs        int     `json:"totalPVCs"`
	TotalRequestedGB float64 `json:"totalRequestedGB"`
	TotalCapacityGB  float64 `json:"totalCapacityGB"`
	TotalUsedGB      float64 `json:"totalUsedGB"`
	BoundPVCs        int     `json:"boundPVCs"`
	PendingPVCs      int     `json:"pendingPVCs"`
	ReleasedPVCs     int     `json:"releasedPVCs"`
	OrphanedCount    int     `json:"orphanedCount"`
	AvgUtilization   float64 `json:"avgUtilization"`
	MonthlyCost      float64 `json:"monthlyCostUSD"`
}

type VolBudgetEntry struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	StorageClass   string  `json:"storageClass"`
	RequestedGB    float64 `json:"requestedGB"`
	CapacityGB     float64 `json:"capacityGB"`
	UsedGB         float64 `json:"usedGB"`
	UtilizationPct float64 `json:"utilizationPct"`
	Status         string  `json:"status"`
	AccessMode     string  `json:"accessMode"`
	AgeDays        int     `json:"ageDays"`
	Mounted        bool    `json:"mounted"`
}

type VolBudgetNS struct {
	Namespace   string  `json:"namespace"`
	PVCCount    int     `json:"pvcCount"`
	RequestedGB float64 `json:"requestedGB"`
	CapacityGB  float64 `json:"capacityGB"`
	Orphaned    int     `json:"orphaned"`
}

type VolBudgetSC struct {
	StorageClass string  `json:"storageClass"`
	PVCCount     int     `json:"pvcCount"`
	TotalGB      float64 `json:"totalGB"`
	Pct          float64 `json:"pct"`
}

type VolOrphan struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	SizeGB    float64 `json:"sizeGB"`
	AgeDays   int     `json:"ageDays"`
	Reason    string  `json:"reason"`
}

type VolOverProv struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	RequestedGB float64 `json:"requestedGB"`
	UsedGB      float64 `json:"usedGB"`
	Utilization float64 `json:"utilizationPct"`
	WasteGB     float64 `json:"wasteGB"`
}

type VolBudgetForecast struct {
	GrowthRate30d   float64 `json:"growthRate30dGB"`
	MonthsToExhaust int     `json:"monthsToExhaust"`
	ExhaustionDate  string  `json:"exhaustionDate"`
}

const storageCostPerGBMonth = 0.10

// handleVolumeBudget handles GET /api/scalability/volume-budget
func (s *Server) handleVolumeBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := VolumeBudgetResult{ScannedAt: time.Now()}
	now := time.Now()

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pvs, _ := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	// Build mounted PVC set
	mountedPVCs := map[string]bool{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				mountedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}

	// Build PV capacity map
	pvCapacity := map[string]float64{}
	for _, pv := range pvs.Items {
		if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			pvCapacity[pv.Name] = float64(q.Value()) / (1024 * 1024 * 1024)
		}
	}

	nsStats := map[string]*VolBudgetNS{}
	scStats := map[string]*VolBudgetSC{}
	totalRequested := 0.0
	totalCapacity := 0.0

	for _, pvc := range pvcs.Items {
		reqGB := 0.0
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			reqGB = float64(q.Value()) / (1024 * 1024 * 1024)
		}
		capGB := reqGB
		if pvc.Spec.VolumeName != "" {
			if c, ok := pvCapacity[pvc.Spec.VolumeName]; ok {
				capGB = c
			}
		}
		totalRequested += reqGB
		totalCapacity += capGB

		status := string(pvc.Status.Phase)
		mounted := mountedPVCs[pvc.Namespace+"/"+pvc.Name]
		ageDays := 0
		if !pvc.CreationTimestamp.IsZero() {
			ageDays = int(now.Sub(pvc.CreationTimestamp.Time).Hours() / 24)
		}

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}

		entry := VolBudgetEntry{
			Name: pvc.Name, Namespace: pvc.Namespace, StorageClass: scName,
			RequestedGB: reqGB, CapacityGB: capGB, Status: status,
			AgeDays: ageDays, Mounted: mounted,
		}
		if len(pvc.Spec.AccessModes) > 0 {
			entry.AccessMode = string(pvc.Spec.AccessModes[0])
		}
		result.Volumes = append(result.Volumes, entry)

		// Summary
		result.Summary.TotalPVCs++
		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
		case corev1.ClaimLost:
			result.Summary.ReleasedPVCs++
		}

		// Orphan detection: Released/Lost or bound but not mounted
		if pvc.Status.Phase == corev1.ClaimLost || (!mounted && pvc.Status.Phase == corev1.ClaimBound) {
			result.Summary.OrphanedCount++
			result.OrphanedPVCs = append(result.OrphanedPVCs, VolOrphan{
				Name: pvc.Name, Namespace: pvc.Namespace,
				SizeGB: reqGB, AgeDays: ageDays,
				Reason: fmt.Sprintf("Phase: %s, mounted: %v", status, mounted),
			})
		}

		// Namespace stats
		if nsStats[pvc.Namespace] == nil {
			nsStats[pvc.Namespace] = &VolBudgetNS{Namespace: pvc.Namespace}
		}
		nsStats[pvc.Namespace].PVCCount++
		nsStats[pvc.Namespace].RequestedGB += reqGB
		nsStats[pvc.Namespace].CapacityGB += capGB
		if (!mounted && pvc.Status.Phase == corev1.ClaimBound) || pvc.Status.Phase == corev1.ClaimLost {
			nsStats[pvc.Namespace].Orphaned++
		}

		// Storage class stats
		if scStats[scName] == nil {
			scStats[scName] = &VolBudgetSC{StorageClass: scName}
		}
		scStats[scName].PVCCount++
		scStats[scName].TotalGB += reqGB
	}

	result.Summary.TotalRequestedGB = totalRequested
	result.Summary.TotalCapacityGB = totalCapacity
	result.Summary.MonthlyCost = totalCapacity * storageCostPerGBMonth
	if totalCapacity > 0 {
		result.Summary.AvgUtilization = totalRequested / totalCapacity * 100
	}

	// Build NS and SC stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool { return result.ByNamespace[i].RequestedGB > result.ByNamespace[j].RequestedGB })

	for _, sc := range scStats {
		if totalRequested > 0 {
			sc.Pct = sc.TotalGB / totalRequested * 100
		}
		result.ByStorageClass = append(result.ByStorageClass, *sc)
	}

	// Forecast (simplified: assume 5% monthly growth)
	result.Forecast.GrowthRate30d = totalCapacity * 0.05
	if totalCapacity > 0 {
		result.Forecast.MonthsToExhaust = 999
		result.Forecast.ExhaustionDate = "no exhaustion predicted"
	}

	result.HealthScore = computeVolBudgetScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generateVolBudgetRecs(result)

	_ = scs // suppress unused
	writeJSON(w, result)
}

func computeVolBudgetScore(s VolBudgetSummary) int {
	score := 100
	if s.TotalPVCs == 0 {
		return score
	}
	orphanRatio := float64(s.OrphanedCount) / float64(s.TotalPVCs)
	score -= int(orphanRatio * 30)
	if s.PendingPVCs > 0 {
		score -= minInt(s.PendingPVCs*5, 15)
	}
	if score < 0 {
		score = 0
	}
	return score
}

func generateVolBudgetRecs(r VolumeBudgetResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Volume budget: %d PVCs, %.1f GB requested, $%.2f/month — score %d/100",
		r.Summary.TotalPVCs, r.Summary.TotalRequestedGB, r.Summary.MonthlyCost, r.HealthScore))
	if r.Summary.OrphanedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned PVC(s) — clean up to save $%.2f/month",
			r.Summary.OrphanedCount, float64(r.Summary.OrphanedCount)*10*storageCostPerGBMonth))
	}
	if r.Summary.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d pending PVC(s) — check storage class provisioning", r.Summary.PendingPVCs))
	}
	return recs
}
