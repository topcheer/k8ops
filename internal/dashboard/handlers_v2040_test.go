package dashboard

import "testing"

func TestSATokenMountResult2040(t *testing.T) {
	r := SATokenMountResult2040{Summary: SATokenMountSummary2040{TotalPods: 100, AutoMountTrue: 80, AutoMountFalse: 20, WithDefaultSA: 60}}
	if r.Summary.WithDefaultSA != 60 {
		t.Errorf("expected 60")
	}
}
func TestSATokenMountEntry2040(t *testing.T) {
	e := SATokenMountEntry2040{Pod: "app", Namespace: "prod", ServiceAccount: "default"}
	if e.ServiceAccount != "default" {
		t.Errorf("expected default")
	}
}
func TestCRBindingResult2040(t *testing.T) {
	r := CRBindingResult2040{Summary: CRBindingSummary2040{TotalCRBs: 30, TotalSubjects: 200, BloatedBindings: 5}}
	if r.Summary.BloatedBindings != 5 {
		t.Errorf("expected 5")
	}
}
func TestCRBindingEntry2040(t *testing.T) {
	e := CRBindingEntry2040{Name: "admin-binding", Subjects: 15, RoleRef: "cluster-admin"}
	if e.Subjects != 15 {
		t.Errorf("expected 15")
	}
}
func TestPortExposureResult2040(t *testing.T) {
	r := PortExposureResult2040{Summary: PortExposureSummary2040{TotalContainers: 200, WithPorts: 150, PrivilegedPorts: 20, HostPorts: 5}}
	if r.Summary.HostPorts != 5 {
		t.Errorf("expected 5")
	}
}
func TestPortExposureEntry2040(t *testing.T) {
	e := PortExposureEntry2040{Pod: "app", Namespace: "prod", Port: 8080, HostPort: 80}
	if e.HostPort != 80 {
		t.Errorf("expected 80")
	}
}
