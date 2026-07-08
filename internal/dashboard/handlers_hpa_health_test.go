package dashboard

import "testing"

func TestHPAAssessRisk(t *testing.T) {
	if hpaAssessRisk(HPAEntry{MetricsCount: 0}) != "high" {
		t.Error("Expected high for no metrics")
	}
	if hpaAssessRisk(HPAEntry{MetricsCount: 1, CurrentReplicas: 10, MaxReplicas: 10}) != "medium" {
		t.Error("Expected medium for at max")
	}
	if hpaAssessRisk(HPAEntry{MetricsCount: 1, ScalingActive: true, CurrentReplicas: 3, MaxReplicas: 10}) != "low" {
		t.Error("Expected low")
	}
}

func TestHPAScore(t *testing.T) {
	if hpaScore(HPASummary{}) != 100 {
		t.Errorf("Expected 100, got %d", hpaScore(HPASummary{}))
	}
	s := HPASummary{
		TotalHPAs:     10,
		NoMetrics:     3, // -45
		AtMaxReplicas: 4, // -20
	}
	// 100 - 45 - 20 = 35
	if score := hpaScore(s); score != 35 {
		t.Errorf("Expected 35, got %d", score)
	}
}

func TestHPAGenRecs(t *testing.T) {
	s := HPASummary{
		TotalHPAs:     10,
		NoMetrics:     3,
		AtMaxReplicas: 2,
		ScalingActive: 7,
		HealthScore:   30,
	}
	noMetrics := []HPAEntry{{Namespace: "app", Name: "api"}}
	recs := hpaGenRecs(s, nil, noMetrics)
	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}
	foundNoMetrics := false
	foundMax := false
	for _, r := range recs {
		if strContains(r, "no metrics") {
			foundNoMetrics = true
		}
		if strContains(r, "maxReplicas") {
			foundMax = true
		}
	}
	if !foundNoMetrics {
		t.Error("Expected recommendation about no metrics")
	}
	if !foundMax {
		t.Error("Expected recommendation about at max replicas")
	}
}

func TestHPAGenRecsClean(t *testing.T) {
	s := HPASummary{TotalHPAs: 5, NoMetrics: 0, AtMaxReplicas: 0, ScalingActive: 5}
	recs := hpaGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestHPARiskRank(t *testing.T) {
	if hpaRiskRank("high") != 0 {
		t.Error("Expected 0")
	}
}

func TestHPAIssueRank(t *testing.T) {
	if hpaIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
