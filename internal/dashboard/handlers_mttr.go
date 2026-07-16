package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MTTRResult analyzes mean time to recovery from pod restart patterns and failures.
type MTTRResult struct {
	ScannedAt         time.Time           `json:"scannedAt"`
	Summary           MTTRSummary         `json:"summary"`
	TopRestarters     []PodRestarter      `json:"topRestarters"`
	IncidentFrequency IncidentFreqInfo    `json:"incidentFrequency"`
	MTTREstimate      MTTREstInfo         `json:"mttrEstimate"`
	ByNamespace       []NSMTTR            `json:"byNamespace"`
	HealthScore       int                 `json:"healthScore"`
	Grade             string              `json:"grade"`
	Recommendations   []string            `json:"recommendations"`
}

type MTTRSummary struct {
	TotalPods       int     `json:"totalPods"`
	CrashedPods     int     `json:"crashedPods"`
	AffectedPods    int     `json:"affectedPods"`
	TotalRestarts   int     `json:"totalRestarts"`
	AvgRestarts     float64 `json:"avgRestarts"`
	HighRestartPods int     `json:"highRestartPods"`
	TotalCrashLoops int     `json:"totalCrashLoops"`
	EstMTTR         string  `json:"estMTTR"`
	StabilityScore  int     `json:"stabilityScore"`
}

type IncidentFreqInfo struct {
	BurstDetected bool `json:"burstDetected"`
}

type MTTREstInfo struct {
	Minutes    int    `json:"minutes"`
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
}

type NSRestartInfo struct {
	Namespace string `json:"namespace"`
	Restarts  int    `json:"restarts"`
	Pods      int    `json:"pods"`
}

// NSMTTR is per-namespace MTTR info.
type NSMTTR struct {
	Namespace string  `json:"namespace"`
	Restarts  int     `json:"restarts"`
	Pods      int     `json:"pods"`
	Stability int     `json:"stability"`
}

// mttrFormatDuration converts seconds to human-readable duration.
func mttrFormatDuration(seconds float64) string {
	if seconds < 60 { return fmt.Sprintf("%.0fs", seconds) }
	if seconds < 3600 { return fmt.Sprintf("%.0fm", seconds/60) }
	return fmt.Sprintf("%.1fh", seconds/3600)
}

type PodRestarter struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"age"`
	Severity  string `json:"severity"`
}

// handleMTTR analyzes mean time to recovery from restart patterns.
// GET /api/product/mttr-analysis
func (s *Server) handleMTTR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MTTRResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}
	now := time.Now()

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	totalRestarts := 0
	crashedPods := 0
	highRestartPods := 0

	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] || pod.Status.Phase != "Running" { continue }
		result.Summary.TotalPods++

		podRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)
		}
		totalRestarts += podRestarts

		if podRestarts > 0 {
			crashedPods++
		}

		if podRestarts >= 5 {
			highRestartPods++
			severity := "medium"
			if podRestarts >= 10 { severity = "high" }
			if podRestarts >= 20 { severity = "critical" }

			ageStr := fmt.Sprintf("%dh", int(now.Sub(pod.CreationTimestamp.Time).Hours()))
			result.TopRestarters = append(result.TopRestarters, PodRestarter{
				Name: pod.Name, Namespace: pod.Namespace,
				Restarts: podRestarts, Age: ageStr, Severity: severity,
			})
		}
	}

	result.Summary.CrashedPods = crashedPods
	result.Summary.TotalRestarts = totalRestarts
	result.Summary.HighRestartPods = highRestartPods
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestarts = float64(totalRestarts) / float64(result.Summary.TotalPods)
	}

	// Estimate MTTR from restart patterns
	// If avg restarts > 5, MTTR is poor; if < 1, good
	mttr := "<1m"
	if result.Summary.AvgRestarts > 10 { mttr = ">15m" } else
	if result.Summary.AvgRestarts > 5 { mttr = "5-15m" } else
	if result.Summary.AvgRestarts > 1 { mttr = "1-5m" }
	result.Summary.EstMTTR = mttr

	// Score: fewer restarts = better
	score := 100
	score -= highRestartPods * 40
	score -= crashedPods * 15
	if score < 0 { score = 0 }
	result.Summary.AffectedPods = crashedPods
	result.Summary.TotalCrashLoops = highRestartPods

	// Per-namespace breakdown
	nsRestarts := map[string]int{}
	nsPodCount := map[string]int{}
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] || pod.Status.Phase != "Running" { continue }
		nsPodCount[pod.Namespace]++
		for _, cs := range pod.Status.ContainerStatuses {
			nsRestarts[pod.Namespace] += int(cs.RestartCount)
		}
	}
	for ns, restarts := range nsRestarts {
		result.ByNamespace = append(result.ByNamespace, NSMTTR{
			Namespace: ns, Restarts: restarts, Pods: nsPodCount[ns], Stability: 100 - restarts*5,
		})
	}

	// MTTR estimate
	result.MTTREstimate = MTTREstInfo{Minutes: 1, Method: "restart-based", Confidence: "low"}
	if result.Summary.AvgRestarts > 5 {
		result.MTTREstimate.Minutes = 10
		result.MTTREstimate.Confidence = "medium"
	}
	if result.Summary.AvgRestarts > 10 {
		result.MTTREstimate.Minutes = 15
		result.MTTREstimate.Confidence = "high"
	}

	// Incident frequency burst detection
	result.IncidentFrequency.BurstDetected = totalRestarts >= 10

	result.HealthScore = min(100, score)
	result.Summary.StabilityScore = result.HealthScore
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.TopRestarters, func(i, j int) bool {
		return result.TopRestarters[i].Restarts > result.TopRestarters[j].Restarts
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("MTTR analysis: %d/100 (grade %s) — %d restarts across %d pods, est MTTR %s", result.HealthScore, result.Grade, totalRestarts, result.Summary.TotalPods, mttr))
	if highRestartPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pods with >=5 restarts — investigate crash causes and add liveness probes", highRestartPods))
	}
	if len(recs) == 1 { recs = append(recs, "Pod recovery rate is healthy") }
	result.Recommendations = recs

	writeJSON(w, result)
}
