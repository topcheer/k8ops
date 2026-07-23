package dashboard

import "testing"

func TestVolumeMountResult1931(t *testing.T) {
	r := VolumeMountResult1931{Summary: VolumeMountSummary1931{TotalPods: 80, WithHostPath: 5, WithHostNet: 2, RiskyMountCount: 7}}
	if r.Summary.WithHostPath != 5 {
		t.Errorf("expected 5")
	}
}

func TestRiskyMountEntry1931(t *testing.T) {
	e := RiskyMountEntry1931{PodName: "web", HostPath: "/etc", Severity: "critical"}
	if e.Severity != "critical" {
		t.Errorf("expected critical")
	}
}

func TestVolumeTypeStat1931(t *testing.T) {
	s := VolumeTypeStat1931{Type: "hostPath", Count: 10}
	if s.Count != 10 {
		t.Errorf("expected 10")
	}
}

func TestPrivEscResult1931(t *testing.T) {
	r := PrivEscResult1931{Summary: PrivEscSummary1931{TotalContainers: 100, RunAsRoot: 30, AllowPrivEsc: 5, PrivilegedMode: 2}}
	if r.Summary.PrivilegedMode != 2 {
		t.Errorf("expected 2")
	}
}

func TestPrivEscEntry1931(t *testing.T) {
	e := PrivEscEntry1931{RiskType: "privileged", Severity: "critical"}
	if e.Severity != "critical" {
		t.Errorf("expected critical")
	}
}

func TestImageBaseScanResult1931(t *testing.T) {
	r := ImageBaseScanResult1931{Summary: ImageBaseScanSummary1931{TotalImages: 50, DistrolessCount: 10, LatestTagCount: 15}}
	if r.Summary.DistrolessCount != 10 {
		t.Errorf("expected 10")
	}
}

func TestImageBaseEntry1931(t *testing.T) {
	e := ImageBaseEntry1931{Image: "nginx:1.25-alpine", BaseImage: "alpine", IsSlim: true}
	if !e.IsSlim {
		t.Errorf("expected slim")
	}
}

func TestImageBaseRisk1931(t *testing.T) {
	r := ImageBaseRisk1931{RiskType: "latest-tag", Severity: "medium"}
	if r.RiskType != "latest-tag" {
		t.Errorf("expected latest-tag")
	}
}
