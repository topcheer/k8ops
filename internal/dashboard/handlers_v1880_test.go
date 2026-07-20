package dashboard

import (
	"testing"
)

func TestHostPathAuditResultStruct1880(t *testing.T) {
	r := HostPathAuditResult{
		Summary:     HostPathSummary{TotalPods: 83, PodsWithHostPath: 5, PrivilegedPaths: 2},
		HealthScore: 93,
	}
	if r.Summary.PrivilegedPaths != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PrivilegedPaths)
	}
}

func TestContainerCapabilitiesResultStruct1880(t *testing.T) {
	r := ContainerCapabilitiesResult{
		Summary:     CapAuditSummary{TotalContainers: 100, WithCapDrop: 10, Privileged: 3},
		HealthScore: 10,
	}
	if r.Summary.Privileged != 3 {
		t.Errorf("expected 3, got %d", r.Summary.Privileged)
	}
}

func TestReadOnlyRootFSResultStruct1880(t *testing.T) {
	r := ReadOnlyRootFSResult{
		Summary:     RORFSummary{TotalContainers: 100, ReadOnlyRootFS: 5, WritableRootFS: 95},
		HealthScore: 5,
	}
	if r.Summary.WritableRootFS != 95 {
		t.Errorf("expected 95, got %d", r.Summary.WritableRootFS)
	}
}
