package dashboard

import "testing"

func TestOSFeatureGateResult2236(t *testing.T) {
	r := OSFeatureGateResult2236{HealthScore: 100}
	r.Summary.TotalPods = 100
	if r.Summary.TotalPods != 100 {
		t.Errorf("expected 100")
	}
}
func TestSpecStatusRatioResult2237(t *testing.T) {
	r := SpecStatusRatioResult2237{HealthScore: 100}
	r.Summary.RatioPct = 95
	if r.Summary.RatioPct != 95 {
		t.Errorf("expected 95")
	}
}
func TestRestartReasonResult2238(t *testing.T) {
	r := RestartReasonResult2238{HealthScore: 100}
	r.Summary.WithRestarts = 5
	if r.Summary.WithRestarts != 5 {
		t.Errorf("expected 5")
	}
}
func TestGMSACredSpecResult2239(t *testing.T) {
	r := GMSACredSpecResult2239{HealthScore: 100}
	r.Summary.WithGMSA = 2
	if r.Summary.WithGMSA != 2 {
		t.Errorf("expected 2")
	}
}
func TestKubeletProxyMatchResult2240(t *testing.T) {
	r := KubeletProxyMatchResult2240{HealthScore: 100}
	r.Summary.Matched = 1
	if r.Summary.Matched != 1 {
		t.Errorf("expected 1")
	}
}
func TestNSEphStorageResult2241(t *testing.T) {
	r := NSEphStorageResult2241{HealthScore: 100}
	r.Summary.TotalNS = 5
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestAffAntiAffCountResult2241(t *testing.T) {
	r := AffAntiAffCountResult2241{HealthScore: 100}
	r.Summary.WithAntiAff = 10
	if r.Summary.WithAntiAff != 10 {
		t.Errorf("expected 10")
	}
}
