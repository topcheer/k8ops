package dashboard

import "testing"

func TestPodOSResult2314(t *testing.T) {
	r := PodOSResult2314{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByOS = map[string]int{"<default-linux>": 50}
	if r.Summary.ByOS["<default-linux>"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestResizePolicyResult2314(t *testing.T) {
	r := ResizePolicyResult2314{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithResizePolicy = 5
	if r.Summary.WithResizePolicy > r.Summary.TotalContainers {
		t.Errorf("resize > total")
	}
}
func TestPubNotReadyResult2314(t *testing.T) {
	r := PubNotReadyResult2314{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.PubNotReady = 3
	if r.Summary.PubNotReady > r.Summary.TotalServices {
		t.Errorf("pub > total")
	}
}
func TestDepAvailRatioResult2315(t *testing.T) {
	r := DepAvailRatioResult2315{HealthScore: 95}
	r.Summary.TotalDeploys = 30
	r.Summary.TotalReps = 100
	r.Summary.TotalAvail = 95
	r.Summary.TotalUnavail = 5
	if r.Summary.TotalAvail+r.Summary.TotalUnavail > r.Summary.TotalReps {
		t.Errorf("sum > total")
	}
}
func TestSTSGenSyncResult2315(t *testing.T) {
	r := STSGenSyncResult2315{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.InSync = 5
	if r.Summary.InSync > r.Summary.TotalSTS {
		t.Errorf("sync > total")
	}
}
func TestDSNumAvailResult2315(t *testing.T) {
	r := DSNumAvailResult2315{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.DesiredNum = 5
	r.Summary.AvailableNum = 5
	if r.Summary.AvailableNum > r.Summary.DesiredNum {
		t.Errorf("avail > desired")
	}
}
func TestImgPullBackOffResult2316(t *testing.T) {
	r := ImgPullBackOffResult2316{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.InBackOff = 0
	if r.Summary.InBackOff > r.Summary.TotalPods {
		t.Errorf("backOff > total")
	}
}
func TestNodeReadyTransResult2316(t *testing.T) {
	r := NodeReadyTransResult2316{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.Ready = 5
	r.Summary.NotReady = 0
	if r.Summary.Ready+r.Summary.NotReady != r.Summary.TotalNodes {
		t.Errorf("sum mismatch")
	}
}
func TestEventWarnRateResult2316(t *testing.T) {
	r := EventWarnRateResult2316{HealthScore: 70}
	r.Summary.TotalEvents = 200
	r.Summary.Warnings = 70
	r.Summary.WarningPct = 35
	if r.Summary.WarningPct > 100 {
		t.Errorf("pct > 100")
	}
}
func TestProcMountResult2317(t *testing.T) {
	r := ProcMountResult2317{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.ByProcMount = map[string]int{"<default>": 95, "Unmasked": 5}
	if r.Summary.ByProcMount["<default>"] != 95 {
		t.Errorf("expected 95")
	}
}
func TestPVSecCtxResult2317(t *testing.T) {
	r := PVSecCtxResult2317{HealthScore: 100}
	r.Summary.TotalPVs = 10
	r.Summary.WithSecCtx = 8
	if r.Summary.WithSecCtx > r.Summary.TotalPVs {
		t.Errorf("secCtx > total")
	}
}
func TestNSDelGuardResult2317(t *testing.T) {
	r := NSDelGuardResult2317{HealthScore: 100}
	r.Summary.TotalNS = 10
	r.Summary.ActiveNS = 10
	r.Summary.TerminatingNS = 0
	if r.Summary.ActiveNS+r.Summary.TerminatingNS > r.Summary.TotalNS {
		t.Errorf("sum > total")
	}
}
func TestSecretAgeResult2318(t *testing.T) {
	r := SecretAgeResult2318{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByAgeBucket = map[string]int{"365d+": 10, "<7d": 5}
	if r.Summary.ByAgeBucket["365d+"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestPVCFinResult2318(t *testing.T) {
	r := PVCFinResult2318{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.WithFinalizer = 2
	if r.Summary.WithFinalizer > r.Summary.TotalPVCs {
		t.Errorf("finalizer > total")
	}
}
func TestNodeMachineIDResult2318(t *testing.T) {
	r := NodeMachineIDResult2318{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.UniqueMachineIDs = 5
	if r.Summary.UniqueMachineIDs > r.Summary.TotalNodes {
		t.Errorf("unique > total")
	}
}
func TestTopNSCPUResult2319(t *testing.T) {
	r := TopNSCPUResult2319{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodePodBalanceResult2319(t *testing.T) {
	r := NodePodBalanceResult2319{HealthScore: 85}
	r.Summary.TotalNodes = 5
	r.Summary.AvgPods = 20
	r.Summary.StdDevPods = 5
	if r.Summary.AvgPods < 0 {
		t.Errorf("negative avg")
	}
}
func TestSvcEPDensityResult2319(t *testing.T) {
	r := SvcEPDensityResult2319{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.TotalEPs = 80
	r.Summary.AvgPerSvc = 2
	if r.Summary.AvgPerSvc != 2 {
		t.Errorf("expected 2")
	}
}
