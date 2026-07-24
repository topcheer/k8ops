package dashboard

import "testing"

func TestScaleLimitResult1957(t *testing.T) {
	r := ScaleLimitResult1957{Summary: ScaleLimitSummary1957{TotalDeployments: 66, WithHPA: 3, WithoutHPA: 63}}
	if r.Summary.WithoutHPA != 63 {
		t.Errorf("expected 63")
	}
}
func TestScaleLimitEntry1957(t *testing.T) {
	e := ScaleLimitEntry1957{Name: "api", Replicas: 3, Reason: "no HPA"}
	if e.Replicas != 3 {
		t.Errorf("expected 3")
	}
}
func TestCMKeyExposureResult1957(t *testing.T) {
	r := CMKeyExposureResult1957{Summary: CMKeyExposureSummary1957{TotalCMs: 52, SensitiveKeys: 5}}
	if r.Summary.SensitiveKeys != 5 {
		t.Errorf("expected 5")
	}
}
func TestCMKeyExposureEntry1957(t *testing.T) {
	e := CMKeyExposureEntry1957{Key: "API_KEY", Reason: "sensitive"}
	if e.Key != "API_KEY" {
		t.Errorf("expected API_KEY")
	}
}
func TestPVCAccessResult1957(t *testing.T) {
	r := PVCAccessResult1957{Summary: PVCAccessSummary1957{TotalPVCs: 15, RWOPVCs: 14, RWXPVCs: 1}}
	if r.Summary.RWXPVCs != 1 {
		t.Errorf("expected 1")
	}
}
func TestPVCAccessEntry1957(t *testing.T) {
	e := PVCAccessEntry1957{Name: "data", AccessMode: "ReadWriteOnce", MountCount: 1}
	if e.MountCount != 1 {
		t.Errorf("expected 1")
	}
}
func TestScaleLimitHPAEntry1957(t *testing.T) {
	e := ScaleLimitHPAEntry1957{Name: "api-hpa", MinReplicas: 2, MaxReplicas: 10}
	if e.MaxReplicas != 10 {
		t.Errorf("expected 10")
	}
}
func TestPVCAccessNS1957(t *testing.T) {
	e := PVCAccessNS1957{Namespace: "prod", PVCCount: 5}
	if e.PVCCount != 5 {
		t.Errorf("expected 5")
	}
}
