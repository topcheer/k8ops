package dashboard

import "testing"

func TestOwnershipRegistryResult1938(t *testing.T) {
	r := OwnershipRegistryResult1938{Summary: OwnershipRegistrySummary1938{TotalWorkloads: 30, WithOwner: 20, WithoutOwner: 10}}
	if r.Summary.WithoutOwner != 10 {
		t.Errorf("expected 10")
	}
}
func TestOwnershipEntry1938(t *testing.T) {
	e := OwnershipEntry1938{Name: "api", Owner: "team-a", Team: "backend"}
	if e.Team != "backend" {
		t.Errorf("expected backend")
	}
}
func TestTeamStat1938(t *testing.T) {
	s := TeamStat1938{Team: "devops", Workloads: 15}
	if s.Workloads != 15 {
		t.Errorf("expected 15")
	}
}
func TestAPIInventoryResult1938(t *testing.T) {
	r := APIInventoryResult1938{Summary: APIInventorySummary1938{TotalResources: 200, CoreResources: 50, CRDResources: 150}}
	if r.Summary.CRDResources != 150 {
		t.Errorf("expected 150")
	}
}
func TestAPIDeprecatedEntry1938(t *testing.T) {
	e := APIDeprecatedEntry1938{Name: "ingress", Version: "extensions/v1beta1"}
	if e.Version != "extensions/v1beta1" {
		t.Errorf("expected extensions/v1beta1")
	}
}
func TestCapacityReportResult1938(t *testing.T) {
	r := CapacityReportResult1938{Summary: CapacityReportSummary1938{TotalNodes: 3, TotalCPUCore: 8.0, CPUUtilization: 65.5}}
	if r.Summary.TotalCPUCore != 8.0 {
		t.Errorf("expected 8.0")
	}
}
func TestCapacityNodeEntry1938(t *testing.T) {
	e := CapacityNodeEntry1938{Name: "node-1", CPUCapacity: "4", MemCapacity: "16GB", PodCount: 50}
	if e.PodCount != 50 {
		t.Errorf("expected 50")
	}
}
func TestCapacityAllocEntry1938(t *testing.T) {
	e := CapacityAllocEntry1938{Namespace: "prod", CPUReq: 2.5, MemReq: 8.0}
	if e.CPUReq != 2.5 {
		t.Errorf("expected 2.5")
	}
}
