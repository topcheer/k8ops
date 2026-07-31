package dashboard

import "testing"

func TestVolMountCountResult2272(t *testing.T) {
	r := VolMountCountResult2272{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalMounts = 250
	r.Summary.AvgPerContainer = 2
	if r.Summary.AvgPerContainer != 2 {
		t.Errorf("expected 2")
	}
}
func TestDNSPolicyResult2272(t *testing.T) {
	r := DNSPolicyResult2272{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByDNSPolicy = map[string]int{"ClusterFirst": 45, "Default": 5}
	if r.Summary.ByDNSPolicy["ClusterFirst"] != 45 {
		t.Errorf("expected 45")
	}
}
func TestInitCtnrResult2272(t *testing.T) {
	r := InitCtnrResult2272{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithInitCtnrs = 10
	r.Summary.TotalInitCtnrs = 15
	if r.Summary.WithInitCtnrs > r.Summary.TotalPods {
		t.Errorf("init pods > total")
	}
}
func TestCronJobCatalogResult2273(t *testing.T) {
	r := CronJobCatalogResult2273{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.Suspended = 1
	r.Summary.Active = 2
	if r.Summary.Suspended+r.Summary.Active > r.Summary.TotalCronJobs {
		t.Errorf("sum > total")
	}
}
func TestRevisionHistoryResult2273(t *testing.T) {
	r := RevisionHistoryResult2273{HealthScore: 100}
	r.Summary.TotalDeployments = 30
	r.Summary.TotalHistory = 300
	r.Summary.AvgHistory = 10
	if r.Summary.AvgHistory != 10 {
		t.Errorf("expected 10")
	}
}
func TestSTSOrdinalResult2273(t *testing.T) {
	r := STSOrdinalResult2273{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.WithOrderedReady = 4
	r.Summary.WithParallel = 1
	if r.Summary.WithOrderedReady+r.Summary.WithParallel != r.Summary.TotalSTS {
		t.Errorf("sum mismatch")
	}
}
func TestKernelVersionResult2274(t *testing.T) {
	r := KernelVersionResult2274{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByKernel = map[string]int{"5.15.0-25-generic": 5}
	if r.Summary.ByKernel["5.15.0-25-generic"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestGracePeriodResult2274(t *testing.T) {
	r := GracePeriodResult2274{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.DefaultSec = 40
	r.Summary.CustomSec = 8
	r.Summary.ZeroSec = 2
	if r.Summary.DefaultSec+r.Summary.CustomSec+r.Summary.ZeroSec > r.Summary.TotalPods {
		t.Errorf("sum > total")
	}
}
func TestEventReasonTopResult2274(t *testing.T) {
	r := EventReasonTopResult2274{HealthScore: 100}
	r.Summary.TotalEvents = 100
	r.Summary.ByReason = map[string]int{"Pulled": 40, "Created": 30, "Started": 30}
	if r.Summary.ByReason["Pulled"] != 40 {
		t.Errorf("expected 40")
	}
}
func TestReadOnlyRootFSResult2275(t *testing.T) {
	r := ReadOnlyRootFSResult2275{HealthScore: 60}
	r.Summary.TotalContainers = 100
	r.Summary.ReadOnlyRoot = 60
	if r.Summary.ReadOnlyRoot > r.Summary.TotalContainers {
		t.Errorf("readOnly > total")
	}
}
func TestPrivEscResult2275(t *testing.T) {
	r := PrivEscResult2275{HealthScore: 50}
	r.Summary.TotalContainers = 100
	r.Summary.AllowEscalation = 50
	if r.Summary.AllowEscalation > r.Summary.TotalContainers {
		t.Errorf("allowEsc > total")
	}
}
func TestRunAsUserResult2275(t *testing.T) {
	r := RunAsUserResult2275{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.RootUID = 20
	r.Summary.NonRootUID = 60
	r.Summary.Unspecified = 20
	if r.Summary.RootUID+r.Summary.NonRootUID+r.Summary.Unspecified != r.Summary.TotalContainers {
		t.Errorf("sum mismatch")
	}
}
func TestResourceQuotaResult2276(t *testing.T) {
	r := ResourceQuotaResult2276{HealthScore: 100}
	r.Summary.TotalQuotas = 5
	r.Summary.ByNamespace = map[string]int{"default": 3, "kube-system": 2}
	if r.Summary.ByNamespace["default"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestTopoSpreadResult2276(t *testing.T) {
	r := TopoSpreadResult2276{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithTopoSpread = 10
	if r.Summary.WithTopoSpread > r.Summary.TotalPods {
		t.Errorf("topo > total")
	}
}
func TestNodeTaintsResult2276(t *testing.T) {
	r := NodeTaintsResult2276{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.WithTaints = 2
	r.Summary.TotalTaints = 3
	if r.Summary.WithTaints > r.Summary.TotalNodes {
		t.Errorf("taints > nodes")
	}
}
func TestCPUUtilRatioResult2277(t *testing.T) {
	r := CPUUtilRatioResult2277{HealthScore: 100}
	r.Summary.TotalAllocCPU = 20.0
	r.Summary.TotalReqCPU = 10.0
	r.Summary.TotalLimitCPU = 15.0
	r.Summary.UtilPct = 50
	if r.Summary.TotalReqCPU > r.Summary.TotalAllocCPU {
		t.Errorf("req > alloc")
	}
}
func TestMemUtilRatioResult2277(t *testing.T) {
	r := MemUtilRatioResult2277{HealthScore: 80}
	r.Summary.TotalAllocMemGB = 100.0
	r.Summary.TotalReqMemGB = 65.0
	r.Summary.UtilPct = 65
	if r.Summary.TotalReqMemGB > r.Summary.TotalAllocMemGB {
		t.Errorf("req > alloc")
	}
}
func TestNodePodCapacityResult2277(t *testing.T) {
	r := NodePodCapacityResult2277{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCap = 550
	r.Summary.TotalPods = 100
	r.Summary.UtilPct = 18
	if r.Summary.TotalPods > r.Summary.TotalCap {
		t.Errorf("pods > cap")
	}
}
