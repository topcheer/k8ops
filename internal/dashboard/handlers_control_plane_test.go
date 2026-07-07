package dashboard

import (
	"testing"
	"time"
)

func TestCPAssessRisk(t *testing.T) {
	if cpAssessRisk(CPEntry{Ready: false}) != "critical" {
		t.Error("Expected critical for not ready")
	}
	if cpAssessRisk(CPEntry{Ready: true, RestartCount: 5}) != "high" {
		t.Error("Expected high for 5 restarts")
	}
	if cpAssessRisk(CPEntry{Ready: true, RestartCount: 3}) != "medium" {
		t.Error("Expected medium for 3 restarts")
	}
	if cpAssessRisk(CPEntry{Ready: true, UptimeHours: 0.5}) != "medium" {
		t.Error("Expected medium for recent restart")
	}
	if cpAssessRisk(CPEntry{Ready: true}) != "low" {
		t.Error("Expected low for healthy")
	}
}

func TestCPScore(t *testing.T) {
	if cpScore(CPSummary{}) != 100 {
		t.Errorf("Expected 100, got %d", cpScore(CPSummary{}))
	}

	s := CPSummary{
		TotalComponents:     4,
		UnhealthyComponents: 1, // -20
		RestartedPods:       2, // -10
		HasEtcd:             true,
		HasAPIServer:        true,
	}
	// 100 - 20 - 10 = 70
	if score := cpScore(s); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	// Missing etcd + apiserver
	s = CPSummary{TotalComponents: 2, HasEtcd: false, HasAPIServer: false}
	// 100 - 20 - 20 = 60
	if score := cpScore(s); score != 60 {
		t.Errorf("Expected 60, got %d", score)
	}
}

func TestCPGenRecs(t *testing.T) {
	s := CPSummary{
		TotalComponents:     4,
		UnhealthyComponents: 1,
		RestartedPods:       2,
		HealthScore:         50,
	}
	components := []CPEntry{
		{Component: "kube-apiserver", RestartCount: 3},
		{Component: "etcd", RestartCount: 0},
	}

	recs := cpGenRecs(s, components)
	if len(recs) < 2 {
		t.Errorf("Expected at least 2 recommendations, got %d", len(recs))
	}

	foundUnhealthy := false
	foundRestarts := false
	for _, r := range recs {
		if strContains(r, "not ready") {
			foundUnhealthy = true
		}
		if strContains(r, "restarted") {
			foundRestarts = true
		}
	}
	if !foundUnhealthy {
		t.Error("Expected recommendation about unhealthy components")
	}
	if !foundRestarts {
		t.Error("Expected recommendation about restarts")
	}
}

func TestCPGenRecsClean(t *testing.T) {
	s := CPSummary{TotalComponents: 4, HealthyComponents: 4}
	recs := cpGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestCPGenRecsEmpty(t *testing.T) {
	s := CPSummary{TotalComponents: 0}
	recs := cpGenRecs(s, nil)
	found := false
	for _, r := range recs {
		if strContains(r, "k3s") {
			found = true
		}
	}
	if !found {
		t.Error("Expected recommendation about k3s/microk8s")
	}
}

func TestCPRiskRank(t *testing.T) {
	if cpRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if cpRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestCPIssueRank(t *testing.T) {
	if cpIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if cpIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestCPUptimeCalc(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Hour)
	uptime := time.Since(startTime).Hours()
	if uptime < 1.9 || uptime > 2.1 {
		t.Errorf("Expected ~2h, got %.1f", uptime)
	}
}
