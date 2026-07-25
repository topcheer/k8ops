package dashboard

import "testing"

func TestRunAsNonRootResult1967(t *testing.T) {
	r := RunAsNonRootResult1967{Summary: RunAsNonRootSummary1967{TotalContainers: 100, RunAsRoot: 30, WithRunAsNonRoot: 60}}
	if r.Summary.RunAsRoot != 30 {
		t.Errorf("expected 30")
	}
}
func TestRunAsNonRootEntry1967(t *testing.T) {
	e := RunAsNonRootEntry1967{Container: "app", Pod: "app-1", Namespace: "prod", RunAsUser: 0, IsRoot: true}
	if !e.IsRoot {
		t.Errorf("expected true")
	}
	if e.RunAsUser != 0 {
		t.Errorf("expected 0")
	}
}
func TestHostPIDIPCResult1967(t *testing.T) {
	r := HostPIDIPCResult1967{Summary: HostPIDIPCSummary1967{TotalPods: 50, HostPIDPods: 3, HostIPCPods: 1, HostNetworkPods: 5}}
	if r.Summary.HostPIDPods != 3 {
		t.Errorf("expected 3")
	}
}
func TestHostPIDIPCEntry1967(t *testing.T) {
	e := HostPIDIPCEntry1967{Name: "daemon", Namespace: "kube-system", Issue: "hostPID: true", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestImageDigestResult1967(t *testing.T) {
	r := ImageDigestResult1967{Summary: ImageDigestSummary1967{TotalImages: 40, PinnedByDigest: 5, UsingLatest: 15}}
	if r.Summary.UsingLatest != 15 {
		t.Errorf("expected 15")
	}
}
func TestImageDigestEntry1967(t *testing.T) {
	e := ImageDigestEntry1967{Image: "nginx:1.25", IsDigest: false, IsLatest: false, Tag: "1.25", UseCount: 3}
	if e.UseCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestImageDigestEntry1967Digest(t *testing.T) {
	e := ImageDigestEntry1967{Image: "nginx@sha256:abc123", IsDigest: true, Tag: "digest"}
	if !e.IsDigest {
		t.Errorf("expected true")
	}
}
func TestRunAsNonRootSummary1967(t *testing.T) {
	s := RunAsNonRootSummary1967{NoSecurityCtx: 20, WithFSGroup: 15}
	if s.NoSecurityCtx != 20 {
		t.Errorf("expected 20")
	}
}
