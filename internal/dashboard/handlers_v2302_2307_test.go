package dashboard

import "testing"

func TestPodOverheadResult2302(t *testing.T) {
	r := PodOverheadResult2302{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithOverhead = 0
	if r.Summary.WithOverhead > r.Summary.TotalPods {
		t.Errorf("overhead > total")
	}
}
func TestLifecycleHookResult2302(t *testing.T) {
	r := LifecycleHookResult2302{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithPostStart = 20
	r.Summary.WithPreStop = 10
	if r.Summary.WithPostStart > r.Summary.TotalContainers {
		t.Errorf("postStart > total")
	}
}
func TestExtTrafficResult2302(t *testing.T) {
	r := ExtTrafficResult2302{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ByPolicy = map[string]int{"Cluster": 25, "Local": 5}
	if r.Summary.ByPolicy["Cluster"] != 25 {
		t.Errorf("expected 25")
	}
}
func TestSTSStatusResult2303(t *testing.T) {
	r := STSStatusResult2303{HealthScore: 95}
	r.Summary.TotalSTS = 5
	r.Summary.TotalReplicas = 15
	r.Summary.TotalReady = 14
	r.Summary.TotalUpdated = 15
	r.Summary.TotalAvailable = 14
	if r.Summary.TotalReady > r.Summary.TotalReplicas {
		t.Errorf("ready > replicas")
	}
}
func TestDSMisScheduleResult2303(t *testing.T) {
	r := DSMisScheduleResult2303{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.DesiredNum = 5
	r.Summary.ScheduledNum = 5
	r.Summary.MisscheduledNum = 0
	if r.Summary.MisscheduledNum > r.Summary.DesiredNum {
		t.Errorf("missched > desired")
	}
}
func TestJobParallelResult2303(t *testing.T) {
	r := JobParallelResult2303{HealthScore: 100}
	r.Summary.TotalJobs = 10
	r.Summary.WithParallelism = 3
	r.Summary.TotalParallel = 10
	if r.Summary.WithParallelism > r.Summary.TotalJobs {
		t.Errorf("parallel > total")
	}
}
func TestSchedGateResult2304(t *testing.T) {
	r := SchedGateResult2304{HealthScore: 100}
	r.Summary.TotalPods = 5
	r.Summary.WithGates = 1
	if r.Summary.WithGates > r.Summary.TotalPods {
		t.Errorf("gates > total")
	}
}
func TestKubeProxyResult2304(t *testing.T) {
	r := KubeProxyResult2304{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByVersion = map[string]int{"v1.28.0": 5}
	if r.Summary.ByVersion["v1.28.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestStartedStateResult2304(t *testing.T) {
	r := StartedStateResult2304{HealthScore: 98}
	r.Summary.TotalContainers = 200
	r.Summary.Started = 196
	r.Summary.NotStarted = 4
	if r.Summary.Started+r.Summary.NotStarted != r.Summary.TotalContainers {
		t.Errorf("sum mismatch")
	}
}
func TestSAAgeResult2305(t *testing.T) {
	r := SAAgeResult2305{HealthScore: 100}
	r.Summary.TotalSAs = 20
	r.Summary.ByAgeBucket = map[string]int{"90d+": 10, "<1d": 5}
	if r.Summary.ByAgeBucket["90d+"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestFSGroupChangeResult2305(t *testing.T) {
	r := FSGroupChangeResult2305{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByPolicy = map[string]int{"<default>": 45, "Always": 5}
	if r.Summary.ByPolicy["Always"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestCRAggRuleResult2305(t *testing.T) {
	r := CRAggRuleResult2305{HealthScore: 100}
	r.Summary.TotalClusterRoles = 70
	r.Summary.WithAggregation = 15
	if r.Summary.WithAggregation > r.Summary.TotalClusterRoles {
		t.Errorf("agg > total")
	}
}
func TestPVPhaseResult2306(t *testing.T) {
	r := PVPhaseResult2306{HealthScore: 100}
	r.Summary.TotalPVs = 10
	r.Summary.ByPhase = map[string]int{"Bound": 8, "Available": 2}
	if r.Summary.ByPhase["Bound"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestResClaimResult2306(t *testing.T) {
	r := ResClaimResult2306{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithResClaims = 0
	r.Summary.TotalClaims = 0
	if r.Summary.WithResClaims > r.Summary.TotalPods {
		t.Errorf("claims > pods")
	}
}
func TestNodePodCIDRResult2306(t *testing.T) {
	r := NodePodCIDRResult2306{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.WithPodCIDR = 5
	r.Summary.ByPrefix = map[string]int{"10.244.0.": 3, "10.244.1.": 2}
	if r.Summary.ByPrefix["10.244.0."] != 3 {
		t.Errorf("expected 3")
	}
}
func TestNSCPURatioResult2307(t *testing.T) {
	r := NSCPURatioResult2307{HealthScore: 80}
	r.Summary.TotalNS = 10
	r.Summary.WellBalanced = 8
	if r.Summary.WellBalanced > r.Summary.TotalNS {
		t.Errorf("balanced > total")
	}
}
func TestNodeStorageCommitResult2307(t *testing.T) {
	r := NodeStorageCommitResult2307{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCapGB = 500.0
	r.Summary.TotalAllocGB = 450.0
	r.Summary.CommitPct = 10
	if r.Summary.TotalAllocGB > r.Summary.TotalCapGB {
		t.Errorf("alloc > cap")
	}
}
func TestPodChurnResult2307(t *testing.T) {
	r := PodChurnResult2307{HealthScore: 80}
	r.Summary.TotalPods = 100
	r.Summary.Created1h = 5
	r.Summary.Created24h = 20
	if r.Summary.Created1h > r.Summary.Created24h {
		t.Errorf("1h > 24h")
	}
}
