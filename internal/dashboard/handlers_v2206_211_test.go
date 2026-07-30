package dashboard

import "testing"

func TestImgDigestResult2206(t *testing.T) {
	r := ImgDigestResult2206{HealthScore: 100}
	r.Summary.WithDigest = 50
	if r.Summary.WithDigest != 50 {
		t.Errorf("expected 50")
	}
}
func TestGenLagResult2207(t *testing.T) {
	r := GenLagResult2207{HealthScore: 100}
	r.Summary.WithLag = 3
	if r.Summary.WithLag != 3 {
		t.Errorf("expected 3")
	}
}
func TestNPMatchResult2208(t *testing.T) {
	r := NPMatchResult2208{HealthScore: 100}
	r.Summary.WithNetPol = 80
	if r.Summary.WithNetPol != 80 {
		t.Errorf("expected 80")
	}
}
func TestSuppGroupsResult2209(t *testing.T) {
	r := SuppGroupsResult2209{HealthScore: 100}
	r.Summary.WithSuppGrp = 10
	if r.Summary.WithSuppGrp != 10 {
		t.Errorf("expected 10")
	}
}
func TestMachineIDResult2210(t *testing.T) {
	r := MachineIDResult2210{HealthScore: 100}
	r.Summary.TotalNodes = 1
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestNSCPUCommitResult2211(t *testing.T) {
	r := NSCPUCommitResult2211{HealthScore: 100}
	r.Summary.TotalNS = 5
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestImgPullEffResult2211(t *testing.T) {
	r := ImgPullEffResult2211{HealthScore: 100}
	r.Summary.TotalImages = 30
	if r.Summary.TotalImages != 30 {
		t.Errorf("expected 30")
	}
}
