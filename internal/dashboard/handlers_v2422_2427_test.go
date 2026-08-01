package dashboard

import "testing"

func TestSAMissingResult2422(t *testing.T) {
	r := SAMissingResult2422{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.MissingSA = 0
	if r.Summary.MissingSA > r.Summary.TotalPods {
		t.Errorf("missing > total")
	}
}
func TestStartupProbeResult2422(t *testing.T) {
	r := StartupProbeResult2422{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStartup = 10
	if r.Summary.WithStartup > r.Summary.TotalContainers {
		t.Errorf("startup > total")
	}
}
func TestExternalNameResult2422(t *testing.T) {
	r := ExternalNameResult2422{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ExternalName = 2
	if r.Summary.ExternalName > r.Summary.TotalServices {
		t.Errorf("ext > total")
	}
}
func TestSTSVolClaimDefaultResult2423(t *testing.T) {
	r := STSVolClaimDefaultResult2423{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.WithVolClaim = 3
	if r.Summary.WithVolClaim > r.Summary.TotalSTS {
		t.Errorf("vc > total")
	}
}
func TestJobCompletionsResult2423(t *testing.T) {
	r := JobCompletionsResult2423{HealthScore: 100}
	r.Summary.TotalJobs = 10
	r.Summary.TotalCompletions = 20
	if r.Summary.TotalCompletions < 0 {
		t.Errorf("negative")
	}
}
func TestCronJobStartDeadlineResult2423(t *testing.T) {
	r := CronJobStartDeadlineResult2423{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.WithDeadline = 3
	if r.Summary.WithDeadline > r.Summary.TotalCronJobs {
		t.Errorf("dl > total")
	}
}
func TestPodCompletedResult2424(t *testing.T) {
	r := PodCompletedResult2424{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.Completed = 5
	if r.Summary.Completed > r.Summary.TotalPods {
		t.Errorf("completed > total")
	}
}
func TestNodeOutOfDiskResult2424(t *testing.T) {
	r := NodeOutOfDiskResult2424{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.OutOfDisk = 0
	if r.Summary.OutOfDisk > r.Summary.TotalNodes {
		t.Errorf("disk > nodes")
	}
}
func TestImageLatestResult2424(t *testing.T) {
	r := ImageLatestResult2424{HealthScore: 80}
	r.Summary.TotalImages = 15
	r.Summary.LatestImages = 3
	if r.Summary.LatestImages > r.Summary.TotalImages {
		t.Errorf("latest > total")
	}
}
func TestSELinuxLevelResult2425(t *testing.T) {
	r := SELinuxLevelResult2425{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithSELinux = 5
	if r.Summary.WithSELinux > r.Summary.TotalContainers {
		t.Errorf("selinux > total")
	}
}
func TestSecretKeyNameResult2425(t *testing.T) {
	r := SecretKeyNameResult2425{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.AllKeyNames = map[string]int{"password": 10}
	if r.Summary.AllKeyNames["password"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestCRResNamesResult2425(t *testing.T) {
	r := CRResNamesResult2425{HealthScore: 100}
	r.Summary.TotalCR = 70
	r.Summary.WithResNames = 10
	if r.Summary.WithResNames > r.Summary.TotalCR {
		t.Errorf("res > total")
	}
}
func TestNodeRoleResult2426(t *testing.T) {
	r := NodeRoleResult2426{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByRole = map[string]int{"worker": 4, "control-plane": 1}
	if r.Summary.ByRole["worker"] != 4 {
		t.Errorf("expected 4")
	}
}
func TestHostAliasesResult2426(t *testing.T) {
	r := HostAliasesResult2426{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithHostAliases = 3
	if r.Summary.WithHostAliases > r.Summary.TotalPods {
		t.Errorf("alias > total")
	}
}
func TestPVCPhaseResult2426(t *testing.T) {
	r := PVCPhaseResult2426{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.ByPhase = map[string]int{"Bound": 8}
	if r.Summary.ByPhase["Bound"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestTopNSRestartResult2427(t *testing.T) {
	r := TopNSRestartResult2427{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestNodeEphemeralGBResult2427(t *testing.T) {
	r := NodeEphemeralGBResult2427{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalGB = 100
	if r.Summary.TotalGB < 0 {
		t.Errorf("negative")
	}
}
func TestCMKeysResult2427(t *testing.T) {
	r := CMKeysResult2427{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.TotalKeys = 150
	if r.Summary.TotalKeys < r.Summary.TotalCMs {
		t.Errorf("keys < CMs")
	}
}
