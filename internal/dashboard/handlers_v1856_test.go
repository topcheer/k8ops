package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestIncidentTimelineRecs(t *testing.T) {
	r := &IncidentTimelineResult{
		LookbackHours: 24,
		Summary: IncidentTimelineSummary{
			TotalEvents:     150,
			WarningEvents:   30,
			CrashEvents:     12,
			ActiveIncidents: 3,
			EventRate:       6.25,
		},
		HealthScore: 55,
		Incidents: []IncidentGroup{
			{Workload: "api-server", Namespace: "prod", EventCount: 15, Severity: "critical", Duration: "8m30s"},
		},
		TopIncident: IncidentGroup{EventCount: 15},
	}
	recs := buildIncidentTimelineRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestRollbackSafetyRecs(t *testing.T) {
	r := &RollbackSafetyResult{
		Summary: RollbackSafetySummary{
			TotalWorkloads:  40,
			SafeToRollback:  28,
			UnsafeRollback:  8,
			NoHistory:       2,
			LowHistoryDepth: 5,
			HasPVC:          10,
		},
		SafetyScore: 70,
	}
	recs := buildRollbackSafetyRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestAPISemVerRecs(t *testing.T) {
	r := &APISemanticVersionResult{
		Summary: APISemVerSummary{
			TotalResources:  120,
			GAResources:     95,
			BetaResources:   20,
			AlphaResources:  5,
			DeprecatedCount: 8,
			RemovalInNext:   3,
		},
		MaturityScore: 79,
		BreakingChanges: []APISemVerEntry{
			{Resource: "Ingress", APIVersion: "extensions/v1beta1"},
		},
	}
	recs := buildAPISemVerRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestIncidentTimelineTypes(t *testing.T) {
	ev := IncidentTimelineEvent{
		Timestamp: time.Now(),
		Kind:      "Pod",
		Reason:    "OOMKilled",
		Message:   "Container exited with 137",
		Namespace: "prod",
		Workload:  "worker",
		Severity:  "critical",
	}
	if ev.Severity != "critical" {
		t.Error("severity should be critical")
	}

	inc := IncidentGroup{
		ID:         "INC-worker-123",
		Severity:   "critical",
		Title:      "OOMKilled in prod",
		Workload:   "worker",
		EventCount: 8,
		Status:     "active",
	}
	if inc.Status != "active" {
		t.Error("should be active")
	}
}

func TestRollbackSafetyTypes(t *testing.T) {
	entry := RollbackSafetyEntry{
		Workload:        "web-app",
		Namespace:       "prod",
		Kind:            "Deployment",
		RevisionHistory: 3,
		CanRollback:     true,
		SafetyLevel:     "safe",
		RiskFactors:     []string{},
	}
	if !entry.CanRollback || entry.SafetyLevel != "safe" {
		t.Error("should be safe to rollback")
	}

	unsafe := RollbackSafetyEntry{
		Workload:        "db-app",
		Namespace:       "prod",
		RevisionHistory: 0,
		CanRollback:     false,
		SafetyLevel:     "unsafe",
		HasPVC:          true,
		RiskFactors:     []string{"no revision history", "uses PVC"},
	}
	if unsafe.CanRollback || unsafe.SafetyLevel != "unsafe" {
		t.Error("should be unsafe")
	}
}

func TestAPISemVerTypes(t *testing.T) {
	entry := APISemVerEntry{
		Resource:   "Ingress",
		APIVersion: "networking.k8s.io/v1",
		Group:      "networking.k8s.io",
		Version:    "v1",
		Maturity:   "ga",
		Count:      15,
		Deprecated: false,
	}
	if entry.Maturity != "ga" || entry.Deprecated {
		t.Error("should be GA and not deprecated")
	}

	dep := APISemVerEntry{
		Resource:   "Deployment",
		APIVersion: "apps/v1beta1",
		Maturity:   "beta",
		Deprecated: true,
		RemovedIn:  "k8s 1.16",
		Severity:   "critical",
	}
	if !dep.Deprecated || dep.Severity != "critical" {
		t.Error("should be deprecated and critical")
	}
}

func TestTruncateIncidentStr(t *testing.T) {
	short := "hello world"
	if truncateIncidentStr(short, 200) != short {
		t.Error("short string should not be truncated")
	}

	long := strings.Repeat("a", 250)
	result := truncateIncidentStr(long, 100)
	if len(result) != 103 { // 100 + "..."
		t.Errorf("expected length 103, got %d", len(result))
	}
}

func TestGroupTimelineEvents(t *testing.T) {
	now := time.Now()
	events := []IncidentTimelineEvent{
		{Timestamp: now, Workload: "app", Namespace: "prod", Severity: "warning", Reason: "BackOff"},
		{Timestamp: now.Add(1 * time.Minute), Workload: "app", Namespace: "prod", Severity: "critical", Reason: "OOMKilled"},
		{Timestamp: now.Add(2 * time.Minute), Workload: "app", Namespace: "prod", Severity: "warning", Reason: "Unhealthy"},
		{Timestamp: now.Add(10 * time.Minute), Workload: "other", Namespace: "prod", Severity: "info", Reason: "Started"},
	}
	incidents := groupTimelineEvents(events)
	if len(incidents) != 1 { // Only first 3 events form a group (same workload, within 5min)
		t.Errorf("expected 1 incident group, got %d", len(incidents))
	}
}
