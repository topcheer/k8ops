package dashboard

import (
	"testing"
	"time"
)

func TestSaturationWatchRecs(t *testing.T) {
	r := &ResourceSaturationWatchResult{
		Summary: SaturationWatchSummary{
			TotalNodes:       5,
			CriticalNodes:    1,
			WarningNodes:     2,
			HealthyNodes:     2,
			AvgCPUSaturation: 72.5,
			AvgMemSaturation: 65.0,
			TopNamespace:     "prod",
		},
		WatchScore: 40,
		Hotspots: []SaturationNodeEntry{
			{NodeName: "worker-3", CPUSaturation: 92, MemSaturation: 85, PodSaturation: 88},
		},
	}
	recs := buildSaturationWatchRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestDeployFreqTrendRecs(t *testing.T) {
	r := &DeployFreqTrendResult{
		LookbackDays: 30,
		Summary: DeployFreqTrendSummary{
			TotalDeploys:    45,
			AvgPerDay:       1.5,
			AvgPerWeek:      10.5,
			PeakDay:         "2026-07-15",
			PeakDayCount:    8,
			ActiveWorkloads: 12,
		},
		DoraLevel:      "Elite",
		FrequencyScore: 100,
		ByWorkload: []DeployFreqTrendWLEntry{
			{Workload: "api-server", Namespace: "prod", DeployCount: 15, DaysSinceLast: 1},
		},
	}
	recs := buildDeployFreqTrendRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestOncallReadinessRecs(t *testing.T) {
	r := &OncallReadinessResult{
		Summary: OncallSummary{
			TotalChecks:       7,
			Passed:            5,
			Failed:            2,
			Warnings:          2,
			CriticalGaps:      1,
			SafeForUnattended: false,
		},
		ReadinessScore: 71,
		CoverageGaps: []OncallCheck{
			{Name: "multi-node-ha", Severity: "critical", Message: "1 worker node"},
			{Name: "pdb-coverage", Severity: "high", Message: "Low PDB ratio"},
		},
	}
	recs := buildOncallReadinessRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestSaturationWatchTypes(t *testing.T) {
	entry := SaturationNodeEntry{
		NodeName:       "worker-1",
		CPUSaturation:  75.5,
		MemSaturation:  68.0,
		DiskPressure:   false,
		PIDPressure:    false,
		PodCount:       35,
		PodCapacity:    110,
		PodSaturation:  31.8,
		OverallStatus:  "warning",
		TrendDirection: "stable",
		ETAHours:       0,
	}
	if entry.OverallStatus != "warning" || entry.PodSaturation < 30 {
		t.Error("entry should be warning with ~32% pod saturation")
	}
}

func TestDeployFreqTypes(t *testing.T) {
	dayEntry := DeployFreqTrendDayEntry{
		Date:    time.Now().Truncate(24 * time.Hour),
		Count:   5,
		DayName: "Mon",
	}
	if dayEntry.Count != 5 {
		t.Error("count should be 5")
	}

	wlEntry := DeployFreqTrendWLEntry{
		Workload:      "web-app",
		Namespace:     "prod",
		DeployCount:   20,
		LastDeploy:    time.Now().Add(-2 * 24 * time.Hour),
		DaysSinceLast: 2,
		AvgInterval:   36.0,
	}
	if wlEntry.DeployCount != 20 || wlEntry.DaysSinceLast != 2 {
		t.Error("workload entry should have correct counts")
	}
}

func TestOncallCheckTypes(t *testing.T) {
	check := OncallCheck{
		Name:     "multi-node-ha",
		Category: "infrastructure",
		Status:   "fail",
		Severity: "critical",
		Message:  "1 worker node",
		Detail:   "Need >=3 for HA",
	}
	if check.Status != "fail" || check.Severity != "critical" {
		t.Error("check should be fail/critical")
	}

	passed := OncallCheck{
		Name:     "health-probes",
		Category: "monitoring",
		Status:   "pass",
		Severity: "none",
	}
	if passed.Status != "pass" {
		t.Error("check should pass")
	}
}

func TestSafeUnattendedText(t *testing.T) {
	if safeUnattendedText(true) != "适合无人值守" {
		t.Error("should return Chinese safe text")
	}
	if safeUnattendedText(false) != "不适合无人值守" {
		t.Error("should return Chinese unsafe text")
	}
}

func TestCheckStatusOKForOncall(t *testing.T) {
	if checkStatusOKForOncall(true) != "pass" {
		t.Error("true should be pass")
	}
	if checkStatusOKForOncall(false) != "fail" {
		t.Error("false should be fail")
	}
}

func TestOncallSeverity(t *testing.T) {
	if oncallSeverity(true, "critical") != "none" {
		t.Error("passing check should have none severity")
	}
	if oncallSeverity(false, "critical") != "critical" {
		t.Error("failing check should preserve severity level")
	}
}
