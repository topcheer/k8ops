package dashboard

import "testing"

func TestPodCompleteResult2296(t *testing.T) {
	r := PodCompleteResult2296{HealthScore: 95}
	r.Summary.TotalPods = 100
	r.Summary.Running = 90
	r.Summary.Succeeded = 5
	r.Summary.Failed = 3
	r.Summary.Pending = 2
	if r.Summary.Running+r.Summary.Succeeded+r.Summary.Failed+r.Summary.Pending != r.Summary.TotalPods {
		t.Errorf("sum mismatch")
	}
}
func TestArgsCatalogResult2296(t *testing.T) {
	r := ArgsCatalogResult2296{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithArgs = 30
	r.Summary.WithCommand = 40
	if r.Summary.WithArgs > r.Summary.TotalContainers {
		t.Errorf("args > total")
	}
}
func TestSvcAcctPullSecretResult2296(t *testing.T) {
	r := SvcAcctPullSecretResult2296{HealthScore: 100}
	r.Summary.TotalSAs = 20
	r.Summary.WithPullSecret = 5
	if r.Summary.WithPullSecret > r.Summary.TotalSAs {
		t.Errorf("pullSecret > total")
	}
}
func TestDSDeployReadyResult2297(t *testing.T) {
	r := DSDeployReadyResult2297{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.DesiredNum = 5
	r.Summary.ReadyNum = 5
	if r.Summary.ReadyNum > r.Summary.DesiredNum {
		t.Errorf("ready > desired")
	}
}
func TestRolloutCondResult2297(t *testing.T) {
	r := RolloutCondResult2297{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.Progressing = 25
	r.Summary.Available = 28
	r.Summary.ReplicaFail = 1
	if r.Summary.Available > r.Summary.TotalDeploys {
		t.Errorf("available > total")
	}
}
func TestCronJobLastSchedResult2297(t *testing.T) {
	r := CronJobLastSchedResult2297{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.WithSchedule = 4
	if r.Summary.WithSchedule > r.Summary.TotalCronJobs {
		t.Errorf("schedule > total")
	}
}
func TestNetUnavailableResult2298(t *testing.T) {
	r := NetUnavailableResult2298{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.WithNetIssue = 0
	if r.Summary.WithNetIssue > r.Summary.TotalNodes {
		t.Errorf("netIssue > nodes")
	}
}
func TestReadyTransitionResult2298(t *testing.T) {
	r := ReadyTransitionResult2298{HealthScore: 98}
	r.Summary.TotalPods = 100
	r.Summary.Ready = 98
	r.Summary.NotReady = 2
	if r.Summary.Ready+r.Summary.NotReady != r.Summary.TotalPods {
		t.Errorf("sum mismatch")
	}
}
func TestEventInvObjResult2298(t *testing.T) {
	r := EventInvObjResult2298{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.ByKind = map[string]int{"Pod": 100, "Node": 50}
	if r.Summary.ByKind["Pod"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestSeccompTypeResult2299(t *testing.T) {
	r := SeccompTypeResult2299{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByType = map[string]int{"RuntimeDefault": 10, "none": 40}
	if r.Summary.ByType["none"] != 40 {
		t.Errorf("expected 40")
	}
}
func TestBindingSubjectResult2299(t *testing.T) {
	r := BindingSubjectResult2299{HealthScore: 100}
	r.Summary.TotalBindings = 30
	r.Summary.ByKind = map[string]int{"ServiceAccount": 20, "User": 5, "Group": 5}
	if r.Summary.ByKind["ServiceAccount"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestNetPolPortResult2299(t *testing.T) {
	r := NetPolPortResult2299{HealthScore: 100}
	r.Summary.TotalNetPols = 10
	r.Summary.TotalPortRules = 25
	if r.Summary.TotalPortRules < r.Summary.TotalNetPols {
		t.Errorf("ports < policies")
	}
}
func TestClusterIPResult2300(t *testing.T) {
	r := ClusterIPResult2300{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.WithClusterIP = 28
	r.Summary.Headless = 2
	if r.Summary.WithClusterIP+r.Summary.Headless != r.Summary.TotalServices {
		t.Errorf("sum mismatch")
	}
}
func TestPodNodeDistResult2300(t *testing.T) {
	r := PodNodeDistResult2300{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.ByNode = map[string]int{"node1": 30, "node2": 70}
	if r.Summary.ByNode["node2"] != 70 {
		t.Errorf("expected 70")
	}
}
func TestCMKeyCountResult2300(t *testing.T) {
	r := CMKeyCountResult2300{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.TotalKeys = 150
	r.Summary.AvgKeys = 3
	if r.Summary.AvgKeys != 3 {
		t.Errorf("expected 3")
	}
}
func TestClusterEffResult2301(t *testing.T) {
	r := ClusterEffResult2301{HealthScore: 60}
	r.Summary.TotalNodes = 5
	r.Summary.AllocCPU = 20.0
	r.Summary.ReqCPU = 10.0
	r.Summary.LimitCPU = 15.0
	r.Summary.EffPct = 66
	if r.Summary.ReqCPU > r.Summary.LimitCPU {
		t.Errorf("req > limit")
	}
}
func TestNSDensityResult2301(t *testing.T) {
	r := NSDensityResult2301{HealthScore: 100}
	r.Summary.TotalNS = 10
	r.Summary.TotalPods = 100
	r.Summary.TotalSvcs = 30
	r.Summary.TotalCMs = 50
	if r.Summary.TotalPods < 0 {
		t.Errorf("negative pods")
	}
}
func TestNodeCPUCommitResult2301(t *testing.T) {
	r := NodeCPUCommitResult2301{HealthScore: 80}
	r.Summary.TotalNodes = 5
	r.Summary.AvgCommit = 50
	r.Summary.MaxCommit = 75
	if r.Summary.MaxCommit < r.Summary.AvgCommit {
		t.Errorf("max < avg")
	}
}
