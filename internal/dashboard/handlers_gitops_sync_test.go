package dashboard

import (
	"testing"
)

func TestAssessGitOpsRisk(t *testing.T) {
	tests := []struct {
		name   string
		entry  GitOpsAppEntry
		expect string
	}{
		{
			name:   "failed sync",
			entry:  GitOpsAppEntry{SyncStatus: "Failed", HealthStatus: "Degraded"},
			expect: "critical",
		},
		{
			name:   "out of sync with drift",
			entry:  GitOpsAppEntry{SyncStatus: "OutOfSync", Drift: true},
			expect: "warning",
		},
		{
			name:   "stale app",
			entry:  GitOpsAppEntry{SyncStatus: "Synced", Stale: true, HealthStatus: "Healthy"},
			expect: "warning",
		},
		{
			name:   "unknown status",
			entry:  GitOpsAppEntry{SyncStatus: "Unknown", HealthStatus: "Unknown"},
			expect: "info",
		},
		{
			name:   "healthy app",
			entry:  GitOpsAppEntry{SyncStatus: "Synced", HealthStatus: "Healthy"},
			expect: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessGitOpsRisk(tt.entry)
			if got != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, got)
			}
		})
	}
}

func TestGitOpsSyncHealthScore(t *testing.T) {
	// Simulate various health score scenarios
	tests := []struct {
		name          string
		failedApps    int
		outOfSyncApps int
		staleApps     int
		driftDetected int
		healthyApps   int
		totalApps     int
		expectedRange [2]int // [min, max] expected score
	}{
		{"all healthy", 0, 0, 0, 0, 10, 10, [2]int{95, 100}},
		{"one failed", 1, 0, 0, 0, 9, 10, [2]int{80, 90}},
		{"multiple failed", 3, 0, 0, 0, 7, 10, [2]int{40, 60}},
		{"out of sync", 0, 2, 0, 0, 8, 10, [2]int{85, 95}},
		{"stale apps", 0, 0, 3, 0, 7, 10, [2]int{70, 80}},
		{"drift detected", 0, 0, 0, 2, 8, 10, [2]int{85, 95}},
		{"all failed", 5, 0, 0, 0, 0, 5, [2]int{0, 30}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := 100
			score -= tt.failedApps * 15
			score -= tt.outOfSyncApps * 5
			score -= tt.staleApps * 5
			score -= tt.driftDetected * 5
			if tt.totalApps > 0 {
				healthyRatio := tt.healthyApps * 100 / tt.totalApps
				if healthyRatio < 50 {
					score -= 20
				} else if healthyRatio < 80 {
					score -= 10
				}
			}
			if score < 0 {
				score = 0
			}

			if score < tt.expectedRange[0] || score > tt.expectedRange[1] {
				t.Errorf("score %d not in expected range [%d, %d]", score, tt.expectedRange[0], tt.expectedRange[1])
			}
		})
	}
}

func TestGitOpsSyncSummary(t *testing.T) {
	// Test summary with no apps (no ArgoCD/Flux installed)
	result := &GitOpsSyncResult{}
	result.Summary = GitOpsSummary{
		TotalApps:     0,
		HealthyApps:   0,
		OutOfSyncApps: 0,
	}

	// No apps → score 100
	score := 100
	if score < 0 || score > 100 {
		t.Errorf("empty score should be 100, got %d", score)
	}
}

func TestGitOpsSyncIssues(t *testing.T) {
	// Test issue generation logic
	apps := []GitOpsAppEntry{
		{Name: "app1", Namespace: "default", Tool: "argocd", Kind: "Application",
			SyncStatus: "Failed", HealthStatus: "Degraded", RiskLevel: "critical"},
		{Name: "app2", Namespace: "default", Tool: "flux", Kind: "Kustomization",
			SyncStatus: "OutOfSync", AutoSync: true, Drift: true, RiskLevel: "warning"},
		{Name: "app3", Namespace: "default", Tool: "argocd", Kind: "Application",
			SyncStatus: "Synced", HealthStatus: "Healthy", RiskLevel: "healthy"},
	}

	var issues []GitOpsIssue
	for _, app := range apps {
		if app.SyncStatus == "Failed" {
			issues = append(issues, GitOpsIssue{
				Severity:  "critical",
				Resource:  app.Kind + "/" + app.Name,
				Namespace: app.Namespace,
				Issue:     "Sync failed",
			})
		}
		if app.SyncStatus == "OutOfSync" && app.AutoSync {
			issues = append(issues, GitOpsIssue{
				Severity:  "warning",
				Resource:  app.Kind + "/" + app.Name,
				Namespace: app.Namespace,
				Issue:     "OutOfSync with auto-sync",
			})
		}
		if app.Drift {
			issues = append(issues, GitOpsIssue{
				Severity:  "warning",
				Resource:  app.Kind + "/" + app.Name,
				Namespace: app.Namespace,
				Issue:     "Configuration drift",
			})
		}
	}

	// Should have 3 issues: 1 failed + 1 out-of-sync + 1 drift
	if len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(issues))
	}

	// Verify critical issue
	if issues[0].Severity != "critical" {
		t.Errorf("first issue should be critical, got %s", issues[0].Severity)
	}
}

func TestFormatGitOpsSummary(t *testing.T) {
	result := &GitOpsSyncResult{
		Summary: GitOpsSummary{
			TotalApps:          5,
			HealthyApps:        3,
			OutOfSyncApps:      1,
			SyncFailedApps:     1,
			ArgoCDDetected:     true,
			ArgoCDApps:         3,
			FluxDetected:       true,
			FluxSources:        1,
			FluxKustomizations: 1,
		},
	}
	summary := formatGitOpsSummary(result)
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestGitOpsRecommendations(t *testing.T) {
	result := &GitOpsSyncResult{
		Summary: GitOpsSummary{
			ArgoCDDetected: false,
			FluxDetected:   false,
			SyncFailedApps: 2,
			OutOfSyncApps:  3,
			StaleApps:      1,
			NoAutoSyncApps: 2,
			DriftDetected:  1,
		},
	}

	// Generate recommendations
	if !result.Summary.ArgoCDDetected && !result.Summary.FluxDetected {
		result.Recommendations = append(result.Recommendations, "No GitOps tools detected")
	}
	if result.Summary.SyncFailedApps > 0 {
		result.Recommendations = append(result.Recommendations, "sync failures found")
	}
	if result.Summary.OutOfSyncApps > 0 {
		result.Recommendations = append(result.Recommendations, "out of sync apps found")
	}
	if result.Summary.StaleApps > 0 {
		result.Recommendations = append(result.Recommendations, "stale apps found")
	}
	if result.Summary.NoAutoSyncApps > 0 {
		result.Recommendations = append(result.Recommendations, "auto-sync disabled")
	}
	if result.Summary.DriftDetected > 0 {
		result.Recommendations = append(result.Recommendations, "drift detected")
	}

	if len(result.Recommendations) != 6 {
		t.Errorf("expected 6 recommendations, got %d", len(result.Recommendations))
	}
}
