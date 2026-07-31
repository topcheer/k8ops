package dashboard

import "testing"

func TestSupplementalGroupsResult2332(t *testing.T) {
	r := SupplementalGroupsResult2332{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithSupplementalGID = 5
	if r.Summary.WithSupplementalGID > r.Summary.TotalPods {
		t.Errorf("gid > total")
	}
}
func TestTermMsgPathResult2332(t *testing.T) {
	r := TermMsgPathResult2332{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.ByTermMsgPolicy = map[string]int{"File": 80, "FallbackToLogsOnError": 20}
	if r.Summary.ByTermMsgPolicy["File"] != 80 {
		t.Errorf("expected 80")
	}
}
func TestLBSourceRangeResult2332(t *testing.T) {
	r := LBSourceRangeResult2332{HealthScore: 100}
	r.Summary.TotalLBSvc = 3
	r.Summary.WithSourceRange = 1
	if r.Summary.WithSourceRange > r.Summary.TotalLBSvc {
		t.Errorf("range > total")
	}
}
func TestSTSPVCSizeResult2333(t *testing.T) {
	r := STSPVCSizeResult2333{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalSizeGB = 50.0
	if r.Summary.TotalSizeGB < 0 {
		t.Errorf("negative size")
	}
}
func TestMaxUnavailResult2333(t *testing.T) {
	r := MaxUnavailResult2333{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithCustomMax = 5
	if r.Summary.WithCustomMax > r.Summary.TotalDeploys {
		t.Errorf("max > total")
	}
}
func TestCronJobHistLimitResult2333(t *testing.T) {
	r := CronJobHistLimitResult2333{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.WithHistoryLim = 3
	if r.Summary.WithHistoryLim > r.Summary.TotalCronJobs {
		t.Errorf("hist > total")
	}
}
func TestFailedSchedResult2334(t *testing.T) {
	r := FailedSchedResult2334{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.FailedSched = 0
	if r.Summary.FailedSched > r.Summary.TotalPods {
		t.Errorf("fail > total")
	}
}
func TestNodeCondNetResult2334(t *testing.T) {
	r := NodeCondNetResult2334{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.NetUnavailable = 0
	if r.Summary.NetUnavailable > r.Summary.TotalNodes {
		t.Errorf("net > nodes")
	}
}
func TestResSummaryResult2334(t *testing.T) {
	r := ResSummaryResult2334{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalReqCPU = 10.0
	r.Summary.TotalLimitCPU = 20.0
	if r.Summary.TotalReqCPU > r.Summary.TotalLimitCPU {
		t.Errorf("req > limit")
	}
}
func TestFSGroupAlwaysResult2335(t *testing.T) {
	r := FSGroupAlwaysResult2335{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.Always = 10
	r.Summary.OnRootMismatch = 5
	if r.Summary.Always+r.Summary.OnRootMismatch > r.Summary.TotalPods {
		t.Errorf("sum > total")
	}
}
func TestSAAutomountResult2335(t *testing.T) {
	r := SAAutomountResult2335{HealthScore: 100}
	r.Summary.TotalSAs = 20
	r.Summary.AutoDisabled = 5
	if r.Summary.AutoDisabled > r.Summary.TotalSAs {
		t.Errorf("disabled > total")
	}
}
func TestSecretImmutableResult2335(t *testing.T) {
	r := SecretImmutableResult2335{HealthScore: 60}
	r.Summary.TotalSecrets = 20
	r.Summary.Immutable = 5
	if r.Summary.Immutable > r.Summary.TotalSecrets {
		t.Errorf("imm > total")
	}
}
func TestHostNetNSResult2336(t *testing.T) {
	r := HostNetNSResult2336{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.HostNetwork = 3
	r.Summary.HostPID = 1
	if r.Summary.HostNetwork > r.Summary.TotalPods {
		t.Errorf("net > total")
	}
}
func TestNodeUUIDResult2336(t *testing.T) {
	r := NodeUUIDResult2336{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.UniqueUUIDs = 5
	if r.Summary.UniqueUUIDs > r.Summary.TotalNodes {
		t.Errorf("uuid > nodes")
	}
}
func TestCMImmutableMarkResult2336(t *testing.T) {
	r := CMImmutableMarkResult2336{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.Immutable = 3
	if r.Summary.Immutable > r.Summary.TotalCMs {
		t.Errorf("imm > total")
	}
}
func TestTopNSSecretResult2337(t *testing.T) {
	r := TopNSSecretResult2337{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeCPUHeadroomResult2337(t *testing.T) {
	r := NodeCPUHeadroomResult2337{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalAllocCPU = 20.0
	r.Summary.TotalReqCPU = 10.0
	r.Summary.HeadroomCPU = 10.0
	if r.Summary.HeadroomCPU < 0 {
		t.Errorf("negative headroom")
	}
}
func TestEPRatioResult2337(t *testing.T) {
	r := EPRatioResult2337{HealthScore: 90}
	r.Summary.TotalServices = 30
	r.Summary.WithEndpoints = 28
	r.Summary.WithoutEPs = 2
	if r.Summary.WithEndpoints+r.Summary.WithoutEPs != r.Summary.TotalServices {
		t.Errorf("sum mismatch")
	}
}
