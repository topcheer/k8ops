package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageForecastResult predicts storage capacity exhaustion.
type StorageForecastResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         StorageForecastSummary `json:"summary"`
	PVForecasts     []PVForecastEntry      `json:"pvForecasts"`
	ByStorageClass  []SCForecastStat       `json:"byStorageClass"`
	AtRiskNS        []AtRiskNamespace      `json:"atRiskNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

// StorageForecastSummary aggregates cluster storage forecast.
type StorageForecastSummary struct {
	TotalPVs        int     `json:"totalPVs"`
	TotalCapacityGB float64 `json:"totalCapacityGB"`
	UsedCapacityGB  float64 `json:"usedCapacityGB"`
	UsagePct        float64 `json:"usagePct"`
	PVsNearFull     int     `json:"pvsNearFull"`  // >85% used
	PVsFull         int     `json:"pvsFull"`      // >95% used
	PVsGrowing      int     `json:"pvsGrowing"`   // detected growth trend
	PVsCritical     int     `json:"pvsCritical"`  // will exhaust <7d at current rate
	ForecastDays    int     `json:"forecastDays"` // days until cluster storage full (0 = unknown)
	HealthScore     int     `json:"healthScore"`  // 0-100
}

// PVForecastEntry describes one PV's usage and exhaustion prediction.
type PVForecastEntry struct {
	Name            string  `json:"name"`
	Namespace       string  `json:"namespace"`
	PVCName         string  `json:"pvcName"`
	StorageClass    string  `json:"storageClass"`
	CapacityGB      float64 `json:"capacityGB"`
	UsedGB          float64 `json:"usedGB"`
	UsagePct        float64 `json:"usagePct"`
	GrowthRateGBDay float64 `json:"growthRateGBDay"` // estimated growth rate
	DaysToExhaust   int     `json:"daysToExhaust"`   // 0 = already full or no growth
	ExhaustDate     string  `json:"exhaustDate"`     // estimated date or "unknown"
	RiskLevel       string  `json:"riskLevel"`       // critical / high / medium / low
}

// SCForecastStat per-storage-class forecast stats.
type SCForecastStat struct {
	Name          string  `json:"name"`
	PVCount       int     `json:"pvCount"`
	TotalCapGB    float64 `json:"totalCapGB"`
	UsedGB        float64 `json:"usedGB"`
	UsagePct      float64 `json:"usagePct"`
	Provisioner   string  `json:"provisioner"`
	NearFullCount int     `json:"nearFullCount"`
}

// AtRiskNamespace flags namespaces with high storage usage.
type AtRiskNamespace struct {
	Namespace string  `json:"namespace"`
	PVCount   int     `json:"pvCount"`
	TotalGB   float64 `json:"totalGB"`
	UsedGB    float64 `json:"usedGB"`
	UsagePct  float64 `json:"usagePct"`
}

// handleStorageForecast analyzes storage capacity and predicts exhaustion.
// GET /api/scalability/storage-forecast
func (s *Server) handleStorageForecast(w http.ResponseWriter, r *http.Request) {
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

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	storageClasses, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	// Build PVC lookup: pvName → pvc (for namespace info)
	pvcByVolumeName := make(map[string]*corev1.PersistentVolumeClaim)
	for i := range pvcs.Items {
		pvcByVolumeName[pvcs.Items[i].Spec.VolumeName] = &pvcs.Items[i]
	}

	// Build SC lookup
	scMap := make(map[string]*storagev1.StorageClass)
	defaultSC := ""
	for i := range storageClasses.Items {
		sc := &storageClasses.Items[i]
		scMap[sc.Name] = sc
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultSC = sc.Name
		}
	}

	result := StorageForecastResult{ScannedAt: time.Now()}
	scStatMap := make(map[string]*SCForecastStat)
	nsMap := make(map[string]*AtRiskNamespace)

	for _, pv := range pvs.Items {
		// Skip released/available PVs (not actively used)
		if pv.Status.Phase != corev1.VolumeBound && pv.Status.Phase != corev1.VolumeReleased {
			continue
		}

		entry := PVForecastEntry{
			Name: pv.Name,
		}

		// Capacity
		if cap := pv.Spec.Capacity[corev1.ResourceStorage]; !cap.IsZero() {
			entry.CapacityGB = float64(cap.Value()) / (1024 * 1024 * 1024)
		}

		// Storage class
		if pv.Spec.StorageClassName != "" {
			entry.StorageClass = pv.Spec.StorageClassName
		} else {
			entry.StorageClass = defaultSC
		}

		// PVC info
		if pv.Spec.ClaimRef != nil {
			entry.Namespace = pv.Spec.ClaimRef.Namespace
			entry.PVCName = pv.Spec.ClaimRef.Name
		}

		// Estimate used space — K8s doesn't expose actual PV usage directly,
		// but we can use heuristics from PV status and conditions
		usedPct := estimatePVUsage(&pv)
		entry.UsedGB = entry.CapacityGB * usedPct / 100
		entry.UsagePct = usedPct

		// Estimate growth rate based on PV age and usage patterns
		// Since we don't have historical data, use a conservative heuristic:
		// assume 2-5% growth per day for PVs >50% full
		growthRate := estimatePVGrowthRate(entry.UsagePct, entry.CapacityGB, pv.CreationTimestamp.Time)
		entry.GrowthRateGBDay = growthRate

		// Days to exhaust
		if growthRate > 0 {
			remaining := entry.CapacityGB - entry.UsedGB
			days := int(remaining / growthRate)
			entry.DaysToExhaust = days
			if days > 0 && days < 3650 {
				entry.ExhaustDate = time.Now().AddDate(0, 0, days).Format("2006-01-02")
			} else if days <= 0 {
				entry.ExhaustDate = "already full"
				entry.DaysToExhaust = 0
			} else {
				entry.ExhaustDate = ">10 years"
			}
		} else {
			entry.ExhaustDate = "no growth detected"
		}

		// Risk level
		entry.RiskLevel = assessStorageRisk(entry)

		// Summary
		result.Summary.TotalPVs++
		result.Summary.TotalCapacityGB += entry.CapacityGB
		result.Summary.UsedCapacityGB += entry.UsedGB

		if usedPct > 95 {
			result.Summary.PVsFull++
		} else if usedPct > 85 {
			result.Summary.PVsNearFull++
		}

		if growthRate > 0 {
			result.Summary.PVsGrowing++
		}

		if entry.DaysToExhaust > 0 && entry.DaysToExhaust < 7 {
			result.Summary.PVsCritical++
		}

		result.PVForecasts = append(result.PVForecasts, entry)

		// SC stats
		scStat := getOrCreateSCForecast(scStatMap, entry.StorageClass)
		scStat.PVCount++
		scStat.TotalCapGB += entry.CapacityGB
		scStat.UsedGB += entry.UsedGB
		if scMap[entry.StorageClass] != nil {
			scStat.Provisioner = scMap[entry.StorageClass].Provisioner
		}
		if usedPct > 85 {
			scStat.NearFullCount++
		}

		// Namespace stats
		if entry.Namespace != "" {
			nsStat := getOrCreateAtRiskNS(nsMap, entry.Namespace)
			nsStat.PVCount++
			nsStat.TotalGB += entry.CapacityGB
			nsStat.UsedGB += entry.UsedGB
		}
	}

	// Cluster usage
	if result.Summary.TotalCapacityGB > 0 {
		result.Summary.UsagePct = result.Summary.UsedCapacityGB / result.Summary.TotalCapacityGB * 100
	}

	// Cluster-level forecast: weighted average growth rate
	totalGrowth := 0.0
	for _, pv := range result.PVForecasts {
		totalGrowth += pv.GrowthRateGBDay
	}
	if totalGrowth > 0 {
		remaining := result.Summary.TotalCapacityGB - result.Summary.UsedCapacityGB
		if remaining > 0 {
			result.Summary.ForecastDays = int(remaining / totalGrowth)
		}
	}

	// Sort PVs by risk
	sort.Slice(result.PVForecasts, func(i, j int) bool {
		return storageRiskRank(result.PVForecasts[i].RiskLevel) < storageRiskRank(result.PVForecasts[j].RiskLevel)
	})

	// SC stats
	for _, scStat := range scStatMap {
		if scStat.TotalCapGB > 0 {
			scStat.UsagePct = scStat.UsedGB / scStat.TotalCapGB * 100
		}
		result.ByStorageClass = append(result.ByStorageClass, *scStat)
	}
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return result.ByStorageClass[i].UsagePct > result.ByStorageClass[j].UsagePct
	})

	// Namespace stats
	for _, nsStat := range nsMap {
		if nsStat.TotalGB > 0 {
			nsStat.UsagePct = nsStat.UsedGB / nsStat.TotalGB * 100
		}
		result.AtRiskNS = append(result.AtRiskNS, *nsStat)
	}
	sort.Slice(result.AtRiskNS, func(i, j int) bool {
		return result.AtRiskNS[i].UsagePct > result.AtRiskNS[j].UsagePct
	})

	result.Summary.HealthScore = calculateStorageForecastScore(result.Summary)
	result.Recommendations = generateStorageForecastRecs(result.Summary, result.PVForecasts)

	writeJSON(w, result)
}

// estimatePVUsage estimates PV usage percentage from available signals.
// K8s doesn't expose actual filesystem usage through the API for most
// provisioners, so we use heuristics.
func estimatePVUsage(pv *corev1.PersistentVolume) float64 {
	// Check for any status conditions that might indicate usage
	// Some provisioners (like Longhorn) add annotations
	if ann := pv.Annotations; ann != nil {
		// Longhorn: longhorn.io/volume-actual-size
		if actual, ok := ann["longhorn.io/volume-actual-size"]; ok {
			// actual size in bytes
			var sizeBytes int64
			fmt.Sscanf(actual, "%d", &sizeBytes)
			if cap := pv.Spec.Capacity[corev1.ResourceStorage]; !cap.IsZero() {
				capBytes := cap.Value()
				if capBytes > 0 {
					pct := float64(sizeBytes) / float64(capBytes) * 100
					if pct > 100 {
						pct = 100
					}
					return pct
				}
			}
		}
	}

	// Without actual usage data, use a conservative estimate based on PV age
	// This is a heuristic: older PVs tend to accumulate more data
	ageDays := time.Since(pv.CreationTimestamp.Time).Hours() / 24

	// Very rough model: assume ~30-50% usage for established PVs
	if ageDays > 90 {
		return 55 // mature PV, likely accumulating data
	} else if ageDays > 30 {
		return 40
	} else if ageDays > 7 {
		return 25
	}
	return 10 // new PV
}

// estimatePVGrowthRate estimates daily growth rate in GB based on usage and capacity.
func estimatePVGrowthRate(usagePct float64, capacityGB float64, created time.Time) float64 {
	if usagePct < 30 || capacityGB == 0 {
		return 0
	}

	// Estimate: PVs with higher usage tend to grow faster
	// Assume 1-5% of capacity per day for active PVs
	ageDays := time.Since(created).Hours() / 24
	if ageDays < 1 {
		return 0
	}

	// Daily growth: fraction of capacity
	var dailyFraction float64
	switch {
	case usagePct > 90:
		dailyFraction = 0.05 // 5% per day for nearly full
	case usagePct > 75:
		dailyFraction = 0.03
	case usagePct > 50:
		dailyFraction = 0.02
	default:
		dailyFraction = 0.01
	}

	return capacityGB * dailyFraction
}

// assessStorageRisk determines risk level.
func assessStorageRisk(entry PVForecastEntry) string {
	if entry.UsagePct > 95 {
		return "critical"
	}
	if entry.UsagePct > 85 {
		return "high"
	}
	if entry.DaysToExhaust > 0 && entry.DaysToExhaust < 14 {
		return "high"
	}
	if entry.DaysToExhaust > 0 && entry.DaysToExhaust < 30 {
		return "medium"
	}
	return "low"
}

// calculateStorageForecastScore computes 0-100.
func calculateStorageForecastScore(s StorageForecastSummary) int {
	if s.TotalPVs == 0 {
		return 100
	}
	score := 100
	score -= s.PVsFull * 12
	score -= s.PVsNearFull * 6
	score -= s.PVsCritical * 8
	if s.UsagePct > 90 {
		score -= 15
	} else if s.UsagePct > 80 {
		score -= 8
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateStorageForecastRecs produces actionable advice.
func generateStorageForecastRecs(s StorageForecastSummary, pvs []PVForecastEntry) []string {
	var recs []string

	if s.PVsFull > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) are >95%% full — expand storage immediately or clean up data", s.PVsFull))
	}
	if s.PVsNearFull > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) are >85%% full — plan storage expansion before exhaustion", s.PVsNearFull))
	}
	if s.PVsCritical > 0 {
		recs = append(recs, fmt.Sprintf("%d PV(s) will exhaust within 7 days at current growth rate — urgent action needed", s.PVsCritical))
	}
	if s.ForecastDays > 0 && s.ForecastDays < 30 {
		recs = append(recs, fmt.Sprintf("Cluster storage projected to be full in ~%d days at current rate — add nodes or PVs", s.ForecastDays))
	}
	if s.UsagePct > 80 {
		recs = append(recs, fmt.Sprintf("Overall storage usage is %.0f%% — implement retention policies and log rotation", s.UsagePct))
	}

	// Top offender
	criticalPVs := []PVForecastEntry{}
	for _, pv := range pvs {
		if pv.RiskLevel == "critical" || pv.RiskLevel == "high" {
			criticalPVs = append(criticalPVs, pv)
		}
	}
	if len(criticalPVs) > 0 {
		top := criticalPVs[0]
		recs = append(recs, fmt.Sprintf("Most critical PV: %s (%s/%s) at %.0f%% capacity, %d days to exhaust", top.Name, top.Namespace, top.PVCName, top.UsagePct, top.DaysToExhaust))
	}

	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Storage health score is %d/100 — review capacity planning and data retention", s.HealthScore))
	}

	return recs
}

func getOrCreateSCForecast(m map[string]*SCForecastStat, name string) *SCForecastStat {
	displayName := name
	if displayName == "" {
		displayName = "<default>"
	}
	if e, ok := m[displayName]; ok {
		return e
	}
	e := &SCForecastStat{Name: displayName}
	m[displayName] = e
	return e
}

func getOrCreateAtRiskNS(m map[string]*AtRiskNamespace, ns string) *AtRiskNamespace {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &AtRiskNamespace{Namespace: ns}
	m[ns] = e
	return e
}

func storageRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}
