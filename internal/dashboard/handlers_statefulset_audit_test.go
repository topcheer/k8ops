package dashboard

import (
	"testing"
)

func TestSTSAssessRisk(t *testing.T) {
	// No headless service = critical
	entry := STSEntry{HasHeadlessSvc: false}
	if level := stsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for no headless svc, got %s", level)
	}

	// Stuck rollout = high
	entry = STSEntry{HasHeadlessSvc: true, ReadyReplicas: 2, Replicas: 3}
	if level := stsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for stuck rollout, got %s", level)
	}

	// PVC Delete policy = high
	entry = STSEntry{HasHeadlessSvc: true, ReadyReplicas: 3, Replicas: 3, PVCRetentionPolicy: "Delete"}
	if level := stsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for PVC Delete, got %s", level)
	}

	// Violations = medium
	entry = STSEntry{HasHeadlessSvc: true, ReadyReplicas: 3, Replicas: 3, Violations: []string{"some issue"}}
	if level := stsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for violations, got %s", level)
	}

	// Clean = low
	entry = STSEntry{HasHeadlessSvc: true, ReadyReplicas: 3, Replicas: 3}
	if level := stsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestSTSScore(t *testing.T) {
	if score := stsScore(STSSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := STSSummary{TotalSTS: 10, Healthy: 10}
	if score := stsScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s = STSSummary{
		TotalSTS:      10,
		NoHeadlessSvc: 2, // -30
		StuckRollout:  1, // -8
		PVCDelete:     2, // -10
		HasPartition:  1, // -4
		NoPVC:         1, // -3
	}
	// 100 - 30 - 8 - 10 - 4 - 3 = 45
	if score := stsScore(s); score != 45 {
		t.Errorf("Expected 45, got %d", score)
	}
}

func TestSTSGenRecs(t *testing.T) {
	s := STSSummary{
		TotalSTS:      10,
		NoHeadlessSvc: 2,
		StuckRollout:  1,
		PVCDelete:     2,
		HasPartition:  1,
		NoPVC:         1,
		OrderedReady:  3,
		HealthScore:   40,
	}
	stuck := []STSEntry{{Namespace: "db", Name: "postgres", ReadyReplicas: 2, Replicas: 3}}

	recs := stsGenRecs(s, stuck, nil, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundHeadless := false
	foundStuck := false
	foundPVC := false
	foundOrdered := false
	for _, r := range recs {
		if strContains(r, "headless service") {
			foundHeadless = true
		}
		if strContains(r, "stuck rollout") {
			foundStuck = true
		}
		if strContains(r, "PVC Delete") {
			foundPVC = true
		}
		if strContains(r, "OrderedReady") {
			foundOrdered = true
		}
	}
	if !foundHeadless {
		t.Error("Expected recommendation about headless service")
	}
	if !foundStuck {
		t.Error("Expected recommendation about stuck rollouts")
	}
	if !foundPVC {
		t.Error("Expected recommendation about PVC Delete policy")
	}
	if !foundOrdered {
		t.Error("Expected recommendation about OrderedReady")
	}
}

func TestSTSGenRecsClean(t *testing.T) {
	s := STSSummary{TotalSTS: 5, Healthy: 5}
	recs := stsGenRecs(s, nil, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestSTSRiskRank(t *testing.T) {
	if stsRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if stsRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if stsRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if stsRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestSTSIssueRank(t *testing.T) {
	if stsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if stsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if stsIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
