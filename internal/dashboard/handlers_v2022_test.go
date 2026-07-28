package dashboard

import "testing"

func TestSCBindModeResult2022(t *testing.T) {
	r := SCBindModeResult2022{Summary: SCBindModeSummary2022{TotalClasses: 5, ImmediateBinding: 3, WaitForConsumer: 2}}
	if r.Summary.WaitForConsumer != 2 {
		t.Errorf("expected 2")
	}
}
func TestSCBindModeEntry2022(t *testing.T) {
	e := SCBindModeEntry2022{Name: "fast", Provisioner: "csi", BindingMode: "Immediate", IsDefault: true}
	if !e.IsDefault {
		t.Errorf("expected true")
	}
}
func TestCRDVerResult2022(t *testing.T) {
	r := CRDVerResult2022{Summary: CRDVerSummary2022{TotalCRDs: 10, WithMultiple: 3, TotalVersions: 15}}
	if r.Summary.WithMultiple != 3 {
		t.Errorf("expected 3")
	}
}
func TestCRDVerEntry2022(t *testing.T) {
	e := CRDVerEntry2022{Name: "foos.example.com", Group: "example.com", Versions: []string{"v1", "v2"}}
	if len(e.Versions) != 2 {
		t.Errorf("expected 2")
	}
}
func TestPLConfigResult2022(t *testing.T) {
	r := PLConfigResult2022{Summary: PLConfigSummary2022{TotalLevels: 8, Exempt: 1, LimitedQueue: 7}}
	if r.Summary.LimitedQueue != 7 {
		t.Errorf("expected 7")
	}
}
func TestPLConfigEntry2022(t *testing.T) {
	e := PLConfigEntry2022{Name: "system", Type: "Exempt"}
	if e.Type != "Exempt" {
		t.Errorf("expected Exempt")
	}
}
func TestSCBindModeSummary2022(t *testing.T) {
	s := SCBindModeSummary2022{HasDefault: true}
	if !s.HasDefault {
		t.Errorf("expected true")
	}
}
func TestCRDVerSummary2022(t *testing.T) {
	s := CRDVerSummary2022{TotalVersions: 15}
	if s.TotalVersions != 15 {
		t.Errorf("expected 15")
	}
}
