package dashboard

import "testing"

func TestPriorityResult2374(t *testing.T) {
	r := PriorityResult2374{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByPriority = map[string]int{"<default>": 50}
	if r.Summary.ByPriority["<default>"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestReadinessExistResult2374(t *testing.T) {
	r := ReadinessExistResult2374{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.WithReadiness = 80
	if r.Summary.WithReadiness > r.Summary.TotalContainers {
		t.Errorf("ready > total")
	}
}
func TestTargetPortCustomResult2374(t *testing.T) {
	r := TargetPortCustomResult2374{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.TotalPorts = 50
	if r.Summary.TotalPorts < r.Summary.TotalServices {
		t.Errorf("ports < svcs")
	}
}
func TestMaxSurgeResult2375(t *testing.T) {
	r := MaxSurgeResult2375{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithCustom = 5
	if r.Summary.WithCustom > r.Summary.TotalDeploys {
		t.Errorf("custom > total")
	}
}
func TestSTSSvcNameEmptyResult2375(t *testing.T) {
	r := STSSvcNameEmptyResult2375{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.EmptyName = 1
	if r.Summary.EmptyName > r.Summary.TotalSTS {
		t.Errorf("empty > total")
	}
}
func TestCronJobSuspendResult2375(t *testing.T) {
	r := CronJobSuspendResult2375{HealthScore: 80}
	r.Summary.TotalCronJobs = 5
	r.Summary.Suspended = 1
	if r.Summary.Suspended > r.Summary.TotalCronJobs {
		t.Errorf("suspended > total")
	}
}
func TestQoSGuaranteedResult2376(t *testing.T) {
	r := QoSGuaranteedResult2376{HealthScore: 60}
	r.Summary.TotalPods = 100
	r.Summary.ByQoS = map[string]int{"Guaranteed": 20, "Burstable": 80}
	if r.Summary.ByQoS["Guaranteed"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestNodeKernelResult2376(t *testing.T) {
	r := NodeKernelResult2376{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByKernel = map[string]int{"6.1.0": 5}
	if r.Summary.ByKernel["6.1.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestEventReasonResult2376(t *testing.T) {
	r := EventReasonResult2376{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.ByReason = map[string]int{"Started": 100}
	if r.Summary.ByReason["Started"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestRunAsNonRootResult2377(t *testing.T) {
	r := RunAsNonRootResult2377{HealthScore: 40}
	r.Summary.TotalPods = 50
	r.Summary.NonRoot = 20
	if r.Summary.NonRoot > r.Summary.TotalPods {
		t.Errorf("nonroot > total")
	}
}
func TestSecretTypeCensusResult2377(t *testing.T) {
	r := SecretTypeCensusResult2377{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByType = map[string]int{"Opaque": 15}
	if r.Summary.ByType["Opaque"] != 15 {
		t.Errorf("expected 15")
	}
}
func TestRoleBindingKindResult2377(t *testing.T) {
	r := RoleBindingKindResult2377{HealthScore: 100}
	r.Summary.TotalBindings = 50
	r.Summary.ByRoleKind = map[string]int{"Role": 40}
	if r.Summary.ByRoleKind["Role"] != 40 {
		t.Errorf("expected 40")
	}
}
func TestNodeCRTVerResult2378(t *testing.T) {
	r := NodeCRTVerResult2378{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByCRVer = map[string]int{"containerd://1.7.0": 5}
	if r.Summary.ByCRVer["containerd://1.7.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodUIDResult2378(t *testing.T) {
	r := PodUIDResult2378{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithUID = 50
	if r.Summary.WithUID > r.Summary.TotalPods {
		t.Errorf("uid > total")
	}
}
func TestCMAgeResult2378(t *testing.T) {
	r := CMAgeResult2378{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.ByAgeBucket = map[string]int{"90d+": 20}
	if r.Summary.ByAgeBucket["90d+"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestTopNSReplicaResult2379(t *testing.T) {
	r := TopNSReplicaResult2379{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeMemCapResult2379(t *testing.T) {
	r := NodeMemCapResult2379{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCapGB = 100
	r.Summary.AvgPerNode = 20
	if r.Summary.AvgPerNode < 0 {
		t.Errorf("negative")
	}
}
func TestDSSpreadResult2379(t *testing.T) {
	r := DSSpreadResult2379{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.TotalDesired = 5
	r.Summary.TotalReady = 5
	if r.Summary.TotalReady > r.Summary.TotalDesired {
		t.Errorf("ready > desired")
	}
}
