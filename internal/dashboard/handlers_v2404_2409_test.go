package dashboard

import "testing"

func TestOverheadResult2404(t *testing.T) {
	r := OverheadResult2404{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithOverhead = 0
	if r.Summary.WithOverhead > r.Summary.TotalPods {
		t.Errorf("overhead > total")
	}
}
func TestRunAsUserResult2404(t *testing.T) {
	r := RunAsUserResult2404{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithRunAsUser = 30
	if r.Summary.WithRunAsUser > r.Summary.TotalContainers {
		t.Errorf("uid > total")
	}
}
func TestExtTrafficPolResult2404(t *testing.T) {
	r := ExtTrafficPolResult2404{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ByPolicy = map[string]int{"Cluster": 25}
	if r.Summary.ByPolicy["Cluster"] != 25 {
		t.Errorf("expected 25")
	}
}
func TestDepConditionsResult2405(t *testing.T) {
	r := DepConditionsResult2405{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.AvailableConds = 28
	if r.Summary.AvailableConds > r.Summary.TotalDeploys {
		t.Errorf("avail > total")
	}
}
func TestSTSAvailableResult2405(t *testing.T) {
	r := STSAvailableResult2405{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalAvail = 14
	if r.Summary.TotalAvail < 0 {
		t.Errorf("negative")
	}
}
func TestDSCondReadyResult2405(t *testing.T) {
	r := DSCondReadyResult2405{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.TotalReady = 5
	if r.Summary.TotalReady < 0 {
		t.Errorf("negative")
	}
}
func TestHighRestartsResult2406(t *testing.T) {
	r := HighRestartsResult2406{HealthScore: 100}
	r.Summary.TotalContainers = 200
	r.Summary.HighRestarts = 0
	if r.Summary.HighRestarts > r.Summary.TotalContainers {
		t.Errorf("high > total")
	}
}
func TestBootIDResult2406(t *testing.T) {
	r := BootIDResult2406{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.UniqueBoots = 5
	if r.Summary.UniqueBoots > r.Summary.TotalNodes {
		t.Errorf("boot > nodes")
	}
}
func TestEventObjKindResult2406(t *testing.T) {
	r := EventObjKindResult2406{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.ByKind = map[string]int{"Pod": 100}
	if r.Summary.ByKind["Pod"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestSeccompRDResult2407(t *testing.T) {
	r := SeccompRDResult2407{HealthScore: 50}
	r.Summary.TotalPods = 50
	r.Summary.RuntimeDefault = 25
	if r.Summary.RuntimeDefault > r.Summary.TotalPods {
		t.Errorf("rd > total")
	}
}
func TestSecretHelmAnnotResult2407(t *testing.T) {
	r := SecretHelmAnnotResult2407{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.HelmAnnotated = 5
	if r.Summary.HelmAnnotated > r.Summary.TotalSecrets {
		t.Errorf("helm > total")
	}
}
func TestCRBSubjectSAResult2407(t *testing.T) {
	r := CRBSubjectSAResult2407{HealthScore: 100}
	r.Summary.TotalCRB = 30
	r.Summary.SASubjects = 20
	if r.Summary.SASubjects > r.Summary.TotalCRB {
		t.Errorf("sa > total")
	}
}
func TestAllocEphemeralResult2408(t *testing.T) {
	r := AllocEphemeralResult2408{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalGB = 100.0
	if r.Summary.TotalGB < 0 {
		t.Errorf("negative")
	}
}
func TestPodSubdomainResult2408(t *testing.T) {
	r := PodSubdomainResult2408{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithSubdomain = 5
	if r.Summary.WithSubdomain > r.Summary.TotalPods {
		t.Errorf("sub > total")
	}
}
func TestCMBinaryResult2408(t *testing.T) {
	r := CMBinaryResult2408{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.WithBinData = 3
	if r.Summary.WithBinData > r.Summary.TotalCMs {
		t.Errorf("bin > total")
	}
}
func TestTopNSCtnrResult2409(t *testing.T) {
	r := TopNSCtnrResult2409{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeAllocStorEphResult2409(t *testing.T) {
	r := NodeAllocStorEphResult2409{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalGB = 50.0
	if r.Summary.TotalGB < 0 {
		t.Errorf("negative")
	}
}
func TestRoleTotalResult2409(t *testing.T) {
	r := RoleTotalResult2409{HealthScore: 100}
	r.Summary.TotalRoles = 30
	r.Summary.TotalClusterRoles = 70
	if r.Summary.TotalRoles < 0 || r.Summary.TotalClusterRoles < 0 {
		t.Errorf("negative")
	}
}
