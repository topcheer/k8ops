package dashboard

import "testing"

func TestRCInvResult2004(t *testing.T) {
	r := RCInvResult2004{Summary: RCInvSummary2004{TotalClasses: 3, WithOverhead: 1, WithScheduling: 2}}
	if r.Summary.WithScheduling != 2 {
		t.Errorf("expected 2")
	}
}
func TestRCInvEntry2004(t *testing.T) {
	e := RCInvEntry2004{Name: "kata", Handler: "kata-runtime", HasOverhead: true, HasSched: true}
	if !e.HasOverhead {
		t.Errorf("expected true")
	}
}
func TestIngBackendResult2004(t *testing.T) {
	r := IngBackendResult2004{Summary: IngBackendSummary2004{TotalIngresses: 10, WithTLS: 5, TotalBackends: 15}}
	if r.Summary.TotalBackends != 15 {
		t.Errorf("expected 15")
	}
}
func TestIngBackendEntry2004(t *testing.T) {
	e := IngBackendEntry2004{Name: "api-ing", Namespace: "prod", Class: "nginx", Hosts: []string{"api.com"}, BackendCount: 2, HasTLS: true}
	if !e.HasTLS {
		t.Errorf("expected true")
	}
}
func TestCSIDriverResult2004(t *testing.T) {
	r := CSIDriverResult2004{Summary: CSIDriverSummary2004{TotalDrivers: 2, WithAttachRequired: 2}}
	if r.Summary.WithAttachRequired != 2 {
		t.Errorf("expected 2")
	}
}
func TestCSIDriverEntry2004(t *testing.T) {
	t1 := true
	e := CSIDriverEntry2004{Name: "disk.csi.aws.com", AttachRequired: &t1, PodInfoOnMount: &t1, StorageCapacity: true}
	if !*e.AttachRequired {
		t.Errorf("expected true")
	}
}
func TestRCInvSummary2004(t *testing.T) {
	s := RCInvSummary2004{TotalClasses: 3}
	if s.TotalClasses != 3 {
		t.Errorf("expected 3")
	}
}
func TestIngBackendSummary2004(t *testing.T) {
	s := IngBackendSummary2004{WithRules: 8}
	if s.WithRules != 8 {
		t.Errorf("expected 8")
	}
}
