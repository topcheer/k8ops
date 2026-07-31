package dashboard

import "testing"

func TestPreemptionResult2290(t *testing.T) {
	r := PreemptionResult2290{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.Preempted = 2
	if r.Summary.Preempted > r.Summary.TotalPods {
		t.Errorf("preempted > total")
	}
}
func TestStdinTTYResult2290(t *testing.T) {
	r := StdinTTYResult2290{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStdin = 5
	r.Summary.WithTTY = 10
	if r.Summary.WithTTY > r.Summary.TotalContainers {
		t.Errorf("tty > total")
	}
}
func TestLBHealthResult2290(t *testing.T) {
	r := LBHealthResult2290{HealthScore: 100}
	r.Summary.TotalLBSvc = 3
	r.Summary.WithIngress = 3
	if r.Summary.WithIngress > r.Summary.TotalLBSvc {
		t.Errorf("ingress > total")
	}
}
func TestDepProgressResult2291(t *testing.T) {
	r := DepProgressResult2291{HealthScore: 95}
	r.Summary.TotalDeploys = 30
	r.Summary.FullyProgress = 28
	r.Summary.Stalled = 2
	if r.Summary.FullyProgress+r.Summary.Stalled > r.Summary.TotalDeploys {
		t.Errorf("sum > total")
	}
}
func TestRSOwnerResult2291(t *testing.T) {
	r := RSOwnerResult2291{HealthScore: 100}
	r.Summary.TotalRS = 20
	r.Summary.WithOwner = 18
	r.Summary.Orphaned = 2
	if r.Summary.WithOwner+r.Summary.Orphaned != r.Summary.TotalRS {
		t.Errorf("sum mismatch")
	}
}
func TestJobDeadlineResult2291(t *testing.T) {
	r := JobDeadlineResult2291{HealthScore: 100}
	r.Summary.TotalJobs = 10
	r.Summary.WithDeadline = 3
	r.Summary.WithBackoff = 8
	if r.Summary.WithDeadline > r.Summary.TotalJobs {
		t.Errorf("deadline > total")
	}
}
func TestMemPressureResult2292(t *testing.T) {
	r := MemPressureResult2292{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.WithPressure = 0
	if r.Summary.WithPressure > r.Summary.TotalNodes {
		t.Errorf("pressure > nodes")
	}
}
func TestRestartTopResult2292(t *testing.T) {
	r := RestartTopResult2292{HealthScore: 90}
	r.Summary.TotalContainers = 100
	r.Summary.HighRestart = 10
	if r.Summary.HighRestart > r.Summary.TotalContainers {
		t.Errorf("high > total")
	}
}
func TestPullDurationResult2292(t *testing.T) {
	r := PullDurationResult2292{HealthScore: 100}
	r.Summary.TotalImages = 15
	r.Summary.ByPullPolicy = map[string]int{"IfNotPresent": 12, "Always": 3}
	if r.Summary.ByPullPolicy["IfNotPresent"] != 12 {
		t.Errorf("expected 12")
	}
}
func TestCapDropResult2293(t *testing.T) {
	r := CapDropResult2293{HealthScore: 40}
	r.Summary.TotalContainers = 100
	r.Summary.WithCapDrop = 40
	if r.Summary.WithCapDrop > r.Summary.TotalContainers {
		t.Errorf("capDrop > total")
	}
}
func TestRegistryTrustResult2293(t *testing.T) {
	r := RegistryTrustResult2293{HealthScore: 100}
	r.Summary.TotalImages = 15
	r.Summary.ByRegistry = map[string]int{"docker.io": 10, "registry.iot2.win": 5}
	if r.Summary.ByRegistry["docker.io"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestSecretEnvResult2293(t *testing.T) {
	r := SecretEnvResult2293{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithSecretEnvRef = 30
	if r.Summary.WithSecretEnvRef > r.Summary.TotalContainers {
		t.Errorf("secret > total")
	}
}
func TestVolTypeResult2294(t *testing.T) {
	r := VolTypeResult2294{HealthScore: 100}
	r.Summary.TotalVolumes = 200
	r.Summary.ByType = map[string]int{"configMap": 50, "secret": 30, "emptyDir": 100, "pvc": 20}
	if r.Summary.ByType["emptyDir"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestNodeSelectorKeyResult2294(t *testing.T) {
	r := NodeSelectorKeyResult2294{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithNodeSel = 10
	r.Summary.ByKey = map[string]int{"kubernetes.io/arch": 10}
	if r.Summary.ByKey["kubernetes.io/arch"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestNSFinalizerResult2294(t *testing.T) {
	r := NSFinalizerResult2294{HealthScore: 100}
	r.Summary.TotalNS = 10
	r.Summary.WithFinalizer = 2
	r.Summary.ByFinalizer = map[string]int{"kubernetes": 2}
	if r.Summary.ByFinalizer["kubernetes"] != 2 {
		t.Errorf("expected 2")
	}
}
func TestResourceWasteResult2295(t *testing.T) {
	r := ResourceWasteResult2295{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.OverProvisioned = 10
	r.Summary.UnderProvisioned = 5
	if r.Summary.OverProvisioned+r.Summary.UnderProvisioned > r.Summary.TotalContainers {
		t.Errorf("waste > total")
	}
}
func TestPodSpreadBalanceResult2295(t *testing.T) {
	r := PodSpreadBalanceResult2295{HealthScore: 85}
	r.Summary.TotalNodes = 5
	r.Summary.MaxPods = 30
	r.Summary.MinPods = 15
	r.Summary.AvgPods = 20
	if r.Summary.MaxPods < r.Summary.MinPods {
		t.Errorf("max < min")
	}
}
func TestWorkloadConcResult2295(t *testing.T) {
	r := WorkloadConcResult2295{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.ByController = map[string]int{"Deployment": 60, "DaemonSet": 20, "ReplicaSet": 20}
	if r.Summary.ByController["Deployment"] != 60 {
		t.Errorf("expected 60")
	}
}
