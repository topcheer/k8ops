package dashboard

import "testing"

func TestHostPathResult1997(t *testing.T) {
	r := HostPathResult1997{Summary: HostPathSummary1997{TotalPods: 80, WithHostPath: 5, WritableMounts: 3}}
	if r.Summary.WithHostPath != 5 {
		t.Errorf("expected 5")
	}
}
func TestHostPathEntry1997(t *testing.T) {
	e := HostPathEntry1997{Pod: "app", Namespace: "prod", HostPath: "/data", MountPath: "/mnt", ReadOnly: false}
	if e.HostPath != "/data" {
		t.Errorf("expected /data")
	}
}
func TestReadOnlyFSResult1997(t *testing.T) {
	r := ReadOnlyFSResult1997{Summary: ReadOnlyFSSummary1997{TotalContainers: 50, ReadOnlyRootFS: 10, WritableRootFS: 40}}
	if r.Summary.WritableRootFS != 40 {
		t.Errorf("expected 40")
	}
}
func TestReadOnlyFSEntry1997(t *testing.T) {
	e := ReadOnlyFSEntry1997{Pod: "app", Namespace: "prod", Container: "web"}
	if e.Container != "web" {
		t.Errorf("expected web")
	}
}
func TestSATokenAgeResult1997(t *testing.T) {
	r := SATokenAgeResult1997{Summary: SATokenAgeSummary1997{TotalSAs: 30, WithAutoMount: 25, OldSAs: 10}}
	if r.Summary.OldSAs != 10 {
		t.Errorf("expected 10")
	}
}
func TestSATokenAgeEntry1997(t *testing.T) {
	e := SATokenAgeEntry1997{Name: "app-sa", Namespace: "prod", AgeDays: 120.5}
	if e.AgeDays != 120.5 {
		t.Errorf("expected 120.5")
	}
}
func TestHostPathSummary1997(t *testing.T) {
	s := HostPathSummary1997{TotalMounts: 8}
	if s.TotalMounts != 8 {
		t.Errorf("expected 8")
	}
}
func TestSATokenAgeSummary1997(t *testing.T) {
	s := SATokenAgeSummary1997{AvgAgeDays: 60.0}
	if s.AvgAgeDays != 60.0 {
		t.Errorf("expected 60")
	}
}
