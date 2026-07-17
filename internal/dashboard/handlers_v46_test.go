package dashboard

import (
	"testing"
)

func TestBuildHeatmapRecs(t *testing.T) {
	r := &DeployHeatmapResult{
		Summary:  HeatmapSummary{TotalUpdates: 15, ActiveNS: 5, PeakHour: 14, PeakWeekday: "Wednesday"},
		Hotspots: []HeatmapHotspot{{Namespace: "prod", Detail: "prod: 35 updates", Severity: "high"}},
	}
	recs := buildHeatmapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildHeatmapRecsLull(t *testing.T) {
	r := &DeployHeatmapResult{
		Summary: HeatmapSummary{TotalUpdates: 0, LullDetected: true},
	}
	recs := buildHeatmapRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildLogVolumeRecs(t *testing.T) {
	r := &LogVolumeResult{
		Summary:        LogVolSummary{EstimatedDailyMB: 500, EstimatedWeeklyGB: 3.4, NoisyWorkloads: 3},
		NoisyWorkloads: []LogVolEntry{{Workload: "noisy-app", Namespace: "prod", EstDailyMB: 200, LogLevel: "debug"}},
	}
	recs := buildLogVolumeRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
