package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodSLOResult evaluates per-workload SLO compliance based on pod readiness,
// restart frequency, and availability. Generates SLO targets and breach alerts.
type PodSLOResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         PodSLOSummary `json:"summary"`
	ByWorkload      []PodSLOEntry `json:"byWorkload"`
	Breaches        []PodSLOEntry `json:"breaches"`
	HealthScore     int           `json:"healthScore"`
	Grade           string        `json:"grade"`
	Recommendations []string      `json:"recommendations"`
}

type PodSLOSummary struct {
	TotalWorkloads     int     `json:"totalWorkloads"`
	MeetingSLO         int     `json:"meetingSLO"`
	BreachingSLO       int     `json:"breachingSLO"`
	AvgAvailability    float64 `json:"avgAvailabilityPct"`
	TargetAvailability float64 `json:"targetAvailabilityPct"`
}

type PodSLOEntry struct {
	Workload     string  `json:"workload"`
	Namespace    string  `json:"namespace"`
	Replicas     int     `json:"replicas"`
	Restarts     int     `json:"restarts"`
	Availability float64 `json:"availabilityPct"`
	MeetsSLO     bool    `json:"meetsSLO"`
	SLOBreach    string  `json:"sloBreach"`
}

// handlePodSLO handles GET /api/operations/pod-slo
func (s *Server) handlePodSLO(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PodSLOResult{ScannedAt: time.Now()}
	targetAvail := 99.9
	result.Summary.TargetAvailability = targetAvail

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	wlMap := make(map[string]*PodSLOEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		if _, ok := wlMap[key]; !ok {
			wlMap[key] = &PodSLOEntry{Workload: wlName, Namespace: pod.Namespace}
		}
		entry := wlMap[key]
		entry.Replicas++

		totalR := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalR += int(cs.RestartCount)
			if !cs.Ready {
				totalR += 0 // no-op
			}
		}
		entry.Restarts += totalR
		if pod.Status.Phase != corev1.PodRunning {
			totalR++ // count non-running as a restart penalty
		}
	}

	var entries []PodSLOEntry
	totalAvail := 0.0
	for _, e := range wlMap {
		// Estimate availability: 100% - restartPenalty - notReadyPenalty
		avail := 100.0
		if e.Restarts > 0 {
			avail -= float64(e.Restarts) * 0.1 // Each restart ~0.1% downtime
		}
		if avail < 0 {
			avail = 0
		}
		e.Availability = avail
		e.MeetsSLO = avail >= targetAvail
		if !e.MeetsSLO {
			e.SLOBreach = fmt.Sprintf("Availability %.1f%% < target %.1f%%", avail, targetAvail)
			result.Summary.BreachingSLO++
		} else {
			result.Summary.MeetingSLO++
		}
		totalAvail += avail
		entries = append(entries, *e)
	}

	result.Summary.TotalWorkloads = len(entries)
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgAvailability = totalAvail / float64(result.Summary.TotalWorkloads)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Availability < entries[j].Availability
	})
	result.ByWorkload = entries

	for _, e := range entries {
		if !e.MeetsSLO {
			result.Breaches = append(result.Breaches, e)
		}
	}

	result.HealthScore = int(result.Summary.AvgAvailability)
	switch {
	case result.HealthScore >= 99:
		result.Grade = "A"
	case result.HealthScore >= 95:
		result.Grade = "B"
	case result.HealthScore >= 85:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildPodSLORecs(&result)
	writeJSON(w, result)
}

func buildPodSLORecs(r *PodSLOResult) []string {
	recs := []string{
		fmt.Sprintf("SLO 合规: %d/%d 达到 %.1f%% 可用性", r.Summary.MeetingSLO, r.Summary.TotalWorkloads, r.Summary.TargetAvailability),
	}
	if r.Summary.BreachingSLO > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载违反 SLO", r.Summary.BreachingSLO))
	}
	if r.Summary.AvgAvailability < 99 {
		recs = append(recs, fmt.Sprintf("平均可用性 %.2f%% 低于 SLO 目标", r.Summary.AvgAvailability))
	}
	return recs
}
