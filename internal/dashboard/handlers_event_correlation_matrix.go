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

// EventCorrelationMatrixResult1882 correlates events across namespaces to find systemic patterns.
type EventCorrelationMatrixResult1882 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	Summary         EventCorrMatrixSummary    `json:"summary"`
	TopReasons      []EventReasonMatrixEntry  `json:"topReasons"`
	CorrelatedNS    []EventCorrMatrixNSEntry  `json:"correlatedNamespaces"`
	RecentWarnings  []EventWarningMatrixEntry `json:"recentWarnings"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Recommendations []string                  `json:"recommendations"`
}

type EventCorrMatrixSummary struct {
	TotalEvents        int     `json:"totalEvents"`
	WarningEvents      int     `json:"warningEvents"`
	NormalEvents       int     `json:"normalEvents"`
	UniqueReasons      int     `json:"uniqueReasons"`
	AffectedNamespaces int     `json:"affectedNamespaces"`
	AvgEventsPerNS     float64 `json:"avgEventsPerNS"`
	TopReason          string  `json:"topReason"`
}

type EventReasonMatrixEntry struct {
	Reason   string `json:"reason"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
	Example  string `json:"example"`
}

type EventCorrMatrixNSEntry struct {
	Namespace    string `json:"namespace"`
	EventCount   int    `json:"eventCount"`
	WarningCount int    `json:"warningCount"`
	TopReason    string `json:"topReason"`
	RiskLevel    string `json:"riskLevel"`
}

type EventWarningMatrixEntry struct {
	Reason    string `json:"reason"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Message   string `json:"message"`
	Count     int    `json:"count"`
}

// handleEventCorrelationMatrix handles GET /api/operations/event-correlation-matrix
func (s *Server) handleEventCorrelationMatrix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EventCorrelationMatrixResult1882{ScannedAt: time.Now()}

	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 1000})

	reasonCount := make(map[string]int)
	reasonExample := make(map[string]string)
	nsData := make(map[string]*EventCorrMatrixNSEntry)
	var warnings []EventWarningMatrixEntry

	for _, ev := range events.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvents++

		reasonCount[ev.Reason]++
		if reasonExample[ev.Reason] == "" {
			reasonExample[ev.Reason] = truncateStrSafe1882(ev.Message, 100)
		}

		if nsData[ev.Namespace] == nil {
			nsData[ev.Namespace] = &EventCorrMatrixNSEntry{Namespace: ev.Namespace}
		}
		nsData[ev.Namespace].EventCount++

		if ev.Type == corev1.EventTypeWarning {
			result.Summary.WarningEvents++
			nsData[ev.Namespace].WarningCount++

			if len(warnings) < 50 {
				warnings = append(warnings, EventWarningMatrixEntry{
					Reason: ev.Reason, Kind: ev.InvolvedObject.Kind,
					Namespace: ev.Namespace, Name: ev.InvolvedObject.Name,
					Message: truncateStrSafe1882(ev.Message, 120), Count: int(ev.Count),
				})
			}
		} else {
			result.Summary.NormalEvents++
		}
	}

	// Build reason ranking
	for reason, count := range reasonCount {
		sev := "low"
		for _, w := range warnings {
			if w.Reason == reason {
				sev = "high"
				break
			}
		}
		result.TopReasons = append(result.TopReasons, EventReasonMatrixEntry{
			Reason: reason, Count: count, Severity: sev,
			Example: reasonExample[reason],
		})
	}
	sort.Slice(result.TopReasons, func(i, j int) bool {
		return result.TopReasons[i].Count > result.TopReasons[j].Count
	})
	if len(result.TopReasons) > 20 {
		result.TopReasons = result.TopReasons[:20]
	}

	result.Summary.UniqueReasons = len(reasonCount)
	result.Summary.AffectedNamespaces = len(nsData)
	if len(nsData) > 0 {
		result.Summary.AvgEventsPerNS = float64(result.Summary.TotalEvents) / float64(len(nsData))
	}
	if len(result.TopReasons) > 0 {
		result.Summary.TopReason = result.TopReasons[0].Reason
	}

	for _, e := range nsData {
		switch {
		case e.WarningCount > 20:
			e.RiskLevel = "critical"
		case e.WarningCount > 10:
			e.RiskLevel = "high"
		case e.WarningCount > 0:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		// Find top reason for this NS
		topCount := 0
		for _, tr := range result.TopReasons {
			if tr.Count > topCount {
				e.TopReason = tr.Reason
				topCount = tr.Count
			}
		}
		result.CorrelatedNS = append(result.CorrelatedNS, *e)
	}
	sort.Slice(result.CorrelatedNS, func(i, j int) bool {
		return result.CorrelatedNS[i].WarningCount > result.CorrelatedNS[j].WarningCount
	})

	result.RecentWarnings = warnings

	if result.Summary.TotalEvents > 0 {
		warningRatio := float64(result.Summary.WarningEvents) / float64(result.Summary.TotalEvents)
		result.HealthScore = int((1 - warningRatio) * 100)
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("事件关联矩阵: %d 事件, %d 警告, %d 原因, %d 受影响命名空间",
			result.Summary.TotalEvents, result.Summary.WarningEvents,
			result.Summary.UniqueReasons, result.Summary.AffectedNamespaces),
	}
	if result.Summary.WarningEvents > 50 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个警告事件, 最高频原因: %s", result.Summary.WarningEvents, result.Summary.TopReason))
	}
	writeJSON(w, result)
}

func truncateStrSafe1882(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var _ = strings.Contains
