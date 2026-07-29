package dashboard

import "testing"

func TestCPUSatResult2069(t *testing.T) {
	r := CPUSatResult2069{Summary: CPUSatSummary2069{TotalContainers: 200, Saturated: 10, NoLimits: 50}}
	if r.Summary.Saturated != 10 {
		t.Errorf("expected 10")
	}
}
func TestMemWSetResult2069(t *testing.T) {
	r := MemWSetResult2069{Summary: MemWSetSummary2069{TotalPods: 100, TotalMemReq: 50.0, TotalMemLim: 100.0}}
	if r.Summary.TotalMemReq != 50.0 {
		t.Errorf("expected 50")
	}
}
func TestStartupPhaseResult2069(t *testing.T) {
	r := StartupPhaseResult2069{Summary: StartupPhaseSummary2069{TotalPods: 100, WithInit: 20, SlowStartup: 3}}
	if r.Summary.SlowStartup != 3 {
		t.Errorf("expected 3")
	}
}
func TestSecTypeResult2070(t *testing.T) {
	r := SecTypeResult2070{Summary: SecTypeSummary2070{TotalSecrets: 100}}
	if r.Summary.TotalSecrets != 100 {
		t.Errorf("expected 100")
	}
}
func TestSAPrivResult2070(t *testing.T) {
	r := SAPrivResult2070{Summary: SAPrivSummary2070{TotalSAs: 50, Privileged: 2}}
	if r.Summary.Privileged != 2 {
		t.Errorf("expected 2")
	}
}
func TestRunAsUserResult2070(t *testing.T) {
	r := RunAsUserResult2070{Summary: RunAsUserSummary2070{TotalContainers: 200, RunningAsRoot: 50}}
	if r.Summary.RunningAsRoot != 50 {
		t.Errorf("expected 50")
	}
}
func TestSCProvResult2071(t *testing.T) {
	r := SCProvResult2071{Summary: SCProvSummary2071{TotalSCs: 3, DefaultSCCount: 1}}
	if r.Summary.DefaultSCCount != 1 {
		t.Errorf("expected 1")
	}
}
func TestNSAnnotResult2071(t *testing.T) {
	r := NSAnnotResult2071{Summary: NSAnnotSummary2071{TotalNS: 20, WithContact: 5, MissingAnnot: 15}}
	if r.Summary.MissingAnnot != 15 {
		t.Errorf("expected 15")
	}
}
func TestCRDConvResult2071(t *testing.T) {
	r := CRDConvResult2071{Summary: CRDConvSummary2071{TotalCRDs: 15, NoneCRDs: 10, WebhookCRDs: 5}}
	if r.Summary.WebhookCRDs != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodBalResult2072(t *testing.T) {
	r := PodBalResult2072{Summary: PodBalSummary2072{TotalNodes: 5, TotalPods: 100, MaxPerNode: 40, MinPerNode: 5}}
	if r.Summary.MaxPerNode != 40 {
		t.Errorf("expected 40")
	}
}
func TestReqSatResult2072(t *testing.T) {
	r := ReqSatResult2072{Summary: ReqSatSummary2072{AllocatableCPU: 8, RequestedCPU: 6, CPUSaturation: 75}}
	if r.Summary.CPUSaturation != 75 {
		t.Errorf("expected 75")
	}
}
func TestVolAttachResult2072(t *testing.T) {
	r := VolAttachResult2072{Summary: VolAttachSummary2072{TotalPVCs: 30, AttachedPVCs: 25, DetachedPVCs: 5}}
	if r.Summary.DetachedPVCs != 5 {
		t.Errorf("expected 5")
	}
}
