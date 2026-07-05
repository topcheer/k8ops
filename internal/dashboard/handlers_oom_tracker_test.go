package dashboard

import (
	"testing"
)

func TestAssessOOMRiskCritical(t *testing.T) {
	entry := OOMPodEntry{
		RestartCount: 10,
		Containers: []OOMContainerInfo{
			{Name: "app", OOMKilled: true, HasMemLimit: false},
		},
	}
	// 15 (OOM) + 5 (no limit) + 20 (>=10 restarts) = 40 → critical
	if level := assessOOMRisk(entry); level != "critical" {
		t.Errorf("Expected critical for OOM+high restarts, got %s", level)
	}
}

func TestAssessOOMRiskHigh(t *testing.T) {
	entry := OOMPodEntry{
		RestartCount: 5,
		Containers: []OOMContainerInfo{
			{Name: "app", OOMKilled: true, HasMemLimit: true},
		},
	}
	// 15 (OOM) + 10 (>=5 restarts) = 25 → high
	if level := assessOOMRisk(entry); level != "high" {
		t.Errorf("Expected high for OOM+5 restarts, got %s", level)
	}
}

func TestAssessOOMRiskMedium(t *testing.T) {
	entry := OOMPodEntry{
		RestartCount: 3,
		Containers: []OOMContainerInfo{
			{Name: "app", OOMKilled: false, HasMemLimit: false},
		},
	}
	// 5 (no limit) = 5 → medium
	if level := assessOOMRisk(entry); level != "medium" {
		t.Errorf("Expected medium for no limit + low restarts, got %s", level)
	}
}

func TestAssessOOMRiskLow(t *testing.T) {
	entry := OOMPodEntry{
		RestartCount: 1,
		Containers: []OOMContainerInfo{
			{Name: "app", OOMKilled: false, HasMemLimit: true},
		},
	}
	// 0 → low
	if level := assessOOMRisk(entry); level != "low" {
		t.Errorf("Expected low for clean container, got %s", level)
	}
}

func TestCalculateOOMScore(t *testing.T) {
	// Clean
	clean := OOMSummary{}
	if score := calculateOOMScore(clean); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With issues
	withIssues := OOMSummary{
		OOMKilledCount:   3, // -24
		HighRestartCount: 2, // -10
		NoMemLimit:       5, // -15
		LowMemLimit:      2, // -4
	}
	// 100 - 24 - 10 - 15 - 4 = 47
	score := calculateOOMScore(withIssues)
	if score != 47 {
		t.Errorf("Expected 47, got %d", score)
	}

	// Floor at 0
	terrible := OOMSummary{
		OOMKilledCount: 20, // -160
	}
	if score := calculateOOMScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestGenerateOOMRecs(t *testing.T) {
	s := OOMSummary{
		OOMKilledCount:     3,
		HighRestartCount:   2,
		NoMemLimit:         5,
		LowMemLimit:        1,
		MemLimitMuchHigher: 2,
		OOMRiskScore:       35,
	}
	killers := []OOMKillEntry{
		{Pod: "app1", Namespace: "default", Container: "main", Restarts: 12},
	}

	recs := generateOOMRecs(s, killers)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundOOM := false
	foundNoLimit := false
	foundTopOffender := false
	for _, r := range recs {
		if containsSubstr(r, "OOMKilled") {
			foundOOM = true
		}
		if containsSubstr(r, "no memory limit") {
			foundNoLimit = true
		}
		if containsSubstr(r, "Top OOM offender") {
			foundTopOffender = true
		}
	}
	if !foundOOM {
		t.Error("Expected recommendation about OOM kills")
	}
	if !foundNoLimit {
		t.Error("Expected recommendation about missing memory limits")
	}
	if !foundTopOffender {
		t.Error("Expected recommendation about top OOM offender")
	}
}

func TestGenerateOOMRecsClean(t *testing.T) {
	s := OOMSummary{OOMRiskScore: 100}
	recs := generateOOMRecs(s, nil)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestGetOrCreateOOMNs(t *testing.T) {
	m := make(map[string]*OOMNamespaceStat)

	e1 := getOrCreateOOMNs(m, "default")
	e1.OOMCount = 3

	e2 := getOrCreateOOMNs(m, "default")
	if e2.OOMCount != 3 {
		t.Errorf("Expected same entry with OOMCount=3, got %d", e2.OOMCount)
	}

	e3 := getOrCreateOOMNs(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestOOMRiskRank(t *testing.T) {
	if oomRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if oomRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if oomRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if oomRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
