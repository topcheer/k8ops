package dashboard

import "testing"

func TestGracePeriodResult1990(t *testing.T) {
	r := GracePeriodResult1990{Summary: GracePeriodSummary1990{TotalPods: 80, WithCustom: 10, TooShort: 3}}
	if r.Summary.TooShort != 3 {
		t.Errorf("expected 3")
	}
}
func TestGracePeriodEntry1990(t *testing.T) {
	e := GracePeriodEntry1990{Pod: "app", Namespace: "prod", GracePeriod: 3, Issue: "too short"}
	if e.GracePeriod != 3 {
		t.Errorf("expected 3")
	}
}
func TestLimitRatioResult1990(t *testing.T) {
	r := LimitRatioResult1990{Summary: LimitRatioSummary1990{TotalContainers: 50, WithBoth: 30, AvgCPURatio: 0.75, OvercommitCPU: 2}}
	if r.Summary.AvgCPURatio != 0.75 {
		t.Errorf("expected 0.75")
	}
}
func TestLimitRatioEntry1990(t *testing.T) {
	e := LimitRatioEntry1990{Pod: "app", Container: "web", CPURatio: 0.8, MemRatio: 0.6}
	if e.CPURatio != 0.8 {
		t.Errorf("expected 0.8")
	}
}
func TestCronJobHealthResult1990(t *testing.T) {
	r := CronJobHealthResult1990{Summary: CronJobHealthSummary1990{TotalCronJobs: 5, Suspended: 1, ActiveJobs: 2}}
	if r.Summary.Suspended != 1 {
		t.Errorf("expected 1")
	}
}
func TestCronJobHealthEntry1990(t *testing.T) {
	e := CronJobHealthEntry1990{Name: "backup", Namespace: "prod", Schedule: "0 2 * * *", Suspended: false, Active: 0, LastScheduled: "5h ago"}
	if e.Schedule != "0 2 * * *" {
		t.Errorf("expected schedule")
	}
}
func TestGracePeriodSummary1990(t *testing.T) {
	s := GracePeriodSummary1990{DefaultGrace: 70, TooLong: 2}
	if s.TooLong != 2 {
		t.Errorf("expected 2")
	}
}
func TestLimitRatioSummary1990(t *testing.T) {
	s := LimitRatioSummary1990{OvercommitMem: 1, AvgMemRatio: 0.5}
	if s.AvgMemRatio != 0.5 {
		t.Errorf("expected 0.5")
	}
}
