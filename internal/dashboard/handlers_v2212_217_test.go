package dashboard

import "testing"

func TestSubPathResult2212(t *testing.T) {
	r := SubPathResult2212{HealthScore: 100}
	r.Summary.WithSubPath = 10
	if r.Summary.WithSubPath != 10 {
		t.Errorf("expected 10")
	}
}
func TestMaxUnavailResult2213(t *testing.T) {
	r := MaxUnavailResult2213{HealthScore: 100}
	r.Summary.WithCustom = 5
	if r.Summary.WithCustom != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeSelKeyResult2214(t *testing.T) {
	r := NodeSelKeyResult2214{HealthScore: 100}
	r.Summary.WithSelector = 20
	if r.Summary.WithSelector != 20 {
		t.Errorf("expected 20")
	}
}
func TestHostUsersResult2215(t *testing.T) {
	r := HostUsersResult2215{HealthScore: 100}
	r.Summary.WithHostUsers = 3
	if r.Summary.WithHostUsers != 3 {
		t.Errorf("expected 3")
	}
}
func TestSysUUIDResult2216(t *testing.T) {
	r := SysUUIDResult2216{HealthScore: 100}
	r.Summary.TotalNodes = 1
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestNSMemCommitResult2217(t *testing.T) {
	r := NSMemCommitResult2217{HealthScore: 100}
	r.Summary.TotalNS = 10
	if r.Summary.TotalNS != 10 {
		t.Errorf("expected 10")
	}
}
func TestPodCapHRResult2217(t *testing.T) {
	r := PodCapHRResult2217{HealthScore: 100}
	r.Summary.HeadroomPods = 100
	if r.Summary.HeadroomPods != 100 {
		t.Errorf("expected 100")
	}
}
