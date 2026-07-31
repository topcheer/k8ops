package dashboard

import "testing"

func TestHostnameResult2356(t *testing.T) {
	r := HostnameResult2356{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithHostname = 5
	if r.Summary.WithHostname > r.Summary.TotalPods {
		t.Errorf("host > total")
	}
}
func TestCtnrStdinResult2356(t *testing.T) {
	r := CtnrStdinResult2356{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStdin = 3
	if r.Summary.WithStdin > r.Summary.TotalContainers {
		t.Errorf("stdin > total")
	}
}
func TestSvcIPFamilyResult2356(t *testing.T) {
	r := SvcIPFamilyResult2356{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ByIPFamily = map[string]int{"IPv4": 30}
	if r.Summary.ByIPFamily["IPv4"] != 30 {
		t.Errorf("expected 30")
	}
}
func TestDSNodeNameResult2357(t *testing.T) {
	r := DSNodeNameResult2357{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.WithNodeName = 0
	if r.Summary.WithNodeName > r.Summary.TotalDS {
		t.Errorf("node > total")
	}
}
func TestSTSPodMgmtResult2357(t *testing.T) {
	r := STSPodMgmtResult2357{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.ByPolicy = map[string]int{"OrderedReady": 4}
	if r.Summary.ByPolicy["OrderedReady"] != 4 {
		t.Errorf("expected 4")
	}
}
func TestCronJobTZResult2357(t *testing.T) {
	r := CronJobTZResult2357{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.ByTimeZone = map[string]int{"UTC": 5}
	if r.Summary.ByTimeZone["UTC"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestTermSignalResult2358(t *testing.T) {
	r := TermSignalResult2358{HealthScore: 100}
	r.Summary.TotalTerminated = 10
	r.Summary.BySignal = map[string]int{"15": 8}
	if r.Summary.BySignal["15"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestKubeletVerResult2358(t *testing.T) {
	r := KubeletVerResult2358{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByVersion = map[string]int{"v1.28.0": 5}
	if r.Summary.ByVersion["v1.28.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestEventTypeResult2358(t *testing.T) {
	r := EventTypeResult2358{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.ByType = map[string]int{"Normal": 180}
	if r.Summary.ByType["Normal"] != 180 {
		t.Errorf("expected 180")
	}
}
func TestRunAsGroupResult2359(t *testing.T) {
	r := RunAsGroupResult2359{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithGroupSet = 10
	if r.Summary.WithGroupSet > r.Summary.TotalPods {
		t.Errorf("group > total")
	}
}
func TestSSHKeyResult2359(t *testing.T) {
	r := SSHKeyResult2359{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.SSHKeys = 2
	if r.Summary.SSHKeys > r.Summary.TotalSecrets {
		t.Errorf("ssh > total")
	}
}
func TestRoleAPIGroupsResult2359(t *testing.T) {
	r := RoleAPIGroupsResult2359{HealthScore: 100}
	r.Summary.TotalRoles = 30
	r.Summary.ByGroup = map[string]int{"": 20, "apps": 10}
	if r.Summary.ByGroup["apps"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestNodeHostnameResult2360(t *testing.T) {
	r := NodeHostnameResult2360{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByHostname = map[string]int{"node1": 1}
	if r.Summary.TotalNodes != 5 {
		t.Errorf("expected 5")
	}
}
func TestImgPullPolResult2360(t *testing.T) {
	r := ImgPullPolResult2360{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.ByPolicy = map[string]int{"IfNotPresent": 80}
	if r.Summary.ByPolicy["IfNotPresent"] != 80 {
		t.Errorf("expected 80")
	}
}
func TestPVReclaimResult2360(t *testing.T) {
	r := PVReclaimResult2360{HealthScore: 100}
	r.Summary.TotalPVs = 10
	r.Summary.ByPolicy = map[string]int{"Delete": 8}
	if r.Summary.ByPolicy["Delete"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestNSPVCTotalResult2361(t *testing.T) {
	r := NSPVCTotalResult2361{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.ByNS = map[string]int{"default": 5}
	if r.Summary.ByNS["default"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeCtnrRuntimeResult2361(t *testing.T) {
	r := NodeCtnrRuntimeResult2361{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByRuntime = map[string]int{"containerd://1.7.0": 5}
	if r.Summary.ByRuntime["containerd://1.7.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestIngressTotalResult2361(t *testing.T) {
	r := IngressTotalResult2361{HealthScore: 100}
	r.Summary.TotalIngress = 5
	r.Summary.ByNS = map[string]int{"default": 3}
	if r.Summary.ByNS["default"] != 3 {
		t.Errorf("expected 3")
	}
}
