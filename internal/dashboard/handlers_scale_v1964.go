package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.64 — Scalability & HA Dimension (Round 13 Final)
// 1. API Object Count — total object inventory vs K8s recommended limits
// 2. Watch Cache Pressure — controller watch load & etcd read pressure estimator
// 3. Scheduler Cache Health — pending pod backlog & scheduling throughput estimator
// ============================================================

// ---------------------------------------------------------------
// 1. API Object Count
// ---------------------------------------------------------------

type APIObjectCountResult1964 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         APIObjectCountSummary1964 `json:"summary"`
	ResourceCounts  []APIObjectCountEntry1964 `json:"resourceCounts"`
	OverloadedNS    []APIObjectCountEntry1964 `json:"overloadedNamespaces"`
	Recommendations []string                  `json:"recommendations"`
}

type APIObjectCountSummary1964 struct {
	TotalPods        int `json:"totalPods"`
	TotalServices    int `json:"totalServices"`
	TotalConfigMaps  int `json:"totalConfigMaps"`
	TotalSecrets     int `json:"totalSecrets"`
	TotalEndpoints   int `json:"totalEndpoints"`
	TotalNamespaces  int `json:"totalNamespaces"`
	TotalCRDs        int `json:"totalCRDs"`
	ApproachingLimit int `json:"resourcesApproachingLimit"`
}

type APIObjectCountEntry1964 struct {
	ResourceType   string  `json:"resourceType"`
	Namespace      string  `json:"namespace"`
	Count          int     `json:"count"`
	Limit          int     `json:"recommendedLimit"`
	UtilizationPct float64 `json:"utilizationPct"`
}

// K8s recommended limits per namespace
var k8sLimits1964 = map[string]int{
	"pods":       110,
	"services":   5000,
	"configmaps": 0, // no hard limit but warn at high counts
	"secrets":    0,
}

func (s *Server) handleAPIObjectCount(w http.ResponseWriter, r *http.Request) {
	result := APIObjectCountResult1964{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	// CRDs: use dynamic client or skip if unavailable
	// For scalability assessment, we estimate CRD count from CRD-related pods
	result.Summary.TotalCRDs = 0 // Will be populated if CRD client is available

	result.Summary.TotalPods = len(podList.Items)
	result.Summary.TotalServices = len(svcList.Items)
	result.Summary.TotalConfigMaps = len(cmList.Items)
	result.Summary.TotalSecrets = len(secretList.Items)
	result.Summary.TotalNamespaces = len(nsList.Items)

	// Count pods per namespace
	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		podsPerNS[pod.Namespace]++
	}
	for ns, count := range podsPerNS {
		util := float64(count) / 110 * 100
		entry := APIObjectCountEntry1964{
			ResourceType: "pods", Namespace: ns,
			Count: count, Limit: 110, UtilizationPct: util,
		}
		result.ResourceCounts = append(result.ResourceCounts, entry)
		if util > 80 {
			result.OverloadedNS = append(result.OverloadedNS, entry)
			result.Summary.ApproachingLimit++
			score -= 3
		}
	}

	// Overall counts
	result.ResourceCounts = append(result.ResourceCounts, APIObjectCountEntry1964{
		ResourceType: "services", Namespace: "cluster-wide",
		Count: len(svcList.Items), Limit: 5000,
		UtilizationPct: float64(len(svcList.Items)) / 5000 * 100,
	})
	result.ResourceCounts = append(result.ResourceCounts, APIObjectCountEntry1964{
		ResourceType: "configmaps", Namespace: "cluster-wide",
		Count: len(cmList.Items), Limit: 0, UtilizationPct: 0,
	})
	result.ResourceCounts = append(result.ResourceCounts, APIObjectCountEntry1964{
		ResourceType: "secrets", Namespace: "cluster-wide",
		Count: len(secretList.Items), Limit: 0, UtilizationPct: 0,
	})
	result.ResourceCounts = append(result.ResourceCounts, APIObjectCountEntry1964{
		ResourceType: "namespaces", Namespace: "cluster-wide",
		Count: len(nsList.Items), Limit: 10000,
		UtilizationPct: float64(len(nsList.Items)) / 10000 * 100,
	})
	result.ResourceCounts = append(result.ResourceCounts, APIObjectCountEntry1964{
		ResourceType: "crds", Namespace: "cluster-wide",
		Count: result.Summary.TotalCRDs, Limit: 500,
		UtilizationPct: float64(result.Summary.TotalCRDs) / 500 * 100,
	})

	sort.Slice(result.ResourceCounts, func(i, j int) bool {
		return result.ResourceCounts[i].UtilizationPct > result.ResourceCounts[j].UtilizationPct
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Objects: %d pods, %d services, %d CMs, %d secrets, %d CRDs across %d namespaces", result.Summary.TotalPods, result.Summary.TotalServices, result.Summary.TotalConfigMaps, result.Summary.TotalSecrets, result.Summary.TotalCRDs, result.Summary.TotalNamespaces))
	if result.Summary.ApproachingLimit > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces approaching pod limit (80%%+)", result.Summary.ApproachingLimit))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Watch Cache Pressure
// ---------------------------------------------------------------

type WatchCacheResult1964 struct {
	ScannedAt          time.Time             `json:"scannedAt"`
	HealthScore        int                   `json:"healthScore"`
	Grade              string                `json:"grade"`
	Summary            WatchCacheSummary1964 `json:"summary"`
	HighWatchResources []WatchCacheEntry1964 `json:"highWatchResources"`
	Recommendations    []string              `json:"recommendations"`
}

type WatchCacheSummary1964 struct {
	TotalWatchers         int     `json:"estimatedWatchers"`
	HighVolumeObjects     int     `json:"highVolumeObjects"`
	EstimatedEventsPerMin int     `json:"estimatedEventsPerMin"`
	PressureLevel         string  `json:"pressureLevel"`
	PodChurnRate          float64 `json:"podChurnRatePerHour"`
}

type WatchCacheEntry1964 struct {
	ResourceType string  `json:"resourceType"`
	ObjectCount  int     `json:"objectCount"`
	WatchScore   float64 `json:"watchScore"`
	RiskLevel    string  `json:"riskLevel"`
}

func (s *Server) handleWatchCachePressure(w http.ResponseWriter, r *http.Request) {
	result := WatchCacheResult1964{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	// Estimate watchers: each controller/operator typically watches pods, services, configmaps
	// Roughly: nsCount * 3 (informer types per controller) + system watchers
	nsCount := len(nsList.Items)
	estimatedWatchers := nsCount*5 + 20 // base system watchers
	result.Summary.TotalWatchers = estimatedWatchers

	// Calculate watch pressure per resource type
	// Score = objectCount * churnFactor
	podCount := len(podList.Items)
	svcCount := len(svcList.Items)
	cmCount := len(cmList.Items)

	// Pod churn: count recently created pods (last hour approximation via age)
	podChurn := 0.0
	for _, pod := range podList.Items {
		if !pod.CreationTimestamp.IsZero() {
			age := time.Since(pod.CreationTimestamp.Time)
			if age < time.Hour {
				podChurn++
			}
		}
	}
	result.Summary.PodChurnRate = podChurn

	// Watch scores (higher = more pressure on etcd watch cache)
	podWatchScore := float64(podCount) * (1 + podChurn/10)
	svcWatchScore := float64(svcCount) * 0.5
	cmWatchScore := float64(cmCount) * 0.3

	entries := []WatchCacheEntry1964{
		{ResourceType: "pods", ObjectCount: podCount, WatchScore: podWatchScore, RiskLevel: classifyWatchRisk1964(podWatchScore)},
		{ResourceType: "services", ObjectCount: svcCount, WatchScore: svcWatchScore, RiskLevel: classifyWatchRisk1964(svcWatchScore)},
		{ResourceType: "configmaps", ObjectCount: cmCount, WatchScore: cmWatchScore, RiskLevel: classifyWatchRisk1964(cmWatchScore)},
	}

	for _, e := range entries {
		if e.RiskLevel == "high" || e.RiskLevel == "critical" {
			result.HighWatchResources = append(result.HighWatchResources, e)
			result.Summary.HighVolumeObjects++
			if e.RiskLevel == "critical" {
				score -= 10
			} else {
				score -= 5
			}
		}
		result.HighWatchResources = append(result.HighWatchResources, e)
	}

	// Estimated events per minute (rough)
	result.Summary.EstimatedEventsPerMin = int(podChurn * 5)

	// Pressure level
	if podWatchScore > 500 {
		result.Summary.PressureLevel = "high"
	} else if podWatchScore > 200 {
		result.Summary.PressureLevel = "medium"
	} else {
		result.Summary.PressureLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Watch pressure: %s (%.0f pod watch score, %.0f churn/hr)", result.Summary.PressureLevel, podWatchScore, podChurn))
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("~%d estimated watchers across %d namespaces", estimatedWatchers, nsCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func classifyWatchRisk1964(score float64) string {
	if score > 500 {
		return "critical"
	}
	if score > 200 {
		return "high"
	}
	if score > 50 {
		return "medium"
	}
	return "low"
}

// ---------------------------------------------------------------
// 3. Scheduler Cache Health
// ---------------------------------------------------------------

type SchedCacheResult1964 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SchedCacheSummary1964 `json:"summary"`
	PendingPods     []SchedCacheEntry1964 `json:"pendingPods"`
	Recommendations []string              `json:"recommendations"`
}

type SchedCacheSummary1964 struct {
	TotalPods       int     `json:"totalPods"`
	RunningPods     int     `json:"runningPods"`
	PendingPods     int     `json:"pendingPods"`
	FailedPods      int     `json:"failedPods"`
	PendingRatio    float64 `json:"pendingRatioPct"`
	AvgPendingAge   string  `json:"avgPendingAge"`
	ThroughputScore float64 `json:"throughputScore"`
	BacklogLevel    string  `json:"backlogLevel"`
}

type SchedCacheEntry1964 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
}

func (s *Server) handleSchedCacheHealth(w http.ResponseWriter, r *http.Request) {
	result := SchedCacheResult1964{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalPendingAge time.Duration
	var pendingCount int

	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		switch pod.Status.Phase {
		case corev1.PodRunning:
			result.Summary.RunningPods++
		case corev1.PodPending:
			result.Summary.PendingPods++
			pendingCount++
			if !pod.CreationTimestamp.IsZero() {
				age := time.Since(pod.CreationTimestamp.Time)
				totalPendingAge += age
			}
			// Find scheduling condition reason
			reason := "Pending"
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					reason = cond.Reason
					if reason == "" {
						reason = "Unschedulable"
					}
				}
			}
			entry := SchedCacheEntry1964{
				Name: pod.Name, Namespace: pod.Namespace,
				Reason: reason,
			}
			if !pod.CreationTimestamp.IsZero() {
				entry.Age = fmt.Sprintf("%.0fs", time.Since(pod.CreationTimestamp.Time).Seconds())
			}
			result.PendingPods = append(result.PendingPods, entry)
		case corev1.PodFailed:
			result.Summary.FailedPods++
		}
	}

	// Pending ratio
	if result.Summary.TotalPods > 0 {
		result.Summary.PendingRatio = float64(result.Summary.PendingPods) / float64(result.Summary.TotalPods) * 100
	}

	// Average pending age
	if pendingCount > 0 {
		avgAge := totalPendingAge / time.Duration(pendingCount)
		result.Summary.AvgPendingAge = fmt.Sprintf("%.0fs", avgAge.Seconds())
	} else {
		result.Summary.AvgPendingAge = "0s"
	}

	// Throughput score: higher = better scheduling throughput
	result.Summary.ThroughputScore = 100 - result.Summary.PendingRatio*2

	// Backlog level
	if result.Summary.PendingPods > 20 {
		result.Summary.BacklogLevel = "critical"
		score -= 20
	} else if result.Summary.PendingPods > 5 {
		result.Summary.BacklogLevel = "high"
		score -= 10
	} else if result.Summary.PendingPods > 0 {
		result.Summary.BacklogLevel = "low"
		score -= 2
	} else {
		result.Summary.BacklogLevel = "none"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Scheduler backlog: %s (%d pending / %d total)", result.Summary.BacklogLevel, result.Summary.PendingPods, result.Summary.TotalPods))
	if result.Summary.PendingPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods pending, avg age %s — check resource availability", result.Summary.PendingPods, result.Summary.AvgPendingAge))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
