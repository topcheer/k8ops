package dashboard

import "testing"

func TestAPIObjectCountResult1964(t *testing.T) {
	r := APIObjectCountResult1964{Summary: APIObjectCountSummary1964{TotalPods: 200, TotalServices: 150, TotalConfigMaps: 300, ApproachingLimit: 2}}
	if r.Summary.TotalPods != 200 {
		t.Errorf("expected 200")
	}
	if r.Summary.ApproachingLimit != 2 {
		t.Errorf("expected 2")
	}
}
func TestAPIObjectCountEntry1964(t *testing.T) {
	e := APIObjectCountEntry1964{ResourceType: "pods", Namespace: "prod", Count: 95, Limit: 110, UtilizationPct: 86.4}
	if e.UtilizationPct != 86.4 {
		t.Errorf("expected 86.4")
	}
}
func TestWatchCacheResult1964(t *testing.T) {
	r := WatchCacheResult1964{Summary: WatchCacheSummary1964{TotalWatchers: 150, HighVolumeObjects: 3, PressureLevel: "medium", PodChurnRate: 12.5}}
	if r.Summary.PressureLevel != "medium" {
		t.Errorf("expected medium")
	}
}
func TestWatchCacheEntry1964(t *testing.T) {
	e := WatchCacheEntry1964{ResourceType: "pods", ObjectCount: 300, WatchScore: 350.5, RiskLevel: "high"}
	if e.RiskLevel != "high" {
		t.Errorf("expected high")
	}
}
func TestClassifyWatchRisk1964(t *testing.T) {
	if classifyWatchRisk1964(600) != "critical" {
		t.Errorf("expected critical for 600")
	}
	if classifyWatchRisk1964(300) != "high" {
		t.Errorf("expected high for 300")
	}
	if classifyWatchRisk1964(100) != "medium" {
		t.Errorf("expected medium for 100")
	}
	if classifyWatchRisk1964(10) != "low" {
		t.Errorf("expected low for 10")
	}
}
func TestSchedCacheResult1964(t *testing.T) {
	r := SchedCacheResult1964{Summary: SchedCacheSummary1964{TotalPods: 100, RunningPods: 95, PendingPods: 5, BacklogLevel: "low", ThroughputScore: 90.0}}
	if r.Summary.PendingPods != 5 {
		t.Errorf("expected 5")
	}
	if r.Summary.ThroughputScore != 90.0 {
		t.Errorf("expected 90.0")
	}
}
func TestSchedCacheEntry1964(t *testing.T) {
	e := SchedCacheEntry1964{Name: "app-1", Namespace: "prod", Age: "120s", Reason: "Unschedulable"}
	if e.Reason != "Unschedulable" {
		t.Errorf("expected Unschedulable")
	}
}
func TestWatchCacheSummary1964(t *testing.T) {
	s := WatchCacheSummary1964{EstimatedEventsPerMin: 60, PressureLevel: "low"}
	if s.EstimatedEventsPerMin != 60 {
		t.Errorf("expected 60")
	}
}
