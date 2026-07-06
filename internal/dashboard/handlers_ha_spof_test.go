package dashboard

import (
	"testing"
)

func TestHAAssessRisk(t *testing.T) {
	// Single replica = critical
	entry := HAEntry{Replicas: 1}
	if level := haAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for single replica, got %s", level)
	}

	// Multi-replica, all on one node = critical
	entry = HAEntry{Replicas: 3, NodeSpread: 1, HasPDB: true, HasAntiAffinity: true}
	if level := haAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for single node spread, got %s", level)
	}

	// Multi-replica, no PDB = high
	entry = HAEntry{Replicas: 3, NodeSpread: 2, HasPDB: false}
	if level := haAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for no PDB, got %s", level)
	}

	// Multi-replica, PDB, no anti-affinity = medium
	entry = HAEntry{Replicas: 3, NodeSpread: 2, HasPDB: true, HasAntiAffinity: false, HasReadiness: true}
	if level := haAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for no anti-affinity, got %s", level)
	}

	// Fully HA = low
	entry = HAEntry{Replicas: 3, NodeSpread: 3, HasPDB: true, HasAntiAffinity: true, HasReadiness: true}
	if level := haAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for fully HA, got %s", level)
	}
}

func TestHAScore(t *testing.T) {
	// Empty
	if score := haScore(HASummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := HASummary{TotalWorkloads: 10, MultiReplica: 10, HasPDB: 10, HasAntiAffinity: 10, HasReadiness: 10}
	if score := haScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = HASummary{
		TotalWorkloads:   10,
		SingleReplicas:   3, // -45
		SingleNodeSpread: 1, // -12
		NoPDB:            4, // -24
		NoAntiAffinity:   3, // -9
		NoReadiness:      2, // -8
	}
	// 100 - 45 - 12 - 24 - 9 - 8 = 2
	if score := haScore(s); score != 2 {
		t.Errorf("Expected 2, got %d", score)
	}
}

func TestHANSScore(t *testing.T) {
	// Empty
	if score := haNSScore(HANSEntry{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	ns := HANSEntry{WorkloadCount: 10, SingleReplicas: 2, NoPDB: 3, NoAntiAffinity: 2}
	// 100 - 30 - 18 - 6 = 46
	if score := haNSScore(ns); score != 46 {
		t.Errorf("Expected 46, got %d", score)
	}
}

func TestHAGenRecs(t *testing.T) {
	s := HASummary{
		TotalWorkloads:   10,
		SingleReplicas:   3,
		NoPDB:            4,
		NoAntiAffinity:   2,
		SingleNodeSpread: 1,
		NoReadiness:      2,
		HAScore:          30,
	}
	singleReplicas := []HAEntry{
		{Namespace: "default", Name: "api"},
	}

	recs := haGenRecs(s, singleReplicas, nil)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundSingle := false
	foundPDB := false
	foundAntiAff := false
	foundNodeSpread := false
	for _, r := range recs {
		if strContains(r, "single replica") {
			foundSingle = true
		}
		if strContains(r, "PodDisruptionBudget") {
			foundPDB = true
		}
		if strContains(r, "anti-affinity") {
			foundAntiAff = true
		}
		if strContains(r, "single node") {
			foundNodeSpread = true
		}
	}
	if !foundSingle {
		t.Error("Expected recommendation about single replicas")
	}
	if !foundPDB {
		t.Error("Expected recommendation about PDB")
	}
	if !foundAntiAff {
		t.Error("Expected recommendation about anti-affinity")
	}
	if !foundNodeSpread {
		t.Error("Expected recommendation about single node spread")
	}
}

func TestHAGenRecsClean(t *testing.T) {
	s := HASummary{TotalWorkloads: 10, MultiReplica: 10}
	recs := haGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestHARiskRank(t *testing.T) {
	if haRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if haRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if haRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if haRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestHAIssueRank(t *testing.T) {
	if haIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if haIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if haIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestHAGetOrCreateNS(t *testing.T) {
	m := make(map[string]*HANSEntry)
	e1 := haGetOrCreateNS(m, "default")
	e1.WorkloadCount = 5

	e2 := haGetOrCreateNS(m, "default")
	if e2.WorkloadCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.WorkloadCount)
	}

	e3 := haGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}
