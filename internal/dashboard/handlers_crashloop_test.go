package dashboard

import (
	"testing"
)

func TestClassifyCrash(t *testing.T) {
	tests := []struct {
		name    string
		entry   CrashPodEntry
		pattern string
	}{
		{
			name:    "OOM",
			entry:   CrashPodEntry{LastState: "OOMKilled", ExitCode: 137},
			pattern: "oom",
		},
		{
			name:    "permission denied exit 137",
			entry:   CrashPodEntry{LastState: "Error", ExitCode: 137},
			pattern: "permission-denied",
		},
		{
			name:    "image pull backoff",
			entry:   CrashPodEntry{LastState: "ImagePullBackOff", ExitCode: 1},
			pattern: "image-issue",
		},
		{
			name:    "config error exit 1",
			entry:   CrashPodEntry{LastState: "Error", ExitCode: 1},
			pattern: "config-error",
		},
		{
			name:    "command not found exit 127",
			entry:   CrashPodEntry{LastState: "Error", ExitCode: 127},
			pattern: "config-error",
		},
		{
			name:    "rapid rolling crash",
			entry:   CrashPodEntry{LastState: "Error", ExitCode: 2, CrashInterval: 15},
			pattern: "rolling-crash",
		},
		{
			name:    "unknown pattern",
			entry:   CrashPodEntry{LastState: "Error", ExitCode: 3, CrashInterval: 300},
			pattern: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, _ := classifyCrash(tt.entry)
			if pattern != tt.pattern {
				t.Errorf("classifyCrash() pattern = %q, want %q", pattern, tt.pattern)
			}
		})
	}
}

func TestAssessCrashRisk(t *testing.T) {
	// Critical — >=10 restarts
	entry := CrashPodEntry{RestartCount: 12}
	if level := assessCrashRisk(entry); level != "critical" {
		t.Errorf("Expected critical for 12 restarts, got %s", level)
	}

	// High — >=5 restarts + rapid
	entry = CrashPodEntry{RestartCount: 5, CrashInterval: 15}
	if level := assessCrashRisk(entry); level != "critical" {
		t.Errorf("Expected critical for 5 restarts + rapid, got %s", level)
	}

	// High — >=5 restarts only
	entry = CrashPodEntry{RestartCount: 6}
	if level := assessCrashRisk(entry); level != "high" {
		t.Errorf("Expected high for 6 restarts, got %s", level)
	}

	// Medium — >=3 restarts
	entry = CrashPodEntry{RestartCount: 3}
	if level := assessCrashRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 3 restarts, got %s", level)
	}

	// Low — 1 restart
	entry = CrashPodEntry{RestartCount: 1}
	if level := assessCrashRisk(entry); level != "low" {
		t.Errorf("Expected low for 1 restart, got %s", level)
	}
}

func TestCalculateCrashScore(t *testing.T) {
	// Clean
	s := CrashLoopSummary{TotalPods: 20}
	if score := calculateCrashScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With crashes
	s = CrashLoopSummary{
		TotalPods:       20,
		CrashLoopPods:   3, // -30
		HighRestartPods: 2, // -10
		RapidRestarts:   1, // -8
		PatternRolling:  1, // -6
	}
	// 100 - 30 - 10 - 8 - 6 = 46
	if score := calculateCrashScore(s); score != 46 {
		t.Errorf("Expected 46, got %d", score)
	}

	// Floor at 0
	s = CrashLoopSummary{
		TotalPods:     5,
		CrashLoopPods: 10, // -100
	}
	if score := calculateCrashScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty cluster
	if score := calculateCrashScore(CrashLoopSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateCrashRecs(t *testing.T) {
	s := CrashLoopSummary{
		CrashLoopPods:     2,
		RapidRestarts:     1,
		PatternOOM:        1,
		PatternRolling:    1,
		PatternConfig:     2,
		PatternPermission: 1,
		HealthScore:       35,
	}
	topCrashers := []CrashPodEntry{
		{Namespace: "default", Name: "app-1", ContainerName: "main", RestartCount: 15, Pattern: "oom"},
	}

	recs := generateCrashRecs(s, nil, topCrashers)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundCrashLoop := false
	foundRolling := false
	foundTopCrasher := false
	for _, r := range recs {
		if containsSubstr(r, "CrashLoopBackOff") {
			foundCrashLoop = true
		}
		if containsSubstr(r, "rollback") {
			foundRolling = true
		}
		if containsSubstr(r, "Top crasher") {
			foundTopCrasher = true
		}
	}
	if !foundCrashLoop {
		t.Error("Expected recommendation about CrashLoopBackOff")
	}
	if !foundRolling {
		t.Error("Expected recommendation about rollback")
	}
	if !foundTopCrasher {
		t.Error("Expected recommendation about top crasher")
	}
}

func TestGenerateCrashRecsClean(t *testing.T) {
	s := CrashLoopSummary{
		TotalPods:   10,
		HealthScore: 100,
	}
	recs := generateCrashRecs(s, nil, nil)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestGetOrCreateCrashNS(t *testing.T) {
	m := make(map[string]*CrashNSStat)

	e1 := getOrCreateCrashNS(m, "default")
	e1.CrashCount = 3

	e2 := getOrCreateCrashNS(m, "default")
	if e2.CrashCount != 3 {
		t.Errorf("Expected same entry with CrashCount=3, got %d", e2.CrashCount)
	}

	e3 := getOrCreateCrashNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestCrashRiskRank(t *testing.T) {
	if crashRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if crashRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if crashRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if crashRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
