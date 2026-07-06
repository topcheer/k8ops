package dashboard

import (
	"testing"
)

func TestPRIsConfigError(t *testing.T) {
	configErrors := []string{"CreateContainerConfigError", "CreateContainerError", "InvalidImageName", "ErrImagePull", "ImagePullBackOff"}
	for _, r := range configErrors {
		if !prIsConfigError(r) {
			t.Errorf("Expected '%s' to be config error", r)
		}
	}

	nonConfig := []string{"OOMKilled", "Error", "Completed", ""}
	for _, r := range nonConfig {
		if prIsConfigError(r) {
			t.Errorf("Expected '%s' to NOT be config error", r)
		}
	}
}

func TestPRAssessRisk(t *testing.T) {
	// OOM with many restarts = critical
	entry := PREntry{LastReason: "OOMKilled", RestartCount: 15}
	if level := prAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for OOM >10 restarts, got %s", level)
	}

	// OOM few restarts = high
	entry = PREntry{LastReason: "OOMKilled", RestartCount: 3}
	if level := prAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for OOM, got %s", level)
	}

	// Config error = high
	entry = PREntry{LastReason: "CreateContainerError", RestartCount: 2}
	if level := prAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for config error, got %s", level)
	}

	// Many restarts, no OOM = critical
	entry = PREntry{LastReason: "Error", RestartCount: 25}
	if level := prAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for >20 restarts, got %s", level)
	}

	// Some restarts = medium
	entry = PREntry{LastReason: "Error", RestartCount: 3}
	if level := prAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}
}

func TestPRScore(t *testing.T) {
	// Empty
	if score := prScore(PRSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := PRSummary{TotalPods: 50, HealthyPods: 50}
	if score := prScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Some restarts
	s = PRSummary{TotalPods: 100, RestartedPods: 20}
	// restartRate = 20%, score = 100 - 30 = 70
	if score := prScore(s); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	// Heavy restarts
	s = PRSummary{TotalPods: 10, RestartedPods: 9}
	// restartRate = 90%, score = 100 - 135 < 0 → 0
	if score := prScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestPRNSScore(t *testing.T) {
	// Empty
	if score := prNSScore(PRNSEntry{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	ns := PRNSEntry{TotalPods: 20, RestartedPods: 4}
	// restartRate = 20%, score = 100 - 30 = 70
	if score := prNSScore(ns); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}
}

func TestPRGenRecs(t *testing.T) {
	s := PRSummary{
		TotalPods:        50,
		RestartedPods:    15,
		OOMKills:         5,
		AppErrors:        3,
		ConfigErrors:     2,
		DeadlineExceeded: 1,
		MaxRestarts:      60,
		TotalRestarts:    150,
		StabilityScore:   55,
	}

	recs := prGenRecs(s, nil, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundOOM := false
	foundAppError := false
	foundConfig := false
	for _, r := range recs {
		if strContains(r, "OOMKilled") {
			foundOOM = true
		}
		if strContains(r, "non-zero") {
			foundAppError = true
		}
		if strContains(r, "config") {
			foundConfig = true
		}
	}
	if !foundOOM {
		t.Error("Expected recommendation about OOM")
	}
	if !foundAppError {
		t.Error("Expected recommendation about app errors")
	}
	if !foundConfig {
		t.Error("Expected recommendation about config errors")
	}
}

func TestPRGenRecsClean(t *testing.T) {
	s := PRSummary{TotalPods: 50, RestartedPods: 0}
	recs := prGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestPRGetOrCreateNS(t *testing.T) {
	m := make(map[string]*PRNSEntry)
	e1 := prGetOrCreateNS(m, "default")
	e1.TotalPods = 5

	e2 := prGetOrCreateNS(m, "default")
	if e2.TotalPods != 5 {
		t.Errorf("Expected same entry, got %d", e2.TotalPods)
	}

	e3 := prGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}

func TestPRIssueRank(t *testing.T) {
	if prIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if prIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if prIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
