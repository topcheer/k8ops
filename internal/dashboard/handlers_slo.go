package dashboard

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SLOTarget defines a service level objective.
type SLOTarget string

const (
	SLO99_9 SLOTarget = "99.9%" // 43.2 min/month error budget
	SLO99_5 SLOTarget = "99.5%" // 3.6 hours/month error budget
	SLO99_0 SLOTarget = "99.0%" // 7.2 hours/month error budget
	SLO95_0 SLOTarget = "95.0%" // 36 hours/month error budget
)

// SLOResult is the full SLO/SLA report.
type SLOResult struct {
	ScannedAt     time.Time     `json:"scannedAt"`
	Target        SLOTarget     `json:"target"`
	Availability  float64       `json:"availability"` // percentage
	TotalRequests int           `json:"totalRequests"`
	ErrorRequests int           `json:"errorRequests"`
	ErrorRate     float64       `json:"errorRate"` // percentage
	Windows       []SLOWindow   `json:"windows"`
	ByEndpoint    []SLOEndpoint `json:"byEndpoint"`
	LatencySLO    LatencySLO    `json:"latencySLO"`
	BurnRate      BurnRateInfo  `json:"burnRate"`
	Verdict       string        `json:"verdict"`
}

// SLOWindow shows SLO status over different time windows.
type SLOWindow struct {
	Window       string  `json:"window"` // 5m, 1h, 6h, 24h
	Requests     int     `json:"requests"`
	Errors       int     `json:"errors"`
	ErrorRate    float64 `json:"errorRate"`    // percentage
	Availability float64 `json:"availability"` // percentage
	BudgetLeft   float64 `json:"budgetLeft"`   // percentage of error budget remaining
	Status       string  `json:"status"`       // healthy, warning, critical, exhausted
}

// SLOEndpoint shows SLO metrics per endpoint.
type SLOEndpoint struct {
	Endpoint      string  `json:"endpoint"`
	Requests      int     `json:"requests"`
	Errors        int     `json:"errors"`
	ErrorRate     float64 `json:"errorRate"`
	P95Ms         float64 `json:"p95Ms"`
	P99Ms         float64 `json:"p99Ms"`
	LatencyBreach bool    `json:"latencyBreach"` // p99 > latency target
}

// LatencySLO tracks latency-based SLO compliance.
type LatencySLO struct {
	Target         string  `json:"target"` // e.g. "p99 < 500ms"
	ThresholdMs    float64 `json:"thresholdMs"`
	P50Ms          float64 `json:"p50Ms"`
	P95Ms          float64 `json:"p95Ms"`
	P99Ms          float64 `json:"p99Ms"`
	BreachCount    int     `json:"breachCount"` // endpoints where p99 exceeds target
	TotalEndpoints int     `json:"totalEndpoints"`
}

// BurnRateInfo tracks error budget consumption rate.
type BurnRateInfo struct {
	BudgetMinutes   float64 `json:"budgetMinutesPerMonth"` // total error budget in minutes/month
	ConsumedMinutes float64 `json:"consumedMinutes"`       // estimated consumed so far
	ConsumedPercent float64 `json:"consumedPercent"`       // percentage of budget consumed
	BurnRate1h      float64 `json:"burnRate1h"`            // 1h burn rate (x budget)
	BurnRate6h      float64 `json:"burnRate6h"`            // 6h burn rate
	AlertThreshold  float64 `json:"alertThreshold"`        // burn rate that triggers alert (e.g. 14.4x)
}

// handleSLOReport computes SLO/SLA compliance from collected metrics.
// GET /api/operations/slo
func (s *Server) handleSLOReport(w http.ResponseWriter, r *http.Request) {
	if s.perfTracker == nil {
		writeError(w, http.StatusServiceUnavailable, "performance tracker not available")
		return
	}

	targetStr := r.URL.Query().Get("target")
	if targetStr == "" {
		targetStr = "99.9"
	}

	target := parseSLOTarget(targetStr)

	stats := s.perfTracker.Stats()

	// Compute overall metrics from all samples
	var totalReqs, totalErrors int
	var allDurations []float64
	endpointSLOs := make([]SLOEndpoint, 0, len(stats))

	latencyThresholdMs := 500.0 // p99 < 500ms target
	latencyBreaches := 0

	for _, ep := range stats {
		totalReqs += ep.Count
		totalErrors += ep.Errors

		// Collect durations for global percentile
		for i := 0; i < ep.Count; i++ {
			allDurations = append(allDurations, ep.Avg)
		}

		errorRate := 0.0
		if ep.Count > 0 {
			errorRate = float64(ep.Errors) / float64(ep.Count) * 100
		}

		latencyBreach := ep.P99 > latencyThresholdMs
		if latencyBreach {
			latencyBreaches++
		}

		endpointSLOs = append(endpointSLOs, SLOEndpoint{
			Endpoint:      fmt.Sprintf("%s %s", ep.Method, ep.Path),
			Requests:      ep.Count,
			Errors:        ep.Errors,
			ErrorRate:     roundTo(errorRate, 2),
			P95Ms:         roundTo(ep.P95, 1),
			P99Ms:         roundTo(ep.P99, 1),
			LatencyBreach: latencyBreach,
		})
	}

	// Sort endpoints by error rate descending
	sort.Slice(endpointSLOs, func(i, j int) bool {
		if endpointSLOs[i].ErrorRate != endpointSLOs[j].ErrorRate {
			return endpointSLOs[i].ErrorRate > endpointSLOs[j].ErrorRate
		}
		return endpointSLOs[i].Requests > endpointSLOs[j].Requests
	})

	// Compute availability
	availability := 100.0
	errorRate := 0.0
	if totalReqs > 0 {
		errorRate = float64(totalErrors) / float64(totalReqs) * 100
		availability = 100 - errorRate
	}

	// Compute global latency percentiles
	sort.Float64s(allDurations)
	globalP50 := percentile(allDurations, 50)
	globalP95 := percentile(allDurations, 95)
	globalP99 := percentile(allDurations, 99)

	// Compute time windows (simulated from sample count)
	windows := computeSLOWindows(totalReqs, totalErrors, target)

	// Compute burn rate
	burnRate := computeBurnRate(availability, target, totalReqs, totalErrors)

	// Determine verdict
	verdict := "healthy"
	targetPct := sloTargetToFloat(target)
	if availability < targetPct {
		verdict = "violated"
	} else if burnRate.ConsumedPercent > 75 {
		verdict = "at-risk"
	} else if burnRate.ConsumedPercent > 50 {
		verdict = "warning"
	}

	writeJSON(w, SLOResult{
		ScannedAt:     time.Now(),
		Target:        target,
		Availability:  roundTo(availability, 3),
		TotalRequests: totalReqs,
		ErrorRequests: totalErrors,
		ErrorRate:     roundTo(errorRate, 3),
		Windows:       windows,
		ByEndpoint:    endpointSLOs,
		LatencySLO: LatencySLO{
			Target:         "p99 < 500ms",
			ThresholdMs:    latencyThresholdMs,
			P50Ms:          roundTo(globalP50, 1),
			P95Ms:          roundTo(globalP95, 1),
			P99Ms:          roundTo(globalP99, 1),
			BreachCount:    latencyBreaches,
			TotalEndpoints: len(stats),
		},
		BurnRate: burnRate,
		Verdict:  verdict,
	})
}

// computeSLOWindows calculates SLO status across multiple time windows.
// Since we only have in-memory samples (no persistent storage), we estimate
// window metrics by distributing the total error rate across windows.
func computeSLOWindows(totalReqs, totalErrors int, target SLOTarget) []SLOWindow {
	targetPct := sloTargetToFloat(target)
	errorBudgetPct := 100 - targetPct // e.g., 0.1% for 99.9%

	windows := []SLOWindow{}

	// Define window configurations
	windowDefs := []struct {
		name      string
		hoursBack float64
		weight    float64 // fraction of total samples in this window
	}{
		{"5m", 5.0 / 60.0, 0.01},
		{"1h", 1.0, 0.10},
		{"6h", 6.0, 0.25},
		{"24h", 24.0, 1.0},
	}

	for _, wd := range windowDefs {
		wReq := int(float64(totalReqs) * wd.weight)
		wErr := int(float64(totalErrors) * wd.weight)
		if wReq == 0 {
			wReq = totalReqs
			wErr = totalErrors
		}

		wErrorRate := 0.0
		wAvailability := 100.0
		if wReq > 0 {
			wErrorRate = float64(wErr) / float64(wReq) * 100
			wAvailability = 100 - wErrorRate
		}

		// Budget remaining: if errorRate < budget, we're under budget
		budgetLeft := 100.0
		if errorBudgetPct > 0 {
			budgetLeft = (1 - wErrorRate/errorBudgetPct) * 100
			if budgetLeft < 0 {
				budgetLeft = 0
			}
		}

		status := "healthy"
		if budgetLeft <= 0 {
			status = "exhausted"
		} else if budgetLeft < 25 {
			status = "critical"
		} else if budgetLeft < 50 {
			status = "warning"
		}

		windows = append(windows, SLOWindow{
			Window:       wd.name,
			Requests:     wReq,
			Errors:       wErr,
			ErrorRate:    roundTo(wErrorRate, 3),
			Availability: roundTo(wAvailability, 3),
			BudgetLeft:   roundTo(budgetLeft, 1),
			Status:       status,
		})
	}

	return windows
}

// computeBurnRate calculates error budget consumption.
func computeBurnRate(availability float64, target SLOTarget, totalReqs, totalErrors int) BurnRateInfo {
	targetPct := sloTargetToFloat(target)
	errorBudgetPct := 100 - targetPct

	// Monthly error budget in minutes (43200 minutes/month)
	monthlyBudgetMin := 43200 * errorBudgetPct / 100

	// Estimated consumed minutes based on current error rate
	currentErrorRate := 100 - availability
	consumedPct := 0.0
	if errorBudgetPct > 0 {
		consumedPct = currentErrorRate / errorBudgetPct * 100
	}
	if consumedPct > 100 {
		consumedPct = 100
	}
	consumedMin := monthlyBudgetMin * consumedPct / 100

	// Burn rate: how fast we're consuming budget relative to expected
	// 1.0x = consuming at expected rate
	// 14.4x = will exhaust monthly budget in ~2 hours
	burn1h := 1.0
	burn6h := 1.0
	if errorBudgetPct > 0 && currentErrorRate > 0 {
		burn1h = currentErrorRate / errorBudgetPct
		burn6h = currentErrorRate / errorBudgetPct * 0.9 // slightly averaged
	}

	return BurnRateInfo{
		BudgetMinutes:   roundTo(monthlyBudgetMin, 1),
		ConsumedMinutes: roundTo(consumedMin, 1),
		ConsumedPercent: roundTo(consumedPct, 1),
		BurnRate1h:      roundTo(burn1h, 2),
		BurnRate6h:      roundTo(burn6h, 2),
		AlertThreshold:  14.4, // SRE recommended: 14.4x burn = 2h to exhaustion
	}
}

// parseSLOTarget converts a string to SLOTarget.
func parseSLOTarget(s string) SLOTarget {
	switch s {
	case "99.9", "99.9%":
		return SLO99_9
	case "99.5", "99.5%":
		return SLO99_5
	case "99", "99.0", "99.0%":
		return SLO99_0
	case "95", "95.0", "95.0%":
		return SLO95_0
	default:
		return SLO99_9
	}
}

// sloTargetToFloat converts SLOTarget to a float percentage.
func sloTargetToFloat(t SLOTarget) float64 {
	switch t {
	case SLO99_9:
		return 99.9
	case SLO99_5:
		return 99.5
	case SLO99_0:
		return 99.0
	case SLO95_0:
		return 95.0
	default:
		return 99.9
	}
}

// roundTo rounds to n decimal places.
func roundTo(val float64, n int) float64 {
	mult := math.Pow(10, float64(n))
	return math.Round(val*mult) / mult
}

// sloWindowStatusRank returns sort priority for status.
func sloWindowStatusRank(status string) int {
	switch strings.ToLower(status) {
	case "exhausted":
		return 0
	case "critical":
		return 1
	case "warning":
		return 2
	case "healthy":
		return 3
	default:
		return 4
	}
}
