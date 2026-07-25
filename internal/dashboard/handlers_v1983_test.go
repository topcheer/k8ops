package dashboard

import "testing"

func TestNodeSelResult1983(t *testing.T) {
	r := NodeSelResult1983{Summary: NodeSelSummary1983{TotalPods: 80, WithNodeSelector: 20, WithoutSelector: 60}}
	if r.Summary.WithoutSelector != 60 {
		t.Errorf("expected 60")
	}
}
func TestNodeSelEntry1983(t *testing.T) {
	e := NodeSelEntry1983{Pod: "app", Namespace: "prod", Selector: map[string]string{"disktype": "ssd"}, HasAffinity: false, TolerationCount: 1}
	if e.Selector["disktype"] != "ssd" {
		t.Errorf("expected ssd")
	}
}
func TestPodOSResult1983(t *testing.T) {
	r := PodOSResult1983{Summary: PodOSSummary1983{TotalPods: 50, WithOSSelector: 5, LinuxPods: 5, NoOSSpecified: 45}}
	if r.Summary.NoOSSpecified != 45 {
		t.Errorf("expected 45")
	}
}
func TestPodOSEntry1983(t *testing.T) {
	e := PodOSEntry1983{Pod: "app", Namespace: "prod", OS: "linux"}
	if e.OS != "linux" {
		t.Errorf("expected linux")
	}
}
func TestWorkDirResult1983(t *testing.T) {
	r := WorkDirResult1983{Summary: WorkDirSummary1983{TotalContainers: 100, WithWorkDir: 30, UsingRoot: 5}}
	if r.Summary.UsingRoot != 5 {
		t.Errorf("expected 5")
	}
}
func TestWorkDirEntry1983(t *testing.T) {
	e := WorkDirEntry1983{Pod: "app", Namespace: "prod", Container: "web", WorkDir: "/app"}
	if e.WorkDir != "/app" {
		t.Errorf("expected /app")
	}
}
func TestNodeSelSummary1983(t *testing.T) {
	s := NodeSelSummary1983{WithNodeAffinity: 10, WithToleration: 8}
	if s.WithToleration != 8 {
		t.Errorf("expected 8")
	}
}
func TestWorkDirSummary1983(t *testing.T) {
	s := WorkDirSummary1983{NonStandard: 12}
	if s.NonStandard != 12 {
		t.Errorf("expected 12")
	}
}
