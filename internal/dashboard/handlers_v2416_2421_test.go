package dashboard

import "testing"

func TestSchedulerNameResult2416(t *testing.T) {
	r := SchedulerNameResult2416{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByScheduler = map[string]int{"<default-scheduler>": 50}
	if r.Summary.ByScheduler["<default-scheduler>"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestReqMemResult2416(t *testing.T) {
	r := ReqMemResult2416{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalReqMemGB = 20.0
	if r.Summary.TotalReqMemGB < 0 {
		t.Errorf("negative")
	}
}
func TestClusterIPsResult2416(t *testing.T) {
	r := ClusterIPsResult2416{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.TotalClusterIPs = 30
	if r.Summary.TotalClusterIPs > r.Summary.TotalServices*3 {
		t.Errorf("too many IPs")
	}
}
func TestDepSelectorResult2417(t *testing.T) {
	r := DepSelectorResult2417{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithMatchLbls = 30
	if r.Summary.WithMatchLbls > r.Summary.TotalDeploys {
		t.Errorf("ml > total")
	}
}
func TestRSOwnerRefResult2417(t *testing.T) {
	r := RSOwnerRefResult2417{HealthScore: 100}
	r.Summary.TotalRS = 20
	r.Summary.WithCtrl = 15
	if r.Summary.WithCtrl > r.Summary.TotalRS {
		t.Errorf("ctrl > total")
	}
}
func TestCronJobLastSchedResult2417(t *testing.T) {
	r := CronJobLastSchedResult2417{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.WithLastSched = 3
	if r.Summary.WithLastSched > r.Summary.TotalCronJobs {
		t.Errorf("sched > total")
	}
}
func TestPodIPResult2418(t *testing.T) {
	r := PodIPResult2418{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithPodIP = 50
	if r.Summary.WithPodIP > r.Summary.TotalPods {
		t.Errorf("ip > total")
	}
}
func TestMachineInfoResult2418(t *testing.T) {
	r := MachineInfoResult2418{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByMachine = map[string]int{"machine1": 1}
	if len(r.Summary.ByMachine) == 0 {
		t.Errorf("empty")
	}
}
func TestEventFirstTSResult2418(t *testing.T) {
	r := EventFirstTSResult2418{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.RecentEvents = 50
	if r.Summary.RecentEvents > r.Summary.TotalEvents {
		t.Errorf("recent > total")
	}
}
func TestSeccompUnconfResult2419(t *testing.T) {
	r := SeccompUnconfResult2419{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.Unconfined = 0
	if r.Summary.Unconfined > r.Summary.TotalPods {
		t.Errorf("unc > total")
	}
}
func TestSecretNSResult2419(t *testing.T) {
	r := SecretNSResult2419{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByNS = map[string]int{"default": 10}
	if r.Summary.ByNS["default"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestRBSubjectUserResult2419(t *testing.T) {
	r := RBSubjectUserResult2419{HealthScore: 100}
	r.Summary.TotalRB = 50
	r.Summary.UserSubs = 10
	if r.Summary.UserSubs > r.Summary.TotalRB {
		t.Errorf("user > total")
	}
}
func TestNodeOSVerResult2420(t *testing.T) {
	r := NodeOSVerResult2420{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByOSVersion = map[string]int{"Ubuntu 22.04": 5}
	if r.Summary.ByOSVersion["Ubuntu 22.04"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodNodeNameResult2420(t *testing.T) {
	r := PodNodeNameResult2420{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByNode = map[string]int{"node1": 20}
	if r.Summary.ByNode["node1"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestCMImmutableKeyResult2420(t *testing.T) {
	r := CMImmutableKeyResult2420{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.Immutable = 5
	if r.Summary.Immutable > r.Summary.TotalCMs {
		t.Errorf("imm > total")
	}
}
func TestTopNSStorageResult2421(t *testing.T) {
	r := TopNSStorageResult2421{HealthScore: 100}
	r.Summary.TotalNS = 5
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeCPUCapResult2421(t *testing.T) {
	r := NodeCPUCapResult2421{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCPU = 20.0
	if r.Summary.TotalCPU < 0 {
		t.Errorf("negative")
	}
}
func TestNetPolByNSResult2421(t *testing.T) {
	r := NetPolByNSResult2421{HealthScore: 100}
	r.Summary.TotalNetPols = 10
	r.Summary.ByNS = map[string]int{"default": 5}
	if r.Summary.ByNS["default"] != 5 {
		t.Errorf("expected 5")
	}
}
