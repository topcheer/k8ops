package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogVolumeResult estimates per-workload log volume to identify noisy loggers
// and log storage pressure. Uses container specs, replica counts, and
// annotation-based estimates to project daily/weekly log output.
type LogVolumeResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         LogVolSummary `json:"summary"`
	ByNamespace     []LogVolNS    `json:"byNamespace"`
	TopLoggers      []LogVolEntry `json:"topLoggers"`
	NoisyWorkloads  []LogVolEntry `json:"noisyWorkloads"`
	HealthScore     int           `json:"healthScore"`
	Grade           string        `json:"grade"`
	Recommendations []string      `json:"recommendations"`
}

type LogVolSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	EstimatedDailyMB  float64 `json:"estimatedDailyMB"`
	EstimatedWeeklyGB float64 `json:"estimatedWeeklyGB"`
	NoisyWorkloads    int     `json:"noisyWorkloads"`
	QuietWorkloads    int     `json:"quietWorkloads"`
}

type LogVolNS struct {
	Namespace string  `json:"namespace"`
	DailyMB   float64 `json:"dailyMB"`
	Workloads int     `json:"workloads"`
}

type LogVolEntry struct {
	Workload   string  `json:"workload"`
	Namespace  string  `json:"namespace"`
	Replicas   int     `json:"replicas"`
	Containers int     `json:"containers"`
	EstDailyMB float64 `json:"estimatedDailyMB"`
	LogLevel   string  `json:"logLevel"`
	Noisy      bool    `json:"noisy"`
}

// handleLogVolume handles GET /api/operations/log-volume
func (s *Server) handleLogVolume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := LogVolumeResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*LogVolNS)
	var entries []LogVolEntry

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int(ptrInt32(d.Spec.Replicas))

		if _, ok := nsMap[d.Namespace]; !ok {
			nsMap[d.Namespace] = &LogVolNS{Namespace: d.Namespace}
		}
		nsMap[d.Namespace].Workloads++

		// Estimate log volume per container
		// Base: ~10MB/day per container for typical web service
		// Adjust: more containers + replicas = more logs
		containerCount := len(d.Spec.Template.Spec.Containers)
		baseDailyMB := 10.0

		// Check log level annotations
		logLevel := "info"
		for _, c := range d.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.Name == "LOG_LEVEL" || env.Name == "LOGLEVEL" {
					logLevel = env.Value
				}
			}
		}

		// Adjust for log level
		switch logLevel {
		case "debug":
			baseDailyMB *= 5
		case "trace":
			baseDailyMB *= 10
		case "warn", "warning":
			baseDailyMB *= 0.3
		case "error":
			baseDailyMB *= 0.1
		}

		estDailyMB := baseDailyMB * float64(containerCount) * float64(replicas)
		if replicas == 0 {
			estDailyMB = 0
		}

		noisy := estDailyMB > 100 // >100MB/day per workload
		if noisy {
			result.Summary.NoisyWorkloads++
		} else if estDailyMB == 0 {
			result.Summary.QuietWorkloads++
		}

		entry := LogVolEntry{
			Workload: d.Name, Namespace: d.Namespace,
			Replicas: replicas, Containers: containerCount,
			EstDailyMB: estDailyMB, LogLevel: logLevel,
			Noisy: noisy,
		}
		entries = append(entries, entry)

		result.Summary.EstimatedDailyMB += estDailyMB
		nsMap[d.Namespace].DailyMB += estDailyMB
	}

	result.Summary.EstimatedWeeklyGB = result.Summary.EstimatedDailyMB * 7 / 1024

	// NS breakdown
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].DailyMB > result.ByNamespace[j].DailyMB
	})

	// Top loggers
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].EstDailyMB > entries[j].EstDailyMB
	})
	result.TopLoggers = entries
	if len(result.TopLoggers) > 20 {
		result.TopLoggers = result.TopLoggers[:20]
	}

	// Noisy workloads
	for _, e := range entries {
		if e.Noisy {
			result.NoisyWorkloads = append(result.NoisyWorkloads, e)
		}
	}

	// Score: more noise = lower score
	result.HealthScore = 100
	if result.Summary.NoisyWorkloads > 5 {
		result.HealthScore -= 30
	} else if result.Summary.NoisyWorkloads > 0 {
		result.HealthScore -= result.Summary.NoisyWorkloads * 5
	}
	if result.Summary.EstimatedDailyMB > 1000 {
		result.HealthScore -= 20
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildLogVolumeRecs(&result)
	writeJSON(w, result)
}

func buildLogVolumeRecs(r *LogVolumeResult) []string {
	recs := []string{
		fmt.Sprintf("预估日志量: %.0f MB/天 (%.1f GB/周)", r.Summary.EstimatedDailyMB, r.Summary.EstimatedWeeklyGB),
	}
	if r.Summary.NoisyWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载日志量过大 (>100MB/天)", r.Summary.NoisyWorkloads))
	}
	if r.Summary.NoisyWorkloads > 0 && len(r.NoisyWorkloads) > 0 {
		top := r.NoisyWorkloads[0]
		recs = append(recs, fmt.Sprintf("最大日志源: %s/%s (%.0f MB/天, level=%s)", top.Namespace, top.Workload, top.EstDailyMB, top.LogLevel))
	}
	recs = append(recs, "建议: 生产环境使用 warn 级别, debug 仅用于排查")
	return recs
}
