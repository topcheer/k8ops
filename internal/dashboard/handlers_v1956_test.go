package dashboard

import "testing"

func TestAnnotationReportResult1956(t *testing.T) {
	r := AnnotationReportResult1956{Summary: AnnotationReportSummary1956{TotalResources: 100, WithAnnotations: 60, WithoutAnnotations: 40}}
	if r.Summary.WithoutAnnotations != 40 {
		t.Errorf("expected 40")
	}
}
func TestAnnotationReportKind1956(t *testing.T) {
	e := AnnotationReportKind1956{Kind: "Deployment", WithAnnot: 30, WithoutAnnot: 10}
	if e.WithAnnot != 30 {
		t.Errorf("expected 30")
	}
}
func TestTopologyMapResult1956(t *testing.T) {
	r := TopologyMapResult1956{Summary: TopologyMapSummary1956{TotalNodes: 1, TotalPods: 91, TotalZones: 1}}
	if r.Summary.TotalPods != 91 {
		t.Errorf("expected 91")
	}
}
func TestTopologyMapNode1956(t *testing.T) {
	e := TopologyMapNode1956{Node: "node-1", Zone: "us-east", Arch: "amd64", PodCount: 91}
	if e.PodCount != 91 {
		t.Errorf("expected 91")
	}
}
func TestStorageInvResult1956(t *testing.T) {
	r := StorageInvResult1956{Summary: StorageInvSummary1956{TotalPVCs: 15, MountedPVCs: 14, UnattachedPVCs: 1}}
	if r.Summary.UnattachedPVCs != 1 {
		t.Errorf("expected 1")
	}
}
func TestStorageInvEntry1956(t *testing.T) {
	e := StorageInvEntry1956{PVCName: "data", PodName: "db-0", MountPath: "/data", ReadOnly: false}
	if e.MountPath != "/data" {
		t.Errorf("expected /data")
	}
}
func TestStorageInvUnattached1956(t *testing.T) {
	e := StorageInvUnattached1956{PVCName: "old-data", Size: "10Gi", Age: "90d"}
	if e.Age != "90d" {
		t.Errorf("expected 90d")
	}
}
func TestTopologyMapNS1956(t *testing.T) {
	e := TopologyMapNS1956{Namespace: "prod", PodCount: 15, NodeCount: 1}
	if e.NodeCount != 1 {
		t.Errorf("expected 1")
	}
}
