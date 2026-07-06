package dashboard

import (
	"testing"
)

func TestGSGraceCategory(t *testing.T) {
	if cat := gsGraceCategory(5); cat != "short" {
		t.Errorf("Expected short for 5s, got %s", cat)
	}
	if cat := gsGraceCategory(30); cat != "default" {
		t.Errorf("Expected default for 30s, got %s", cat)
	}
	if cat := gsGraceCategory(90); cat != "long" {
		t.Errorf("Expected long for 90s, got %s", cat)
	}
	if cat := gsGraceCategory(20); cat != "custom" {
		t.Errorf("Expected custom for 20s, got %s", cat)
	}
}

func TestGSAssessRisk(t *testing.T) {
	// No preStop + no readiness = critical
	entry := GSEntry{HasPreStop: false, HasReadiness: false}
	if level := gsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	// No preStop = high
	entry = GSEntry{HasPreStop: false, HasReadiness: true}
	if level := gsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	// Short grace = high
	entry = GSEntry{HasPreStop: true, HasReadiness: true, GraceCategory: "short"}
	if level := gsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for short grace, got %s", level)
	}

	// No readiness = medium
	entry = GSEntry{HasPreStop: true, HasReadiness: false}
	if level := gsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}

	// Fully compliant = low
	entry = GSEntry{HasPreStop: true, HasReadiness: true, GraceCategory: "default"}
	if level := gsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestGSScore(t *testing.T) {
	// Empty
	if score := gsScore(GSSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := GSSummary{TotalContainers: 10, HasPreStop: 10, HasReadiness: 10}
	if score := gsScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = GSSummary{
		TotalContainers: 10,
		LikelyDropReqs:  3, // -45
		NoPreStop:       4, // -20
		NoReadiness:     3, // -12
		GraceShort:      2, // -6
	}
	// 100 - 45 - 20 - 12 - 6 = 17
	if score := gsScore(s); score != 17 {
		t.Errorf("Expected 17, got %d", score)
	}
}

func TestGSGenRecs(t *testing.T) {
	s := GSSummary{
		TotalContainers: 10,
		LikelyDropReqs:  2,
		NoPreStop:       5,
		NoReadiness:     3,
		GraceShort:      1,
		GraceLong:       2,
		ShutdownScore:   30,
	}
	noPreStop := []GSEntry{{Namespace: "default", Workload: "api"}}

	recs := gsGenRecs(s, noPreStop, nil)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundDrop := false
	foundPreStop := false
	foundReadiness := false
	for _, r := range recs {
		if strContains(r, "drop in-flight") {
			foundDrop = true
		}
		if strContains(r, "preStop hook") {
			foundPreStop = true
		}
		if strContains(r, "readiness probe") {
			foundReadiness = true
		}
	}
	if !foundDrop {
		t.Error("Expected recommendation about dropped requests")
	}
	if !foundPreStop {
		t.Error("Expected recommendation about preStop hooks")
	}
	if !foundReadiness {
		t.Error("Expected recommendation about readiness probes")
	}
}

func TestGSGenRecsClean(t *testing.T) {
	s := GSSummary{TotalContainers: 10, HasPreStop: 10}
	recs := gsGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestGSJoinCmd(t *testing.T) {
	if cmd := gsJoinCmd([]string{}); cmd != "" {
		t.Errorf("Expected empty, got %s", cmd)
	}
	if cmd := gsJoinCmd([]string{"sleep", "5"}); cmd != "sleep 5" {
		t.Errorf("Expected 'sleep 5', got %s", cmd)
	}
	if cmd := gsJoinCmd([]string{"sh", "-c", "nginx", "-s", "quit", "extra"}); !strContains(cmd, "...") {
		t.Errorf("Expected truncation for long command, got %s", cmd)
	}
}

func TestGSRiskRank(t *testing.T) {
	if gsRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if gsRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if gsRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if gsRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestGSIssueRank(t *testing.T) {
	if gsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if gsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if gsIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
