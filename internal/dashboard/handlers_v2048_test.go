package dashboard

import "testing"

func TestAutoBehaviorResult2048(t *testing.T) {
	r := AutoBehaviorResult2048{Summary: AutoBehaviorSummary2048{TotalHPAs: 10, WithBehavior: 3, NoBehavior: 7, WithPolicies: 2}}
	if r.Summary.NoBehavior != 7 {
		t.Errorf("expected 7")
	}
}
func TestAutoBehaviorEntry2048(t *testing.T) {
	e := AutoBehaviorEntry2048{Name: "api-hpa", Namespace: "prod"}
	if e.Name != "api-hpa" {
		t.Errorf("expected api-hpa")
	}
}
func TestNodePoolResult2048(t *testing.T) {
	r := NodePoolResult2048{Summary: NodePoolSummary2048{TotalNodes: 5, UniqueTypes: 2, Zones: 1, SingleZone: 5}}
	if r.Summary.SingleZone != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodePoolEntry2048(t *testing.T) {
	e := NodePoolEntry2048{InstanceType: "n1-standard-2", Count: 3, Zone: "us-central1-a"}
	if e.Count != 3 {
		t.Errorf("expected 3")
	}
}
func TestCSIDriverResult2048(t *testing.T) {
	r := CSIDriverResult2048{Summary: CSIDriverSummary2048{TotalDrivers: 3, StorageClasses: 5}}
	if r.Summary.TotalDrivers != 3 {
		t.Errorf("expected 3")
	}
}
func TestCSIDriverEntry2048(t *testing.T) {
	e := CSIDriverEntry2048{Name: "local-path", Provisioner: "local-path-provisioner"}
	if e.Name != "local-path" {
		t.Errorf("expected local-path")
	}
}
