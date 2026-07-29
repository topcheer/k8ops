package dashboard

import "testing"

func TestRootFSResult2046(t *testing.T) {
	r := RootFSResult2046{Summary: RootFSSummary2046{TotalContainers: 100, ReadOnlyRootFS: 30, WritableRootFS: 70}}
	if r.Summary.WritableRootFS != 70 {
		t.Errorf("expected 70")
	}
}
func TestRootFSEntry2046(t *testing.T) {
	e := RootFSEntry2046{Pod: "app", Namespace: "prod", Container: "web"}
	if e.Container != "web" {
		t.Errorf("expected web")
	}
}
func TestHostPathResult2046(t *testing.T) {
	r := HostPathResult2046{Summary: HostPathSummary2046{TotalPods: 50, PodsWithHostPath: 3, TotalMounts: 5}}
	if r.Summary.PodsWithHostPath != 3 {
		t.Errorf("expected 3")
	}
}
func TestHostPathEntry2046(t *testing.T) {
	e := HostPathEntry2046{Pod: "app", Namespace: "prod", HostPath: "/data", MountPath: "/app/data"}
	if e.HostPath != "/data" {
		t.Errorf("expected /data")
	}
}
func TestTokenRotResult2046(t *testing.T) {
	r := TokenRotResult2046{Summary: TokenRotSummary2046{TotalTokens: 30, OldTokens: 10, ManuallyCreated: 5}}
	if r.Summary.OldTokens != 10 {
		t.Errorf("expected 10")
	}
}
func TestTokenRotEntry2046(t *testing.T) {
	e := TokenRotEntry2046{Name: "sa-token", Namespace: "prod", AgeDays: 120}
	if e.AgeDays != 120 {
		t.Errorf("expected 120")
	}
}
