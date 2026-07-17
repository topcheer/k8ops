package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertFatigueResult analyzes Kubernetes Warning events to identify alert
// noise patterns: repeated warnings, noisy namespaces, and event storms.
// Helps operators tune alerting to reduce fatigue.
type AlertFatigueResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         FatigueSummary      `json:"summary"`
	ByNamespace     []FatigueNSStat     `json:"byNamespace"`
	ByReason        []FatigueReasonStat `json:"byReason"`
	TopOffenders    []FatigueOffender   `json:"topOffenders"`
	NoiseScore      int                 `json:"noiseScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type FatigueSummary struct {
	TotalEvents   int `json:"totalEvents"`
	WarningEvents int `json:"warningEvents"`
	NormalEvents  int `json:"normalEvents"`
	UniqueReasons int `json:"uniqueReasons"`
	RepeatCount   int `json:"repeatCount"`
	NoisyNS       int `json:"noisyNamespaces"`
}

type FatigueNSStat struct {
	Namespace string `json:"namespace"`
	Warnings  int    `json:"warnings"`
	Normal    int    `json:"normal"`
	NoisePct  int    `json:"noisePct"`
}

type FatigueReasonStat struct {
	Reason   string `json:"reason"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
	Action   string `json:"action"`
}

type FatigueOffender struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	Count     int    `json:"count"`
	LastSeen  string `json:"lastSeen"`
}

// handleAlertFatigue handles GET /api/operations/alert-fatigue
func (s *Server) handleAlertFatigue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AlertFatigueResult{ScannedAt: time.Now()}

	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
	})

	nsMap := make(map[string]*FatigueNSStat)
	reasonMap := make(map[string]int)
	reasonAction := make(map[string]string)
	reasonSeverity := make(map[string]string)
	offenderMap := make(map[string]*FatigueOffender)

	for _, ev := range events.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvents++
		result.Summary.WarningEvents++

		// Namespace stats
		if _, ok := nsMap[ev.Namespace]; !ok {
			nsMap[ev.Namespace] = &FatigueNSStat{Namespace: ev.Namespace}
		}
		nsMap[ev.Namespace].Warnings++

		// Reason stats
		reason := ev.Reason
		if reason == "" {
			reason = "Unknown"
		}
		reasonMap[reason]++
		if _, ok := reasonAction[reason]; !ok {
			reasonAction[reason] = fatigueAction(reason)
			reasonSeverity[reason] = fatigueSeverity(reason, ev.Message)
		}

		// Offender tracking
		offKey := ev.Namespace + "/" + ev.InvolvedObject.Name + "/" + ev.InvolvedObject.Kind + "/" + reason
		if _, ok := offenderMap[offKey]; !ok {
			offenderMap[offKey] = &FatigueOffender{
				Name: ev.InvolvedObject.Name, Namespace: ev.Namespace,
				Kind: ev.InvolvedObject.Kind, Reason: reason,
				LastSeen: svcAge(ev.LastTimestamp.Time),
			}
		}
		offenderMap[offKey].Count++
		if result.Summary.TotalEvents > 0 {
			result.Summary.RepeatCount++
		}
	}

	result.Summary.UniqueReasons = len(reasonMap)

	// Build reason stats
	for reason, count := range reasonMap {
		result.ByReason = append(result.ByReason, FatigueReasonStat{
			Reason: reason, Count: count,
			Severity: reasonSeverity[reason],
			Action:   reasonAction[reason],
		})
	}
	sort.Slice(result.ByReason, func(i, j int) bool {
		return result.ByReason[i].Count > result.ByReason[j].Count
	})

	// Top offenders
	for _, off := range offenderMap {
		if off.Count >= 3 {
			result.TopOffenders = append(result.TopOffenders, *off)
		}
	}
	sort.Slice(result.TopOffenders, func(i, j int) bool {
		return result.TopOffenders[i].Count > result.TopOffenders[j].Count
	})
	if len(result.TopOffenders) > 20 {
		result.TopOffenders = result.TopOffenders[:20]
	}

	// NS stats
	noisyThreshold := 10
	for _, ns := range nsMap {
		if ns.Warnings > 0 {
			ns.NoisePct = ns.Warnings * 100 / maxInt(result.Summary.WarningEvents, 1)
		}
		if ns.Warnings > noisyThreshold {
			result.Summary.NoisyNS++
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Warnings > result.ByNamespace[j].Warnings
	})

	// Score
	if result.Summary.WarningEvents > 0 {
		repeatRate := result.Summary.RepeatCount * 100 / maxInt(result.Summary.WarningEvents, 1)
		result.NoiseScore = 100 - repeatRate/2
		if result.Summary.NoisyNS > 5 {
			result.NoiseScore -= 20
		}
		if result.NoiseScore < 0 {
			result.NoiseScore = 0
		}
	} else {
		result.NoiseScore = 100
	}

	switch {
	case result.NoiseScore >= 80:
		result.Grade = "A"
	case result.NoiseScore >= 60:
		result.Grade = "B"
	case result.NoiseScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildFatigueRecs(&result)
	writeJSON(w, result)
}

func fatigueAction(reason string) string {
	actions := map[string]string{
		"FailedScheduling": "Add resources or adjust node affinity",
		"BackOff":          "Check image availability and pull policy",
		"Unhealthy":        "Review probe configuration and app health",
		"FailedMount":      "Verify Secret/ConfigMap/PVC exists",
		"Evicted":          "Check node pressure and resource limits",
		"PolicyViolation":  "Review PSA and admission policies",
	}
	if a, ok := actions[reason]; ok {
		return a
	}
	return "Investigate event details"
}

func fatigueSeverity(reason, msg string) string {
	critical := []string{"FailedScheduling", "Evicted", "PolicyViolation"}
	for _, c := range critical {
		if reason == c {
			return "critical"
		}
	}
	if strings.Contains(strings.ToLower(msg), "error") || strings.Contains(strings.ToLower(msg), "fail") {
		return "high"
	}
	return "medium"
}

func buildFatigueRecs(r *AlertFatigueResult) []string {
	recs := []string{
		fmt.Sprintf("事件噪音: %d/%d 告警事件 (%d%%)", r.Summary.WarningEvents, maxInt(r.Summary.TotalEvents, 1), r.Summary.WarningEvents*100/maxInt(r.Summary.TotalEvents, 1)),
	}
	if r.Summary.NoisyNS > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间事件超过 10 条", r.Summary.NoisyNS))
	}
	if len(r.ByReason) > 0 {
		top := r.ByReason[0]
		recs = append(recs, fmt.Sprintf("最常见原因: %s (%d 次) -> %s", top.Reason, top.Count, top.Action))
	}
	if len(r.TopOffenders) > 5 {
		recs = append(recs, fmt.Sprintf("%d 个高频告警对象，建议排查根因", len(r.TopOffenders)))
	}
	return recs
}
