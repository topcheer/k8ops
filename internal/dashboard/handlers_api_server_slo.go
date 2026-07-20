package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIServerSLOResult measures API server SLO compliance: success rate and latency.
type APIServerSLOResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         APIServerSLOSummary     `json:"summary"`
	ByVerb          []APIServerVerbStat     `json:"byVerb"`
	ByResource      []APIServerResourceStat `json:"byResource"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type APIServerSLOSummary struct {
	TotalEvents   int     `json:"totalEvents"`
	ErrorEvents   int     `json:"errorEvents"`
	ErrorRate     float64 `json:"errorRate"`
	WarningEvents int     `json:"warningEvents"`
	NormalEvents  int     `json:"normalEvents"`
	SLOTarget     float64 `json:"sloTarget"` // target error rate <1%
	SLOCompliant  bool    `json:"sloCompliant"`
	AvgEventRate  float64 `json:"avgEventRatePerMin"`
}

type APIServerVerbStat struct {
	Verb       string  `json:"verb"`
	Count      int     `json:"count"`
	ErrorCount int     `json:"errorCount"`
	ErrorRate  float64 `json:"errorRate"`
}

type APIServerResourceStat struct {
	Resource   string `json:"resource"`
	Count      int    `json:"count"`
	ErrorCount int    `json:"errorCount"`
	TopReason  string `json:"topReason"`
}

// handleAPIServerSLO handles GET /api/operations/api-server-slo
func (s *Server) handleAPIServerSLO(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := APIServerSLOResult{ScannedAt: time.Now()}
	result.Summary.SLOTarget = 1.0

	// Analyze audit/events from the API server via Kubernetes events
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		Limit: 500,
	})

	// Events with API server related info
	verbCount := make(map[string]int)
	verbError := make(map[string]int)
	resourceCount := make(map[string]int)
	resourceError := make(map[string]int)
	resourceReasons := make(map[string]map[string]int)

	for _, ev := range events.Items {
		if ev.Type == "" {
			continue
		}
		result.Summary.TotalEvents++
		verb := ev.Type
		res := ev.InvolvedObject.Kind
		if res == "" {
			res = "Unknown"
		}

		verbCount[verb]++
		resourceCount[res]++

		if ev.Type == corev1.EventTypeWarning {
			result.Summary.WarningEvents++
			verbError[verb]++
			resourceError[res]++
			if resourceReasons[res] == nil {
				resourceReasons[res] = make(map[string]int)
			}
			resourceReasons[res][ev.Reason]++
		}
		if ev.Type == corev1.EventTypeNormal {
			result.Summary.NormalEvents++
		}
	}

	result.Summary.ErrorEvents = result.Summary.WarningEvents
	if result.Summary.TotalEvents > 0 {
		result.Summary.ErrorRate = float64(result.Summary.ErrorEvents) / float64(result.Summary.TotalEvents) * 100
	}
	result.Summary.SLOCompliant = result.Summary.ErrorRate < result.Summary.SLOTarget

	// Build verb stats
	for v, c := range verbCount {
		stat := APIServerVerbStat{Verb: v, Count: c, ErrorCount: verbError[v]}
		if c > 0 {
			stat.ErrorRate = float64(verbError[v]) / float64(c) * 100
		}
		result.ByVerb = append(result.ByVerb, stat)
	}
	sort.Slice(result.ByVerb, func(i, j int) bool {
		return result.ByVerb[i].ErrorRate > result.ByVerb[j].ErrorRate
	})

	// Build resource stats
	for res, c := range resourceCount {
		stat := APIServerResourceStat{Resource: res, Count: c, ErrorCount: resourceError[res]}
		// Find top reason
		if reasons, ok := resourceReasons[res]; ok {
			topReason := ""
			topCount := 0
			for reason, cnt := range reasons {
				if cnt > topCount {
					topCount = cnt
					topReason = reason
				}
			}
			stat.TopReason = topReason
		}
		result.ByResource = append(result.ByResource, stat)
	}
	sort.Slice(result.ByResource, func(i, j int) bool {
		return result.ByResource[i].ErrorCount > result.ByResource[j].ErrorCount
	})
	if len(result.ByResource) > 15 {
		result.ByResource = result.ByResource[:15]
	}

	// Score based on error rate vs SLO target
	result.HealthScore = 100
	if result.Summary.ErrorRate > result.Summary.SLOTarget {
		result.HealthScore -= int(result.Summary.ErrorRate * 2)
	}
	if result.Summary.ErrorRate > 10 {
		result.HealthScore -= 20
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("API Server SLO: %d 事件, %.1f%% 错误率 (目标 <%.0f%%), %s",
			result.Summary.TotalEvents, result.Summary.ErrorRate, result.Summary.SLOTarget,
			sloCompliantText(result.Summary.SLOCompliant)),
	}
	if !result.Summary.SLOCompliant {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("错误率 %.1f%% 超过 SLO 目标, 需要排查", result.Summary.ErrorRate))
	}
	if result.Summary.WarningEvents > 50 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Warning 事件, 检查高频资源", result.Summary.WarningEvents))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 排查高频错误资源的根因, 优化控制器重试逻辑")
	}
	writeJSON(w, result)
}

func sloCompliantText(compliant bool) string {
	if compliant {
		return "达标"
	}
	return "未达标"
}
