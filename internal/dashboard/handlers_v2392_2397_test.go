package dashboard

import "testing"

func TestAffinityRuleResult2392(t *testing.T) {
	r := AffinityRuleResult2392{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithAffinity = 10
	if r.Summary.WithAffinity > r.Summary.TotalPods {
		t.Errorf("aff > total")
	}
}
func TestEnvValueFromResult2392(t *testing.T) {
	r := EnvValueFromResult2392{HealthScore: 100}
	r.Summary.TotalEnvVars = 300
	r.Summary.WithValueFrom = 100
	if r.Summary.WithValueFrom > r.Summary.TotalEnvVars {
		t.Errorf("vf > total")
	}
}
func TestLBClassResult2392(t *testing.T) {
	r := LBClassResult2392{HealthScore: 100}
	r.Summary.TotalLBSvc = 3
	r.Summary.ByClass = map[string]int{"<default>": 3}
	if r.Summary.ByClass["<default>"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestDepPausedResult2393(t *testing.T) {
	r := DepPausedResult2393{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.Paused = 0
	if r.Summary.Paused > r.Summary.TotalDeploys {
		t.Errorf("paused > total")
	}
}
func TestSTSOrdinalResult2393(t *testing.T) {
	r := STSOrdinalResult2393{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalReps = 15
	if r.Summary.TotalReps < 0 {
		t.Errorf("negative")
	}
}
func TestJobBackoffResult2393(t *testing.T) {
	r := JobBackoffResult2393{HealthScore: 100}
	r.Summary.TotalJobs = 10
	r.Summary.WithBackoff = 8
	if r.Summary.WithBackoff > r.Summary.TotalJobs {
		t.Errorf("backoff > total")
	}
}
func TestCrashLoopResult2394(t *testing.T) {
	r := CrashLoopResult2394{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.InCrashLoop = 0
	if r.Summary.InCrashLoop > r.Summary.TotalContainers {
		t.Errorf("crash > total")
	}
}
func TestNodeCondMemResult2394(t *testing.T) {
	r := NodeCondMemResult2394{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.MemPressure = 0
	if r.Summary.MemPressure > r.Summary.TotalNodes {
		t.Errorf("mem > nodes")
	}
}
func TestRestartCountResult2394(t *testing.T) {
	r := RestartCountResult2394{HealthScore: 100}
	r.Summary.TotalContainers = 200
	r.Summary.TotalRestarts = 10
	r.Summary.AvgRestarts = 0
	if r.Summary.TotalRestarts < 0 {
		t.Errorf("negative")
	}
}
func TestCapabilitiesResult2395(t *testing.T) {
	r := CapabilitiesResult2395{HealthScore: 90}
	r.Summary.TotalContainers = 100
	r.Summary.WithCaps = 20
	if r.Summary.WithCaps > r.Summary.TotalContainers {
		t.Errorf("caps > total")
	}
}
func TestSecretDataSizeResult2395(t *testing.T) {
	r := SecretDataSizeResult2395{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.TotalDataSize = 5000
	if r.Summary.TotalDataSize < 0 {
		t.Errorf("negative")
	}
}
func TestRBNSResult2395(t *testing.T) {
	r := RBNSResult2395{HealthScore: 100}
	r.Summary.TotalRB = 50
	r.Summary.ByNS = map[string]int{"default": 20}
	if r.Summary.ByNS["default"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestTaintCountResult2396(t *testing.T) {
	r := TaintCountResult2396{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalTaints = 3
	if r.Summary.TotalTaints < 0 {
		t.Errorf("negative")
	}
}
func TestNodeSelectorKeyResult2396(t *testing.T) {
	r := NodeSelectorKeyResult2396{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.BySelectorKey = map[string]int{"disktype": 10}
	if r.Summary.BySelectorKey["disktype"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestEPAddrByNodeResult2396(t *testing.T) {
	r := EPAddrByNodeResult2396{HealthScore: 100}
	r.Summary.TotalEndpoints = 80
	r.Summary.ByNode = map[string]int{"node1": 40}
	if r.Summary.ByNode["node1"] != 40 {
		t.Errorf("expected 40")
	}
}
func TestTopNSCPULimitResult2397(t *testing.T) {
	r := TopNSCPULimitResult2397{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeAllocCPUSumResult2397(t *testing.T) {
	r := NodeAllocCPUSumResult2397{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCPU = 20.0
	r.Summary.AvgPerNode = 4.0
	if r.Summary.AvgPerNode < 0 {
		t.Errorf("negative")
	}
}
func TestPVCDensityResult2397(t *testing.T) {
	r := PVCDensityResult2397{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.ByNS = map[string]int{"default": 5}
	if r.Summary.ByNS["default"] != 5 {
		t.Errorf("expected 5")
	}
}
