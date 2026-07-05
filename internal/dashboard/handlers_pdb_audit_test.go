package dashboard

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestClassifyPDB(t *testing.T) {
	// Impossible: allowed < 0
	entry := PDBEntry{CurrentReplicas: 3, AllowedDisruptions: -1}
	status, risk := classifyPDB(entry)
	if status != "impossible" || risk != "critical" {
		t.Errorf("Expected impossible/critical, got %s/%s", status, risk)
	}

	// Blocked: allowed = 0 with running pods
	entry = PDBEntry{CurrentReplicas: 3, AllowedDisruptions: 0}
	status, risk = classifyPDB(entry)
	if status != "blocked" || risk != "high" {
		t.Errorf("Expected blocked/high, got %s/%s", status, risk)
	}

	// Healthy with unhealthy replicas
	entry = PDBEntry{CurrentReplicas: 5, AllowedDisruptions: 2, UnhealthyReplicas: 1}
	status, risk = classifyPDB(entry)
	if status != "healthy" || risk != "medium" {
		t.Errorf("Expected healthy/medium, got %s/%s", status, risk)
	}

	// Clean
	entry = PDBEntry{CurrentReplicas: 5, AllowedDisruptions: 2, UnhealthyReplicas: 0}
	status, risk = classifyPDB(entry)
	if status != "healthy" || risk != "low" {
		t.Errorf("Expected healthy/low, got %s/%s", status, risk)
	}
}

func TestClassifyUnprotectedRisk(t *testing.T) {
	if level := classifyUnprotectedRisk(10); level != "high" {
		t.Errorf("Expected high for 10 replicas, got %s", level)
	}
	if level := classifyUnprotectedRisk(4); level != "medium" {
		t.Errorf("Expected medium for 4 replicas, got %s", level)
	}
	if level := classifyUnprotectedRisk(2); level != "low" {
		t.Errorf("Expected low for 2 replicas, got %s", level)
	}
}

func TestCalculatePDBScore(t *testing.T) {
	// No workloads
	if score := calculatePDBScore(PDBAuditSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// Full coverage
	s := PDBAuditSummary{ProtectedCount: 5, UnprotectedCount: 0}
	if score := calculatePDBScore(s); score != 100 {
		t.Errorf("Expected 100 for full coverage, got %d", score)
	}

	// 50% coverage + 1 blocked
	s = PDBAuditSummary{
		ProtectedCount:   3,
		UnprotectedCount: 3,
		BlockedCount:     1,
	}
	// coverage = 50, score = 50 - 10 = 40
	if score := calculatePDBScore(s); score != 40 {
		t.Errorf("Expected 40, got %d", score)
	}

	// Impossible PDB
	s = PDBAuditSummary{
		ProtectedCount:   5,
		UnprotectedCount: 0,
		ImpossibleCount:  1,
	}
	// coverage = 100, score = 100 - 20 = 80
	if score := calculatePDBScore(s); score != 80 {
		t.Errorf("Expected 80, got %d", score)
	}
}

func TestGeneratePDBRecs(t *testing.T) {
	// With unprotected high-risk
	s := PDBAuditSummary{
		ProtectedCount:   3,
		UnprotectedCount: 4,
		BlockedCount:     1,
		HealthScore:      45,
	}
	unprotected := []UnprotectedEntry{
		{Name: "app1", Replicas: 6, RiskLevel: "high"},
		{Name: "app2", Replicas: 3, RiskLevel: "medium"},
	}
	blockers := []PDBBlocker{
		{Namespace: "default", Name: "pdb-1", TargetName: "app1"},
	}

	recs := generatePDBRecs(s, unprotected, blockers)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundHighRisk := false
	foundBlocked := false
	foundScore := false
	for _, r := range recs {
		if strContains(r, "high-risk") {
			foundHighRisk = true
		}
		if strContains(r, "block voluntary") {
			foundBlocked = true
		}
		if strContains(r, "health score") {
			foundScore = true
		}
	}
	if !foundHighRisk {
		t.Error("Expected recommendation about high-risk unprotected deployments")
	}
	if !foundBlocked {
		t.Error("Expected recommendation about blocking PDBs")
	}
	if !foundScore {
		t.Error("Expected recommendation about low health score")
	}
}

func TestGeneratePDBRecsFull(t *testing.T) {
	s := PDBAuditSummary{
		ProtectedCount:   5,
		UnprotectedCount: 0,
		BlockedCount:     0,
	}
	recs := generatePDBRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected at least 1 recommendation (positive)")
	}
	foundPositive := false
	for _, r := range recs {
		if strContains(r, "excellent") {
			foundPositive = true
			break
		}
	}
	if !foundPositive {
		t.Error("Expected positive recommendation for full coverage")
	}
}

func TestPDBAuditIntStrToString(t *testing.T) {
	// Integer value
	v := intstr.FromInt(3)
	result := intStrToString(&v)
	if result != "3" {
		t.Errorf("Expected '3', got '%s'", result)
	}

	// Percentage value
	v = intstr.FromString("50%")
	result = intStrToString(&v)
	if result != "50%" {
		t.Errorf("Expected '50%%', got '%s'", result)
	}

	// Nil
	result = intStrToString(nil)
	if result != "" {
		t.Errorf("Expected '' for nil, got '%s'", result)
	}
}

func TestPDBRiskRank(t *testing.T) {
	if pdbRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if pdbRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if pdbRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if pdbRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestPDBIssueRank(t *testing.T) {
	if pdbIssueRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if pdbIssueRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if pdbIssueRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}
