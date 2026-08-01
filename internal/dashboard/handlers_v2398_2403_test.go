package dashboard

import "testing"

func TestNodeAffReqResult2398(t *testing.T) {
	r := NodeAffReqResult2398{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithNodeAff = 5
	if r.Summary.WithNodeAff > r.Summary.TotalPods {
		t.Errorf("aff > total")
	}
}
func TestVolumeMountResult2398(t *testing.T) {
	r := VolumeMountResult2398{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalMounts = 200
	if r.Summary.TotalMounts < r.Summary.TotalContainers {
		t.Errorf("mounts < containers")
	}
}
func TestAllocLBNodePortsResult2398(t *testing.T) {
	r := AllocLBNodePortsResult2398{HealthScore: 100}
	r.Summary.TotalLBSvc = 3
	r.Summary.AllocNPTrue = 3
	if r.Summary.AllocNPTrue > r.Summary.TotalLBSvc {
		t.Errorf("alloc > total")
	}
}
func TestProgressDeadlineResult2399(t *testing.T) {
	r := ProgressDeadlineResult2399{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithDeadline = 10
	if r.Summary.WithDeadline > r.Summary.TotalDeploys {
		t.Errorf("deadline > total")
	}
}
func TestSTSPVCRetainResult2399(t *testing.T) {
	r := STSPVCRetainResult2399{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.WithRetain = 1
	if r.Summary.WithRetain > r.Summary.TotalSTS {
		t.Errorf("retain > total")
	}
}
func TestDSTemplateGenResult2399(t *testing.T) {
	r := DSTemplateGenResult2399{HealthScore: 100}
	r.Summary.TotalDS = 3
	r.Summary.TotalGen = 15
	if r.Summary.TotalGen < 0 {
		t.Errorf("negative")
	}
}
func TestOOMKilledResult2400(t *testing.T) {
	r := OOMKilledResult2400{HealthScore: 100}
	r.Summary.TotalTerminated = 10
	r.Summary.OOMKilled = 2
	if r.Summary.OOMKilled > r.Summary.TotalTerminated {
		t.Errorf("oom > total")
	}
}
func TestNodeCondKubeletResult2400(t *testing.T) {
	r := NodeCondKubeletResult2400{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.ByVer = map[string]int{"v1.28.0": 5}
	if r.Summary.ByVer["v1.28.0"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestVolDeviceCountResult2400(t *testing.T) {
	r := VolDeviceCountResult2400{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.TotalVolDevices = 5
	if r.Summary.TotalVolDevices > r.Summary.TotalContainers {
		t.Errorf("dev > total")
	}
}
func TestPrivilegedResult2401(t *testing.T) {
	r := PrivilegedResult2401{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.Privileged = 0
	if r.Summary.Privileged > r.Summary.TotalContainers {
		t.Errorf("priv > total")
	}
}
func TestSecretOpaqueResult2401(t *testing.T) {
	r := SecretOpaqueResult2401{HealthScore: 100}
	r.Summary.TotalSecrets = 20
	r.Summary.OpaqueSecrets = 15
	if r.Summary.OpaqueSecrets > r.Summary.TotalSecrets {
		t.Errorf("opaque > total")
	}
}
func TestRBSubjectsResult2401(t *testing.T) {
	r := RBSubjectsResult2401{HealthScore: 100}
	r.Summary.TotalRB = 50
	r.Summary.TotalSubjects = 80
	if r.Summary.TotalSubjects < r.Summary.TotalRB {
		t.Errorf("subjects < RBs")
	}
}
func TestNodeLabelCountResult2402(t *testing.T) {
	r := NodeLabelCountResult2402{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalLabels = 50
	if r.Summary.TotalLabels < r.Summary.TotalNodes {
		t.Errorf("labels < nodes")
	}
}
func TestPodVolumeCountResult2402(t *testing.T) {
	r := PodVolumeCountResult2402{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.TotalVolumes = 100
	if r.Summary.TotalVolumes < r.Summary.TotalPods {
		t.Errorf("vols < pods")
	}
}
func TestCMDataKeySizeResult2402(t *testing.T) {
	r := CMDataKeySizeResult2402{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.TotalKeys = 150
	if r.Summary.TotalKeys < r.Summary.TotalCMs {
		t.Errorf("keys < CMs")
	}
}
func TestTopNodeMemReqResult2403(t *testing.T) {
	r := TopNodeMemReqResult2403{HealthScore: 100}
	r.Summary.TotalNodes = 5
	if r.Summary.TotalNodes != 5 {
		t.Errorf("expected 5")
	}
}
func TestNSSACountResult2403(t *testing.T) {
	r := NSSACountResult2403{HealthScore: 100}
	r.Summary.TotalSAs = 20
	r.Summary.ByNS = map[string]int{"default": 10}
	if r.Summary.ByNS["default"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestImgUniqueResult2403(t *testing.T) {
	r := ImgUniqueResult2403{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.UniqueImages = 15
	if r.Summary.UniqueImages > r.Summary.TotalContainers {
		t.Errorf("unique > total")
	}
}
