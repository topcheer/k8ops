package dashboard

import "testing"

func TestVolLifeResult1999(t *testing.T) {
	r := VolLifeResult1999{Summary: VolLifeSummary1999{TotalPVCs: 20, BoundPVCs: 18, OldPVCs: 5}}
	if r.Summary.OldPVCs != 5 {
		t.Errorf("expected 5")
	}
}
func TestVolLifeEntry1999(t *testing.T) {
	e := VolLifeEntry1999{Name: "data", Namespace: "prod", Status: "Bound", AgeDays: 120.0}
	if e.AgeDays != 120.0 {
		t.Errorf("expected 120")
	}
}
func TestSvcEPResult1999(t *testing.T) {
	r := SvcEPResult1999{Summary: SvcEPSummary1999{TotalServices: 50, WithEndpoints: 45, NoEndpoints: 5}}
	if r.Summary.NoEndpoints != 5 {
		t.Errorf("expected 5")
	}
}
func TestSvcEPEntry1999(t *testing.T) {
	e := SvcEPEntry1999{Name: "api", Namespace: "prod", Type: "ClusterIP", Issue: "no endpoints"}
	if e.Issue != "no endpoints" {
		t.Errorf("expected issue")
	}
}
func TestImgTagResult1999(t *testing.T) {
	r := ImgTagResult1999{Summary: ImgTagSummary1999{TotalImages: 30, UsingLatest: 5, UsingSHA: 3, UsingSemver: 20}}
	if r.Summary.UsingLatest != 5 {
		t.Errorf("expected 5")
	}
}
func TestImgTagEntry1999(t *testing.T) {
	e := ImgTagEntry1999{Image: "nginx:latest", Issue: "non-reproducible"}
	if e.Issue != "non-reproducible" {
		t.Errorf("expected issue")
	}
}
func TestIsSHADigest1999(t *testing.T) {
	if !isSHADigest1999("nginx@sha256:abc123") {
		t.Errorf("expected true for SHA")
	}
	if isSHADigest1999("nginx:1.25") {
		t.Errorf("expected false for tag")
	}
}
func TestGetTag1999(t *testing.T) {
	if getTag1999("nginx:1.25") != "1.25" {
		t.Errorf("expected 1.25")
	}
	if getTag1999("nginx") != "" {
		t.Errorf("expected empty")
	}
}
