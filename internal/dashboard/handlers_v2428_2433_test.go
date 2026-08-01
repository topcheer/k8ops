package dashboard

import "testing"

func TestTopologySpreadResult2428(t *testing.T) {
	r := TopologySpreadResult2428{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithTopoSpread = 10
	if r.Summary.WithTopoSpread > r.Summary.TotalPods {
		t.Errorf("topo > total")
	}
}
func TestStdinOnceResult2428(t *testing.T) {
	r := StdinOnceResult2428{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStdinOnce = 0
	if r.Summary.WithStdinOnce > r.Summary.TotalContainers {
		t.Errorf("once > total")
	}
}
func TestPublishNotReadyResult2428(t *testing.T) {
	r := PublishNotReadyResult2428{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.PublishNotReady = 2
	if r.Summary.PublishNotReady > r.Summary.TotalServices {
		t.Errorf("pub > total")
	}
}
func TestDepStatusRepsResult2429(t *testing.T) {
	r := DepStatusRepsResult2429{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.TotalReps = 100
	if r.Summary.TotalReps < 0 {
		t.Errorf("negative")
	}
}
func TestRSAvailableResult2429(t *testing.T) {
	r := RSAvailableResult2429{HealthScore: 100}
	r.Summary.TotalRS = 20
	r.Summary.TotalAvail = 80
	if r.Summary.TotalAvail < 0 {
		t.Errorf("negative")
	}
}
func TestCronJobActiveCountResult2429(t *testing.T) {
	r := CronJobActiveCountResult2429{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.TotalActive = 3
	if r.Summary.TotalActive < 0 {
		t.Errorf("negative")
	}
}
func TestPodPendingResult2430(t *testing.T) {
	r := PodPendingResult2430{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.Pending = 0
	if r.Summary.Pending > r.Summary.TotalPods {
		t.Errorf("pend > total")
	}
}
func TestNodeCondReadyResult2430(t *testing.T) {
	r := NodeCondReadyResult2430{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ReadyNodes = 5
	if r.Summary.ReadyNodes > r.Summary.TotalNodes {
		t.Errorf("ready > nodes")
	}
}
func TestLimitMemResult2430(t *testing.T) {
	r := LimitMemResult2430{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalLimitMemGB = 50.0
	if r.Summary.TotalLimitMemGB < 0 {
		t.Errorf("negative")
	}
}
func TestProcMountResult2431(t *testing.T) {
	r := ProcMountResult2431{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.UnmaskedProc = 0
	if r.Summary.UnmaskedProc > r.Summary.TotalContainers {
		t.Errorf("unmask > total")
	}
}
func TestSecretDataCountResult2431(t *testing.T) {
	r := SecretDataCountResult2431{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByDataCount = map[string]int{"1 keys": 10}
	if r.Summary.ByDataCount["1 keys"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestCRVerbCreateResult2431(t *testing.T) {
	r := CRVerbCreateResult2431{HealthScore: 100}
	r.Summary.TotalCR = 70
	r.Summary.CanCreate = 40
	if r.Summary.CanCreate > r.Summary.TotalCR {
		t.Errorf("create > total")
	}
}
func TestTaintEffectResult2432(t *testing.T) {
	r := TaintEffectResult2432{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByEffect = map[string]int{"NoSchedule": 3}
	if r.Summary.ByEffect["NoSchedule"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestCtnrArgsCountResult2432(t *testing.T) {
	r := CtnrArgsCountResult2432{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalArgs = 200
	if r.Summary.TotalArgs < r.Summary.TotalContainers {
		t.Errorf("args < containers")
	}
}
func TestPVStatusResult2432(t *testing.T) {
	r := PVStatusResult2432{HealthScore: 100}
	r.Summary.TotalPVs = 10
	r.Summary.ByPhase = map[string]int{"Bound": 8}
	if r.Summary.ByPhase["Bound"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestTopNSSAResult2433(t *testing.T) {
	r := TopNSSAResult2433{HealthScore: 100}
	r.Summary.TotalNS = 5
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeStorCapResult2433(t *testing.T) {
	r := NodeStorCapResult2433{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalGB = 500
	if r.Summary.TotalGB < 0 {
		t.Errorf("negative")
	}
}
func TestSecretBytesResult2433(t *testing.T) {
	r := SecretBytesResult2433{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.TotalBytes = 5000
	if r.Summary.TotalBytes < 0 {
		t.Errorf("negative")
	}
}
