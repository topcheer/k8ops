package dashboard

import "testing"

func TestOwnerRefResult2049(t *testing.T) {
	r := OwnerRefResult2049{Summary: OwnerRefSummary2049{TotalPods: 100, WithOwner: 80, Orphaned: 20}}
	if r.Summary.Orphaned != 20 {
		t.Errorf("expected 20")
	}
}
func TestOwnerRefEntry2049(t *testing.T) {
	e := OwnerRefEntry2049{Pod: "standalone", Namespace: "default"}
	if e.Pod != "standalone" {
		t.Errorf("expected standalone")
	}
}
func TestSvcTypeResult2049(t *testing.T) {
	r := SvcTypeResult2049{Summary: SvcTypeSummary2049{TotalServices: 50, ClusterIP: 40, NodePort: 3, LoadBalancer: 5, ExternalName: 2}}
	if r.Summary.NodePort != 3 {
		t.Errorf("expected 3")
	}
}
func TestSvcTypeEntry2049(t *testing.T) {
	e := SvcTypeEntry2049{Name: "web", Namespace: "prod", Type: "LoadBalancer"}
	if e.Type != "LoadBalancer" {
		t.Errorf("expected LoadBalancer")
	}
}
func TestResGapResult2049(t *testing.T) {
	r := ResGapResult2049{Summary: ResGapSummary2049{TotalContainers: 200, WideGap: 30, NarrowGap: 10}}
	if r.Summary.WideGap != 30 {
		t.Errorf("expected 30")
	}
}
func TestResGapEntry2049(t *testing.T) {
	e := ResGapEntry2049{Pod: "app", Namespace: "prod", Container: "web", CPURatio: 0.05, MemRatio: 0.08}
	if e.CPURatio != 0.05 {
		t.Errorf("expected 0.05")
	}
}
