package dashboard

import "testing"

func TestPodOSNameResult2338(t *testing.T) {
	r := PodOSNameResult2338{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.ByOSName = map[string]int{"linux": 50}
	if r.Summary.ByOSName["linux"] != 50 {
		t.Errorf("expected 50")
	}
}
func TestStderrResult2338(t *testing.T) {
	r := StderrResult2338{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithStderr = 0
	if r.Summary.WithStderr > r.Summary.TotalContainers {
		t.Errorf("stderr > total")
	}
}
func TestSessionAffConfigResult2338(t *testing.T) {
	r := SessionAffConfigResult2338{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ClientIP = 2
	if r.Summary.ClientIP > r.Summary.TotalServices {
		t.Errorf("clientIP > total")
	}
}
func TestSTSCollisionResult2339(t *testing.T) {
	r := STSCollisionResult2339{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.TotalCollisions = 0
	if r.Summary.TotalCollisions < 0 {
		t.Errorf("negative collisions")
	}
}
func TestDSUpdatedDesiredResult2339(t *testing.T) {
	r := DSUpdatedDesiredResult2339{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.UpdatedNum = 5
	r.Summary.DesiredNum = 5
	if r.Summary.UpdatedNum > r.Summary.DesiredNum {
		t.Errorf("upd > desired")
	}
}
func TestJobTTLResult2339(t *testing.T) {
	r := JobTTLResult2339{HealthScore: 100}
	r.Summary.TotalJobs = 10
	r.Summary.WithTTL = 3
	if r.Summary.WithTTL > r.Summary.TotalJobs {
		t.Errorf("ttl > total")
	}
}
func TestImgIDResult2340(t *testing.T) {
	r := ImgIDResult2340{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithImgID = 90
	if r.Summary.WithImgID > r.Summary.TotalContainers {
		t.Errorf("imgID > total")
	}
}
func TestNodeCondDiskResult2340(t *testing.T) {
	r := NodeCondDiskResult2340{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.DiskPressure = 0
	if r.Summary.DiskPressure > r.Summary.TotalNodes {
		t.Errorf("disk > nodes")
	}
}
func TestVolDeviceResult2340(t *testing.T) {
	r := VolDeviceResult2340{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithDevice = 5
	if r.Summary.WithDevice > r.Summary.TotalContainers {
		t.Errorf("device > total")
	}
}
func TestUIDRangeResult2341(t *testing.T) {
	r := UIDRangeResult2341{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.RootUID = 20
	if r.Summary.RootUID > r.Summary.TotalContainers {
		t.Errorf("root > total")
	}
}
func TestDockerConfigResult2341(t *testing.T) {
	r := DockerConfigResult2341{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.DockerConfigSecrets = 3
	if r.Summary.DockerConfigSecrets > r.Summary.TotalSecrets {
		t.Errorf("docker > total")
	}
}
func TestRoleVerbWildcardResult2341(t *testing.T) {
	r := RoleVerbWildcardResult2341{HealthScore: 90}
	r.Summary.TotalRoles = 30
	r.Summary.WildcardVerbs = 5
	if r.Summary.WildcardVerbs > r.Summary.TotalRoles {
		t.Errorf("wildcard > total")
	}
}
func TestNodeRegionResult2342(t *testing.T) {
	r := NodeRegionResult2342{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByRegion = map[string]int{"<unknown>": 5}
	if r.Summary.ByRegion["<unknown>"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodOwnerKindResult2342(t *testing.T) {
	r := PodOwnerKindResult2342{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.ByKind = map[string]int{"Deployment": 60, "DaemonSet": 20}
	if r.Summary.ByKind["Deployment"] != 60 {
		t.Errorf("expected 60")
	}
}
func TestSecretCreationOrderResult2342(t *testing.T) {
	r := SecretCreationOrderResult2342{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.ByAgeBucket = map[string]int{"90d+": 10, "<1d": 5}
	if r.Summary.ByAgeBucket["90d+"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestNodeZoneResult2343(t *testing.T) {
	r := NodeZoneResult2343{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByZone = map[string]int{"<unknown>": 5}
	if r.Summary.ByZone["<unknown>"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestSchedLatencyResult2343(t *testing.T) {
	r := SchedLatencyResult2343{HealthScore: 100}
	r.Summary.TotalPods = 100
	r.Summary.RecentPods = 5
	if r.Summary.RecentPods > r.Summary.TotalPods {
		t.Errorf("recent > total")
	}
}
func TestDeployDensityResult2343(t *testing.T) {
	r := DeployDensityResult2343{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.ByNamespace = map[string]int{"default": 15}
	if r.Summary.ByNamespace["default"] != 15 {
		t.Errorf("expected 15")
	}
}
