package dashboard

import "testing"

func TestFQDNCoverageResult2350(t *testing.T) {
	r := FQDNCoverageResult2350{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithFQDN = 2
	if r.Summary.WithFQDN > r.Summary.TotalPods {
		t.Errorf("fqdn > total")
	}
}
func TestEmptyResResult2350(t *testing.T) {
	r := EmptyResResult2350{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.WithoutRequests = 30
	if r.Summary.WithoutRequests > r.Summary.TotalContainers {
		t.Errorf("empty > total")
	}
}
func TestHCNodePortResult2350(t *testing.T) {
	r := HCNodePortResult2350{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.NodePortSvc = 5
	if r.Summary.NodePortSvc > r.Summary.TotalServices {
		t.Errorf("np > total")
	}
}
func TestDepUpdatedRepsResult2351(t *testing.T) {
	r := DepUpdatedRepsResult2351{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.TotalReps = 100
	r.Summary.TotalUpdated = 95
	if r.Summary.TotalUpdated > r.Summary.TotalReps {
		t.Errorf("upd > reps")
	}
}
func TestSTSCurrentRepsResult2351(t *testing.T) {
	r := STSCurrentRepsResult2351{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalReps = 15
	r.Summary.TotalReady = 14
	if r.Summary.TotalReady > r.Summary.TotalReps {
		t.Errorf("ready > reps")
	}
}
func TestRSFullStatusResult2351(t *testing.T) {
	r := RSFullStatusResult2351{HealthScore: 95}
	r.Summary.TotalRS = 20
	r.Summary.TotalReps = 80
	r.Summary.TotalReady = 78
	if r.Summary.TotalReady > r.Summary.TotalReps {
		t.Errorf("ready > reps")
	}
}
func TestWaitingReasonResult2352(t *testing.T) {
	r := WaitingReasonResult2352{HealthScore: 100}
	r.Summary.TotalWaiting = 5
	r.Summary.ByReason = map[string]int{"ContainerCreating": 3}
	if r.Summary.ByReason["ContainerCreating"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestNodeMemAllocResult2352(t *testing.T) {
	r := NodeMemAllocResult2352{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalAllocGB = 100.0
	if r.Summary.TotalAllocGB < 0 {
		t.Errorf("negative")
	}
}
func TestLimitCPUResult2352(t *testing.T) {
	r := LimitCPUResult2352{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalLimitCPU = 20.0
	if r.Summary.TotalLimitCPU < 0 {
		t.Errorf("negative")
	}
}
func TestSeccompLocalResult2353(t *testing.T) {
	r := SeccompLocalResult2353{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.Localhost = 0
	if r.Summary.Localhost > r.Summary.TotalPods {
		t.Errorf("local > total")
	}
}
func TestBasicAuthResult2353(t *testing.T) {
	r := BasicAuthResult2353{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.BasicAuth = 2
	if r.Summary.BasicAuth > r.Summary.TotalSecrets {
		t.Errorf("basic > total")
	}
}
func TestRoleResWildcardResult2353(t *testing.T) {
	r := RoleResWildcardResult2353{HealthScore: 90}
	r.Summary.TotalRoles = 30
	r.Summary.WildcardRes = 5
	if r.Summary.WildcardRes > r.Summary.TotalRoles {
		t.Errorf("wild > total")
	}
}
func TestInstanceTypeResult2354(t *testing.T) {
	r := InstanceTypeResult2354{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByInstance = map[string]int{"<unknown>": 5}
	if r.Summary.ByInstance["<unknown>"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestEnvFromCMResult2354(t *testing.T) {
	r := EnvFromCMResult2354{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithEnvFromCM = 30
	if r.Summary.WithEnvFromCM > r.Summary.TotalContainers {
		t.Errorf("env > total")
	}
}
func TestPVCSCNameResult2354(t *testing.T) {
	r := PVCSCNameResult2354{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.BySCName = map[string]int{"standard": 8}
	if r.Summary.BySCName["standard"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestTopNSCMResult2355(t *testing.T) {
	r := TopNSCMResult2355{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeCPUAllocResult2355(t *testing.T) {
	r := NodeCPUAllocResult2355{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCPU = 20
	r.Summary.AvgPerNode = 4
	if r.Summary.AvgPerNode > r.Summary.TotalCPU {
		t.Errorf("avg > total")
	}
}
func TestSTSDensityResult2355(t *testing.T) {
	r := STSDensityResult2355{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.ByNS = map[string]int{"default": 3}
	if r.Summary.ByNS["default"] != 3 {
		t.Errorf("expected 3")
	}
}
