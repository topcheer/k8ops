package dashboard

import "testing"

func TestImageLifecycleResult1921(t *testing.T) {
	r := ImageLifecycleResult1921{
		Summary:     ImageLifecycleSummary1921{TotalImages: 50, UniqueImages: 30, StaleCount: 5},
		HealthScore: 88,
	}
	if r.Summary.UniqueImages != 30 {
		t.Errorf("expected 30, got %d", r.Summary.UniqueImages)
	}
	if r.HealthScore != 88 {
		t.Errorf("expected 88, got %d", r.HealthScore)
	}
}

func TestImageLifecycleSummary1921(t *testing.T) {
	s := ImageLifecycleSummary1921{
		FloatingTagCount: 12,
		PinnedCount:      8,
		MaxReuse:         25,
	}
	if s.FloatingTagCount != 12 {
		t.Errorf("expected 12, got %d", s.FloatingTagCount)
	}
	if s.MaxReuse != 25 {
		t.Errorf("expected 25, got %d", s.MaxReuse)
	}
}

func TestImageStaleEntry1921(t *testing.T) {
	e := ImageStaleEntry1921{Image: "nginx:latest", Reason: "floating tag", Severity: "warning"}
	if e.Severity != "warning" {
		t.Errorf("expected warning, got %s", e.Severity)
	}
}

func TestVolSnapshotReadyResult1921(t *testing.T) {
	r := VolSnapshotReadyResult1921{
		Summary: VolSnapshotReadySummary1921{TotalPVCs: 10, ReadyForSnapshot: 7, NotReadyCount: 3},
	}
	if r.Summary.ReadyForSnapshot != 7 {
		t.Errorf("expected 7, got %d", r.Summary.ReadyForSnapshot)
	}
}

func TestVolSnapshotNotReady1921(t *testing.T) {
	n := VolSnapshotNotReady1921{Name: "data-pvc", Namespace: "prod", Reason: "no CSI"}
	if n.Reason != "no CSI" {
		t.Errorf("expected 'no CSI', got %s", n.Reason)
	}
}

func TestIdleResourceResult1921(t *testing.T) {
	r := IdleResourceResult1921{
		Summary: IdleResourceSummary1921{TotalPods: 80, IdlePodCount: 5, UnusedServiceCount: 12, EstMonthlyCostUSD: 42.5},
	}
	if r.Summary.IdlePodCount != 5 {
		t.Errorf("expected 5, got %d", r.Summary.IdlePodCount)
	}
	if r.Summary.EstMonthlyCostUSD != 42.5 {
		t.Errorf("expected 42.5, got %f", r.Summary.EstMonthlyCostUSD)
	}
}

func TestIdleServiceEntry1921(t *testing.T) {
	e := IdleServiceEntry1921{Name: "old-svc", Namespace: "dev", Type: "ClusterIP", Age: "30d"}
	if e.Type != "ClusterIP" {
		t.Errorf("expected ClusterIP, got %s", e.Type)
	}
}

func TestWastedResourceEntry1921(t *testing.T) {
	w := WastedResourceEntry1921{Resource: "CPU", Detail: "2.5 cores", CostUSD: 70.0}
	if w.CostUSD != 70.0 {
		t.Errorf("expected 70.0, got %f", w.CostUSD)
	}
}
