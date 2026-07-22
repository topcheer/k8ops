package dashboard

import "testing"

func TestRestartRateResult1928(t *testing.T) {
	r := RestartRateResult1928{
		Summary: RestartRateSummary1928{TotalPods: 80, WithRestarts: 15, FlappingCount: 3, CrashLoopCount: 1},
	}
	if r.Summary.FlappingCount != 3 {
		t.Errorf("expected 3, got %d", r.Summary.FlappingCount)
	}
}

func TestRestartRateEntry1928(t *testing.T) {
	e := RestartRateEntry1928{Name: "api-1", Restarts: 12, RestartRate: "3.5/day", Status: "unstable"}
	if e.Status != "unstable" {
		t.Errorf("expected unstable")
	}
}

func TestRestartFlapEntry1928(t *testing.T) {
	f := RestartFlapEntry1928{Name: "worker-1", Restarts: 50, Severity: "critical"}
	if f.Severity != "critical" {
		t.Errorf("expected critical")
	}
}

func TestNodeAffinityResult1928(t *testing.T) {
	r := NodeAffinityResult1928{
		Summary: NodeAffinitySummary1928{TotalPods: 80, WithNodeSelector: 10, Violations: 2},
	}
	if r.Summary.Violations != 2 {
		t.Errorf("expected 2, got %d", r.Summary.Violations)
	}
}

func TestNodeAffinityEntry1928(t *testing.T) {
	e := NodeAffinityEntry1928{PodName: "db-1", NodeName: "node-1", HasAffinity: true}
	if !e.HasAffinity {
		t.Errorf("expected affinity=true")
	}
}

func TestQuotaPressureResult1928(t *testing.T) {
	r := QuotaPressureResult1928{
		Summary: QuotaPressureSummary1928{TotalNamespaces: 30, WithQuota: 15, CriticalCount: 3},
	}
	if r.Summary.CriticalCount != 3 {
		t.Errorf("expected 3, got %d", r.Summary.CriticalCount)
	}
}

func TestQuotaResource1928(t *testing.T) {
	q := QuotaResource1928{Name: "cpu", Hard: "10", Used: "8", Utilization: 80, Status: "warning"}
	if q.Status != "warning" {
		t.Errorf("expected warning")
	}
}

func TestQuotaCriticalEntry1928(t *testing.T) {
	c := QuotaCriticalEntry1928{Namespace: "prod", Resource: "memory", Utilization: 95}
	if c.Utilization != 95 {
		t.Errorf("expected 95")
	}
}
