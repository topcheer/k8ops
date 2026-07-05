package dashboard

import (
	"testing"
	"time"
)

func TestEstimatePVGrowthRate(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Low usage — no growth
	if rate := estimatePVGrowthRate(20, 100, old); rate != 0 {
		t.Errorf("Expected 0 for low usage, got %f", rate)
	}

	// High usage (>90%), 100GB capacity
	rate := estimatePVGrowthRate(92, 100, old)
	// 5% of 100 = 5 GB/day
	if rate != 5 {
		t.Errorf("Expected 5 GB/day for 92%% usage, got %f", rate)
	}

	// Medium usage (>50%), 50GB capacity
	rate = estimatePVGrowthRate(60, 50, old)
	// 2% of 50 = 1 GB/day
	if rate != 1 {
		t.Errorf("Expected 1 GB/day for 60%% usage, got %f", rate)
	}

	// Zero capacity
	if rate := estimatePVGrowthRate(90, 0, old); rate != 0 {
		t.Errorf("Expected 0 for zero capacity, got %f", rate)
	}
}

func TestAssessStorageRisk(t *testing.T) {
	// Critical — >95%
	entry := PVForecastEntry{UsagePct: 97}
	if level := assessStorageRisk(entry); level != "critical" {
		t.Errorf("Expected critical for 97%%, got %s", level)
	}

	// High — >85%
	entry = PVForecastEntry{UsagePct: 88}
	if level := assessStorageRisk(entry); level != "high" {
		t.Errorf("Expected high for 88%%, got %s", level)
	}

	// High — will exhaust <14d
	entry = PVForecastEntry{UsagePct: 70, DaysToExhaust: 10}
	if level := assessStorageRisk(entry); level != "high" {
		t.Errorf("Expected high for 10d to exhaust, got %s", level)
	}

	// Medium — will exhaust <30d
	entry = PVForecastEntry{UsagePct: 60, DaysToExhaust: 20}
	if level := assessStorageRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 20d to exhaust, got %s", level)
	}

	// Low
	entry = PVForecastEntry{UsagePct: 30, DaysToExhaust: 100}
	if level := assessStorageRisk(entry); level != "low" {
		t.Errorf("Expected low for healthy PV, got %s", level)
	}
}

func TestCalculateStorageForecastScore(t *testing.T) {
	// Empty
	if score := calculateStorageForecastScore(StorageForecastSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// With issues
	s := StorageForecastSummary{
		TotalPVs:    10,
		PVsFull:     1,  // -12
		PVsNearFull: 2,  // -12
		PVsCritical: 1,  // -8
		UsagePct:    82, // -8
	}
	// 100 - 12 - 12 - 8 - 8 = 60
	if score := calculateStorageForecastScore(s); score != 60 {
		t.Errorf("Expected 60, got %d", score)
	}

	// Floor at 0
	terrible := StorageForecastSummary{
		TotalPVs:    5,
		PVsFull:     10, // -120
		PVsCritical: 5,  // -40
	}
	if score := calculateStorageForecastScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestGenerateStorageForecastRecs(t *testing.T) {
	s := StorageForecastSummary{
		PVsFull:      1,
		PVsNearFull:  2,
		PVsCritical:  1,
		ForecastDays: 20,
		UsagePct:     85,
		HealthScore:  40,
	}
	pvs := []PVForecastEntry{
		{Name: "pv-1", Namespace: "default", PVCName: "data-1", UsagePct: 97, RiskLevel: "critical", DaysToExhaust: 2},
	}

	recs := generateStorageForecastRecs(s, pvs)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundFull := false
	foundCritical := false
	foundTopPV := false
	for _, r := range recs {
		if containsSubstr(r, "95%") || containsSubstr(r, ">95") {
			foundFull = true
		}
		if containsSubstr(r, "7 days") {
			foundCritical = true
		}
		if containsSubstr(r, "Most critical PV") {
			foundTopPV = true
		}
	}
	if !foundFull {
		t.Error("Expected recommendation about full PVs")
	}
	if !foundCritical {
		t.Error("Expected recommendation about critical PVs")
	}
	if !foundTopPV {
		t.Error("Expected recommendation about most critical PV")
	}
}

func TestGenerateStorageForecastRecsClean(t *testing.T) {
	s := StorageForecastSummary{
		TotalPVs:    5,
		UsagePct:    30,
		HealthScore: 100,
	}
	recs := generateStorageForecastRecs(s, nil)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestGetOrCreateSCForecast(t *testing.T) {
	m := make(map[string]*SCForecastStat)

	e1 := getOrCreateSCForecast(m, "fast-ssd")
	e1.PVCount = 5

	e2 := getOrCreateSCForecast(m, "fast-ssd")
	if e2.PVCount != 5 {
		t.Errorf("Expected same entry with PVCount=5, got %d", e2.PVCount)
	}

	e3 := getOrCreateSCForecast(m, "")
	if e3.Name != "<default>" {
		t.Errorf("Expected '<default>', got %q", e3.Name)
	}
}

func TestGetOrCreateAtRiskNS(t *testing.T) {
	m := make(map[string]*AtRiskNamespace)

	e1 := getOrCreateAtRiskNS(m, "default")
	e1.PVCount = 3

	e2 := getOrCreateAtRiskNS(m, "default")
	if e2.PVCount != 3 {
		t.Errorf("Expected same entry with PVCount=3, got %d", e2.PVCount)
	}

	e3 := getOrCreateAtRiskNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestStorageRiskRank(t *testing.T) {
	if storageRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if storageRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if storageRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if storageRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
