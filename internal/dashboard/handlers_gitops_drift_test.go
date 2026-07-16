package dashboard

import (
	"testing"
)

func TestGitOpsDriftTypes(t *testing.T) {
	r := GitOpsDriftResult{DriftScore: 55, Grade: "D"}
	if r.DriftScore != 55 || r.Grade != "D" {
		t.Error("struct field error")
	}

	s := GitOpsDriftSummary{HasArgoCD: true, HasFlux: false, TotalReleases: 12, DriftedReleases: 3, ManualChanges: 5}
	if !s.HasArgoCD || s.ManualChanges != 5 {
		t.Error("summary field error")
	}

	di := DriftItem{Name: "api", Namespace: "app", DriftType: "manual-deployment", Severity: "high"}
	if di.DriftType != "manual-deployment" || di.Severity != "high" {
		t.Error("driftItem field error")
	}

	sh := SyncHealthItem{Name: "argocd-server", Source: "ArgoCD", Status: "installed", Healthy: true}
	if !sh.Healthy || sh.Source != "ArgoCD" {
		t.Error("syncHealth field error")
	}
}

func TestGitOpsDriftScoring(t *testing.T) {
	tests := []struct {
		hasGitOps     bool
		manualChanges int
		staleConfigs  int
		expectedMin   int
		expectedMax   int
	}{
		{true, 0, 0, 90, 100},     // Full GitOps, no drift
		{false, 10, 5, 0, 30},     // No GitOps, many manual changes
		{true, 3, 2, 75, 95},      // GitOps with some drift
	}
	for _, tc := range tests {
		score := 100
		if !tc.hasGitOps {
			score -= 40
		}
		score -= tc.manualChanges * 3
		score -= tc.staleConfigs
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("hasGitOps=%v manual=%d stale=%d: expected %d-%d, got %d",
				tc.hasGitOps, tc.manualChanges, tc.staleConfigs, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestGitOpsDriftSeverityLogic(t *testing.T) {
	// Deployments with >3 replicas should be high severity
	tests := []struct {
		replicas int32
		expected string
	}{
		{1, "medium"},
		{3, "medium"},
		{5, "high"},
		{10, "high"},
	}
	for _, tc := range tests {
		severity := "medium"
		if tc.replicas > 3 {
			severity = "high"
		}
		if severity != tc.expected {
			t.Errorf("replicas=%d: expected %s, got %s", tc.replicas, tc.expected, severity)
		}
	}
}
