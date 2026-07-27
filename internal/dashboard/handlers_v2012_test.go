package dashboard

import "testing"

func TestETCDSizeResult2012(t *testing.T) {
	r := ETCDSizeResult2012{Summary: ETCDSizeSummary2012{TotalObjects: 500, EstDBSizeMB: 125.0, SizeLevel: "low"}}
	if r.Summary.EstDBSizeMB != 125.0 {
		t.Errorf("expected 125")
	}
}
func TestETCDSizeEntry2012(t *testing.T) {
	e := ETCDSizeEntry2012{Type: "Pods", Count: 100, EstSizeMB: 2.5}
	if e.EstSizeMB != 2.5 {
		t.Errorf("expected 2.5")
	}
}
func TestSchedCacheResult2012(t *testing.T) {
	r := SchedCacheResult2012{Summary: SchedCacheSummary2012{TotalNodes: 5, TotalPods: 150, PodsPerNode: 30.0, CachePressure: "low"}}
	if r.Summary.PodsPerNode != 30.0 {
		t.Errorf("expected 30")
	}
}
func TestSchedCacheEntry2012(t *testing.T) {
	e := SchedCacheEntry2012{Node: "node-1", PodCount: 50, Pressure: 45.0}
	if e.Pressure != 45.0 {
		t.Errorf("expected 45")
	}
}
func TestAPILatResult2012(t *testing.T) {
	r := APILatResult2012{Summary: APILatSummary2012{TotalPods: 200, EstLatencyMs: 11.0, LatencyLevel: "medium"}}
	if r.Summary.EstLatencyMs != 11.0 {
		t.Errorf("expected 11")
	}
}
func TestAPILatEntry2012(t *testing.T) {
	e := APILatEntry2012{Namespace: "prod", PodCount: 50, EstLatencyMs: 3.5}
	if e.EstLatencyMs != 3.5 {
		t.Errorf("expected 3.5")
	}
}
func TestETCDSizePerType2012(t *testing.T) {
	if etcdSizePerType2012["Pods"] != 25 {
		t.Errorf("expected 25")
	}
}
func TestSchedCacheSummary2012(t *testing.T) {
	s := SchedCacheSummary2012{CachePressure: "low"}
	if s.CachePressure != "low" {
		t.Errorf("expected low")
	}
}
