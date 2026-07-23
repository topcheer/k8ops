package dashboard

import "testing"

func TestLogVolumeResult1948(t *testing.T) {
	r := LogVolumeResult1948{Summary: LogVolumeSummary1948{TotalContainers: 87, WithLogLimits: 5, EstDailyGB: 12.5}}
	if r.Summary.EstDailyGB != 12.5 {
		t.Errorf("expected 12.5")
	}
}
func TestLogVolumeEntry1948(t *testing.T) {
	e := LogVolumeEntry1948{Container: "app", HasLimit: false, EstMBPerHr: 15.5}
	if e.EstMBPerHr != 15.5 {
		t.Errorf("expected 15.5")
	}
}
func TestEvictionResult1948(t *testing.T) {
	r := EvictionResult1948{Summary: EvictionSummary1948{TotalEvictions: 10, Forced: 3, Recent24h: 2}}
	if r.Summary.Forced != 3 {
		t.Errorf("expected 3")
	}
}
func TestEvictionEntry1948(t *testing.T) {
	e := EvictionEntry1948{PodName: "web", Reason: "node-pressure", AgeHours: 12.5}
	if e.Reason != "node-pressure" {
		t.Errorf("expected node-pressure")
	}
}
func TestKubeletSyncResult1948(t *testing.T) {
	r := KubeletSyncResult1948{Summary: KubeletSyncSummary1948{TotalNodes: 3, StaleNodes: 1, AvgHeartbeatAge: 3.2}}
	if r.Summary.StaleNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestKubeletSyncEntry1948(t *testing.T) {
	e := KubeletSyncEntry1948{Node: "node-1", HeartbeatAge: 2.5, IsStale: false}
	if e.IsStale {
		t.Errorf("expected not stale")
	}
}
func TestKubeletSyncStale1948(t *testing.T) {
	e := KubeletSyncStale1948{Node: "node-2", HeartbeatAge: 20, Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestContainsStr1948(t *testing.T) {
	if !containsStr1948("log-limit", "log") {
		t.Errorf("expected true")
	}
	if containsStr1948("hello", "world") {
		t.Errorf("expected false")
	}
}
