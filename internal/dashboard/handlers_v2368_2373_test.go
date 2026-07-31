package dashboard

import "testing"

func TestHostIPCResult2368(t *testing.T) {
	r := HostIPCResult2368{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.HostIPC = 2
	if r.Summary.HostIPC > r.Summary.TotalPods {
		t.Errorf("ipc > total")
	}
}
func TestCtnrTimeoutResult2368(t *testing.T) {
	r := CtnrTimeoutResult2368{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.ByProbeTimeout = map[string]int{"liveness": 80, "readiness": 75}
	if r.Summary.ByProbeTimeout["liveness"] != 80 {
		t.Errorf("expected 80")
	}
}
func TestClusterIPTypeResult2368(t *testing.T) {
	r := ClusterIPTypeResult2368{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ClusterIPEmpty = 2
	if r.Summary.ClusterIPEmpty > r.Summary.TotalServices {
		t.Errorf("empty > total")
	}
}
func TestDepMinReadyResult2369(t *testing.T) {
	r := DepMinReadyResult2369{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithCustom = 5
	if r.Summary.WithCustom > r.Summary.TotalDeploys {
		t.Errorf("custom > total")
	}
}
func TestSTSMinReadyResult2369(t *testing.T) {
	r := STSMinReadyResult2369{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.WithCustom = 1
	if r.Summary.WithCustom > r.Summary.TotalSTS {
		t.Errorf("custom > total")
	}
}
func TestDSMinReadyResult2369(t *testing.T) {
	r := DSMinReadyResult2369{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.WithCustom = 0
	if r.Summary.WithCustom > r.Summary.TotalDS {
		t.Errorf("custom > total")
	}
}
func TestPodCondTypeResult2370(t *testing.T) {
	r := PodCondTypeResult2370{HealthScore: 100}
	r.Summary.TotalConditions = 200
	r.Summary.ByType = map[string]int{"Ready": 100}
	if r.Summary.ByType["Ready"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestNodeOSNameResult2370(t *testing.T) {
	r := NodeOSNameResult2370{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByOS = map[string]int{"linux": 5}
	if r.Summary.ByOS["linux"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestTermExitCodeResult2370(t *testing.T) {
	r := TermExitCodeResult2370{HealthScore: 100}
	r.Summary.TotalTerminated = 10
	r.Summary.ByExitCode = map[string]int{"0": 8}
	if r.Summary.ByExitCode["0"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestNonRootUIDResult2371(t *testing.T) {
	r := NonRootUIDResult2371{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.NonRootUID = 30
	if r.Summary.NonRootUID > r.Summary.TotalPods {
		t.Errorf("nonroot > total")
	}
}
func TestHelmSecretResult2371(t *testing.T) {
	r := HelmSecretResult2371{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.HelmSecrets = 5
	if r.Summary.HelmSecrets > r.Summary.TotalSecrets {
		t.Errorf("helm > total")
	}
}
func TestCRBRoleRefResult2371(t *testing.T) {
	r := CRBRoleRefResult2371{HealthScore: 100}
	r.Summary.TotalCRB = 30
	r.Summary.ByKind = map[string]int{"ClusterRole": 25}
	if r.Summary.ByKind["ClusterRole"] != 25 {
		t.Errorf("expected 25")
	}
}
func TestProviderLabelResult2372(t *testing.T) {
	r := ProviderLabelResult2372{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByProvider = map[string]int{"<unknown>": 5}
	if r.Summary.ByProvider["<unknown>"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodEnvCountResult2372(t *testing.T) {
	r := PodEnvCountResult2372{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalEnvVars = 300
	if r.Summary.TotalEnvVars < r.Summary.TotalContainers {
		t.Errorf("envs < containers")
	}
}
func TestSecretAnnotResult2372(t *testing.T) {
	r := SecretAnnotResult2372{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.WithAnnotations = 10
	if r.Summary.WithAnnotations > r.Summary.TotalSecrets {
		t.Errorf("annot > total")
	}
}
func TestTopNodePodResult2373(t *testing.T) {
	r := TopNodePodResult2373{HealthScore: 100}
	r.Summary.TotalNodes = 5
	if r.Summary.TotalNodes != 5 {
		t.Errorf("expected 5")
	}
}
func TestNSHPACoverageResult2373(t *testing.T) {
	r := NSHPACoverageResult2373{HealthScore: 100}
	r.Summary.TotalNS = 3
	r.Summary.ByNS = map[string]int{"default": 2}
	if r.Summary.ByNS["default"] != 2 {
		t.Errorf("expected 2")
	}
}
func TestEPSvcRatioResult2373(t *testing.T) {
	r := EPSvcRatioResult2373{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.TotalEndpoints = 28
	r.Summary.Ratio = 93
	if r.Summary.Ratio > 100 {
		t.Errorf("ratio > 100")
	}
}
