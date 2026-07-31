package dashboard

import "testing"

func TestRuntimeClassResult2326(t *testing.T) {
	r := RuntimeClassResult2326{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByRuntimeClass = map[string]int{"<default>": 50}
	if r.Summary.ByRuntimeClass["<default>"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestStdinOnceResult2326(t *testing.T) {
	r := StdinOnceResult2326{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStdinOnce = 0
	if r.Summary.WithStdinOnce > r.Summary.TotalContainers {
		t.Errorf("once > total")
	}
}
func TestAllocCIDRResult2326(t *testing.T) {
	r := AllocCIDRResult2326{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ByCIDRPrefix = map[string]int{"10.96.": 30}
	if r.Summary.ByCIDRPrefix["10.96."] != 30 {
		t.Errorf("expected 30")
	}
}
func TestSTSRevisionResult2327(t *testing.T) {
	r := STSRevisionResult2327{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalRevisions = 15
	r.Summary.TotalUpdatedRev = 15
	if r.Summary.TotalUpdatedRev > r.Summary.TotalRevisions {
		t.Errorf("upd > rev")
	}
}
func TestDSUpdatedNumResult2327(t *testing.T) {
	r := DSUpdatedNumResult2327{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.TotalDesired = 5
	r.Summary.TotalUpdated = 5
	if r.Summary.TotalUpdated > r.Summary.TotalDesired {
		t.Errorf("upd > desired")
	}
}
func TestJobFailRateResult2327(t *testing.T) {
	r := JobFailRateResult2327{HealthScore: 90}
	r.Summary.TotalJobs = 20
	r.Summary.Succeeded = 18
	r.Summary.Failed = 2
	r.Summary.FailPct = 10
	if r.Summary.Succeeded+r.Summary.Failed > r.Summary.TotalJobs {
		t.Errorf("sum > total")
	}
}
func TestBurstableResult2328(t *testing.T) {
	r := BurstableResult2328{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByQoS = map[string]int{"Burstable": 30, "Guaranteed": 20}
	if r.Summary.ByQoS["Burstable"] != 30 {
		t.Errorf("expected 30")
	}
}
func TestNodeMemFrag2Result2328(t *testing.T) {
	r := NodeMemFrag2Result2328{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCapGB = 100
	r.Summary.TotalReqGB = 50
	r.Summary.FragPct = 50
	if r.Summary.FragPct > 100 {
		t.Errorf("frag > 100")
	}
}
func TestImageAgeResult2328(t *testing.T) {
	r := ImageAgeResult2328{HealthScore: 80}
	r.Summary.TotalImages = 15
	r.Summary.ByTag = map[string]int{"versioned": 12, "latest/none": 3}
	if r.Summary.ByTag["latest/none"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestSysctlResult2329(t *testing.T) {
	r := SysctlResult2329{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithSysctl = 2
	if r.Summary.WithSysctl > r.Summary.TotalPods {
		t.Errorf("sysctl > total")
	}
}
func TestCMProjectedVolResult2329(t *testing.T) {
	r := CMProjectedVolResult2329{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithProjected = 5
	if r.Summary.WithProjected > r.Summary.TotalPods {
		t.Errorf("proj > total")
	}
}
func TestRoleBindingUserResult2329(t *testing.T) {
	r := RoleBindingUserResult2329{HealthScore: 100}
	r.Summary.TotalBindings = 30
	r.Summary.BySubjectKind = map[string]int{"ServiceAccount": 20, "User": 10}
	if r.Summary.BySubjectKind["ServiceAccount"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestEPReadyResult2330(t *testing.T) {
	r := EPReadyResult2330{HealthScore: 100}
	r.Summary.TotalEndpoints = 30
	r.Summary.ReadyAddrs = 80
	r.Summary.NotReadyAddrs = 2
	if r.Summary.NotReadyAddrs > r.Summary.ReadyAddrs {
		t.Errorf("notReady > ready")
	}
}
func TestNodeAllocCPUResult2330(t *testing.T) {
	r := NodeAllocCPUResult2330{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCPUs = 20
	if r.Summary.TotalCPUs < 0 {
		t.Errorf("negative CPUs")
	}
}
func TestPodDNSConfigResult2330(t *testing.T) {
	r := PodDNSConfigResult2330{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithDNSConfig = 3
	r.Summary.WithDNSPolicy = 1
	if r.Summary.WithDNSConfig > r.Summary.TotalPods {
		t.Errorf("dns > total")
	}
}
func TestImgRegistryResult2331(t *testing.T) {
	r := ImgRegistryResult2331{HealthScore: 100}
	r.Summary.TotalImages = 15
	r.Summary.ByRegistry = map[string]int{"docker.io": 10}
	if r.Summary.ByRegistry["docker.io"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestNodeHeadroomResult2331(t *testing.T) {
	r := NodeHeadroomResult2331{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCapPods = 550
	r.Summary.TotalPods = 100
	r.Summary.HeadroomPods = 450
	if r.Summary.HeadroomPods < 0 {
		t.Errorf("negative headroom")
	}
}
func TestNSSvcDensityResult2331(t *testing.T) {
	r := NSSvcDensityResult2331{HealthScore: 100}
	r.Summary.TotalNS = 10
	r.Summary.TotalServices = 30
	r.Summary.AvgPerNS = 3
	if r.Summary.AvgPerNS != 3 {
		t.Errorf("expected 3")
	}
}
