package dashboard

import "testing"

func TestCPLoadResult1994(t *testing.T) {
	r := CPLoadResult1994{Summary: CPLoadSummary1994{TotalObjects: 500, TotalPods: 300, EstAPIQPS: 175.0, LoadLevel: "medium"}}
	if r.Summary.LoadLevel != "medium" {
		t.Errorf("expected medium")
	}
}
func TestCPLoadNSEntry1994(t *testing.T) {
	e := CPLoadNSEntry1994{Namespace: "prod", ObjectCount: 120}
	if e.ObjectCount != 120 {
		t.Errorf("expected 120")
	}
}
func TestVolAttachResult1994(t *testing.T) {
	r := VolAttachResult1994{Summary: VolAttachSummary1994{TotalPVCs: 20, BoundPVCs: 18, MaxPerNode: 8, DensityLevel: "low"}}
	if r.Summary.DensityLevel != "low" {
		t.Errorf("expected low")
	}
}
func TestVolAttachNodeEntry1994(t *testing.T) {
	e := VolAttachNodeEntry1994{Node: "node-1", AttachCount: 5}
	if e.AttachCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestQuotaUtilResult1994(t *testing.T) {
	r := QuotaUtilResult1994{Summary: QuotaUtilSummary1994{TotalQuotas: 5, NearLimitNS: 1, ExceededNS: 0}}
	if r.Summary.NearLimitNS != 1 {
		t.Errorf("expected 1")
	}
}
func TestQuotaUtilEntry1994(t *testing.T) {
	e := QuotaUtilEntry1994{Namespace: "prod", Hard: map[string]string{"pods": "100"}, Used: map[string]string{"pods": "80"}, MaxUtilPct: 80.0}
	if e.MaxUtilPct != 80.0 {
		t.Errorf("expected 80")
	}
}
func TestCPLoadSummary1994(t *testing.T) {
	s := CPLoadSummary1994{TotalServices: 50}
	if s.TotalServices != 50 {
		t.Errorf("expected 50")
	}
}
func TestVolAttachSummary1994(t *testing.T) {
	s := VolAttachSummary1994{AvgPerNode: 4.5}
	if s.AvgPerNode != 4.5 {
		t.Errorf("expected 4.5")
	}
}
