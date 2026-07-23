package dashboard

import "testing"

func TestPVCLifecycleResult1930(t *testing.T) {
	r := PVCLifecycleResult1930{
		Summary: PVCLifecycleSummary1930{TotalPVCs: 15, BoundPVCs: 12, PendingPVCs: 2, ReleasedPVs: 1},
	}
	if r.Summary.PendingPVCs != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PendingPVCs)
	}
}

func TestPVCPendingEntry1930(t *testing.T) {
	e := PVCPendingEntry1930{Name: "data-pvc", Namespace: "prod", Size: "10Gi", Reason: "waiting"}
	if e.Name != "data-pvc" {
		t.Errorf("expected data-pvc")
	}
}

func TestPVCReclaimEntry1930(t *testing.T) {
	e := PVCReclaimEntry1930{PVName: "pv-001", Phase: "Released", ReclaimP: "Retain"}
	if e.ReclaimP != "Retain" {
		t.Errorf("expected Retain")
	}
}

func TestEndpointLatencyResult1930(t *testing.T) {
	r := EndpointLatencyResult1930{
		Summary: EndpointLatencySummary1930{TotalServices: 50, WithEndpoints: 40, WithoutEndpoints: 10, NotReadyCount: 5},
	}
	if r.Summary.WithoutEndpoints != 10 {
		t.Errorf("expected 10, got %d", r.Summary.WithoutEndpoints)
	}
}

func TestEndpointSlowEntry1930(t *testing.T) {
	e := EndpointSlowEntry1930{Name: "api", Namespace: "prod", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}

func TestContainerForensicsResult1930(t *testing.T) {
	r := ContainerForensicsResult1930{
		Summary: ContainerForensicsSummary1930{TotalContainers: 100, RunningCount: 85, OOMKilledCount: 3, ErrorExitCount: 5},
	}
	if r.Summary.OOMKilledCount != 3 {
		t.Errorf("expected 3, got %d", r.Summary.OOMKilledCount)
	}
}

func TestContainerStateEntry1930(t *testing.T) {
	e := ContainerStateEntry1930{PodName: "worker-1", Container: "main", State: "terminated", ExitCode: 137, OOMKilled: true}
	if !e.OOMKilled {
		t.Errorf("expected OOMKilled")
	}
}

func TestExitCodeStat1930(t *testing.T) {
	s := ExitCodeStat1930{ExitCode: 137, Count: 3, Meaning: "Killed (SIGKILL / OOM)"}
	if s.Meaning != "Killed (SIGKILL / OOM)" {
		t.Errorf("expected OOM meaning")
	}
}
