package dashboard

import "testing"

func TestCtnrPortCatalogResult2260(t *testing.T) {
	r := CtnrPortCatalogResult2260{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.ByPort = map[string]int{"TCP:8080": 50}
	if r.Summary.ByPort["TCP:8080"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestPodQoSDistResult2260(t *testing.T) {
	r := PodQoSDistResult2260{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByQoS = map[string]int{"Burstable": 30, "Guaranteed": 20}
	if r.Summary.ByQoS["Burstable"] != 30 {
		t.Errorf("expected 30")
	}
}
func TestResLimitAdherenceResult2260(t *testing.T) {
	r := ResLimitAdherenceResult2260{HealthScore: 85}
	r.Summary.TotalContainers = 100
	r.Summary.WithLimits = 85
	r.Summary.WithRequests = 90
	if r.Summary.WithLimits > r.Summary.TotalContainers {
		t.Errorf("limits > total")
	}
}
func TestDepStrategyResult2261(t *testing.T) {
	r := DepStrategyResult2261{HealthScore: 100}
	r.Summary.TotalDeployments = 30
	r.Summary.ByStrategy = map[string]int{"RollingUpdate": 28, "Recreate": 2}
	if r.Summary.ByStrategy["RollingUpdate"] != 28 {
		t.Errorf("expected 28")
	}
}
func TestSTSUpdateStrategyResult2261(t *testing.T) {
	r := STSUpdateStrategyResult2261{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.ByStrategy = map[string]int{"RollingUpdate": 5}
	r.Summary.WithPartition = 1
	if r.Summary.WithPartition > r.Summary.TotalSTS {
		t.Errorf("partition > total")
	}
}
func TestDSUpdateStrategyResult2261(t *testing.T) {
	r := DSUpdateStrategyResult2261{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.ByStrategy = map[string]int{"RollingUpdate": 3}
	if r.Summary.TotalDS != 3 {
		t.Errorf("expected 3")
	}
}
func TestRestartPolicyResult2262(t *testing.T) {
	r := RestartPolicyResult2262{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.ByRestartPolicy = map[string]int{"Always": 95, "OnFailure": 5}
	if r.Summary.ByRestartPolicy["Always"] != 95 {
		t.Errorf("expected 95")
	}
}
func TestNodeArchResult2262(t *testing.T) {
	r := NodeArchResult2262{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByArch = map[string]int{"amd64": 5}
	if r.Summary.ByArch["amd64"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPrivilegedEscResult2262(t *testing.T) {
	r := PrivilegedEscResult2262{HealthScore: 90}
	r.Summary.TotalContainers = 100
	r.Summary.Privileged = 10
	if r.Summary.Privileged > r.Summary.TotalContainers {
		t.Errorf("privileged > total")
	}
}
func TestSvcAccountResult2263(t *testing.T) {
	r := SvcAccountResult2263{HealthScore: 70}
	r.Summary.TotalPods = 50
	r.Summary.WithDefaultSA = 30
	if r.Summary.WithDefaultSA > r.Summary.TotalPods {
		t.Errorf("defaultSA > total")
	}
}
func TestSecretTypeResult2263(t *testing.T) {
	r := SecretTypeResult2263{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByType = map[string]int{"Opaque": 15, "kubernetes.io/tls": 5}
	if r.Summary.ByType["Opaque"] != 15 {
		t.Errorf("expected 15")
	}
}
func TestPodSecViolationResult2263(t *testing.T) {
	r := PodSecViolationResult2263{HealthScore: 80}
	r.Summary.TotalPods = 50
	r.Summary.HostNetwork = 5
	if r.Summary.HostNetwork > r.Summary.TotalPods {
		t.Errorf("hostNetwork > total")
	}
}
func TestTolerationCatalogResult2264(t *testing.T) {
	r := TolerationCatalogResult2264{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithTolerations = 10
	r.Summary.ByOperator = map[string]int{"Equal": 8, "Exists": 2}
	if r.Summary.ByOperator["Equal"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeOSImageResult2264(t *testing.T) {
	r := NodeOSImageResult2264{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByOSImage = map[string]int{"Ubuntu 22.04": 5}
	if r.Summary.ByOSImage["Ubuntu 22.04"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPVReclaimResult2264(t *testing.T) {
	r := PVReclaimResult2264{HealthScore: 100}
	r.Summary.TotalPVs = 10
	r.Summary.ByReclaimPolicy = map[string]int{"Retain": 7, "Delete": 3}
	if r.Summary.ByReclaimPolicy["Retain"] != 7 {
		t.Errorf("expected 7")
	}
}
func TestNSMemReqResult2265(t *testing.T) {
	r := NSMemReqResult2265{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestPodDensityPerNodeResult2265(t *testing.T) {
	r := PodDensityPerNodeResult2265{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalPods = 100
	r.Summary.AvgPerNode = 20
	if r.Summary.AvgPerNode != 20 {
		t.Errorf("expected 20")
	}
}
func TestEndpointCountResult2265(t *testing.T) {
	r := EndpointCountResult2265{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.TotalEndpoints = 80
	r.Summary.WithEndpoints = 28
	if r.Summary.WithEndpoints > r.Summary.TotalServices {
		t.Errorf("withEndpoints > totalServices")
	}
}
