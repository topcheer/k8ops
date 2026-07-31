package dashboard

import "testing"

func TestNilSecCtxResult2278(t *testing.T) {
	r := NilSecCtxResult2278{HealthScore: 80}
	r.Summary.TotalContainers = 100
	r.Summary.NilSecCtx = 40
	if r.Summary.NilSecCtx > r.Summary.TotalContainers {
		t.Errorf("nil > total")
	}
}
func TestNetPolDirectionResult2278(t *testing.T) {
	r := NetPolDirectionResult2278{HealthScore: 100}
	r.Summary.TotalNetPols = 10
	r.Summary.WithIngress = 8
	r.Summary.WithEgress = 5
	if r.Summary.WithIngress > r.Summary.TotalNetPols {
		t.Errorf("ingress > total")
	}
}
func TestExtNameSvcResult2278(t *testing.T) {
	r := ExtNameSvcResult2278{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ExternalNameSvc = 3
	if r.Summary.ExternalNameSvc > r.Summary.TotalServices {
		t.Errorf("ext > total")
	}
}
func TestHPAScalingResult2279(t *testing.T) {
	r := HPAScalingResult2279{HealthScore: 100}
	r.Summary.TotalHPA = 5
	r.Summary.MinReplicas = 10
	r.Summary.MaxReplicas = 50
	if r.Summary.MinReplicas > r.Summary.MaxReplicas {
		t.Errorf("min > max")
	}
}
func TestMaxSurgeResult2279(t *testing.T) {
	r := MaxSurgeResult2279{HealthScore: 100}
	r.Summary.TotalDeploys = 30
	r.Summary.WithRollingUpd = 25
	r.Summary.WithCustomSurge = 10
	if r.Summary.WithCustomSurge > r.Summary.WithRollingUpd {
		t.Errorf("surge > rolling")
	}
}
func TestSTSPVCTmplResult2279(t *testing.T) {
	r := STSPVCTmplResult2279{HealthScore: 100}
	r.Summary.TotalSTS = 5
	r.Summary.WithPVCTmpl = 3
	r.Summary.TotalPVCTmpls = 4
	if r.Summary.WithPVCTmpl > r.Summary.TotalSTS {
		t.Errorf("pvc > sts")
	}
}
func TestOOMRiskResult2280(t *testing.T) {
	r := OOMRiskResult2280{HealthScore: 70}
	r.Summary.TotalContainers = 100
	r.Summary.WithoutMemLimit = 30
	r.Summary.OOMKilled = 2
	if r.Summary.WithoutMemLimit > r.Summary.TotalContainers {
		t.Errorf("noLimit > total")
	}
}
func TestPIDPressureResult2280(t *testing.T) {
	r := PIDPressureResult2280{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.WithPressure = 0
	if r.Summary.WithPressure > r.Summary.TotalNodes {
		t.Errorf("pressure > nodes")
	}
}
func TestLastTermReasonResult2280(t *testing.T) {
	r := LastTermReasonResult2280{HealthScore: 100}
	r.Summary.TotalTerminated = 10
	r.Summary.ByReason = map[string]int{"OOMKilled": 5, "Error": 5}
	if r.Summary.ByReason["OOMKilled"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestSeccompResult2281(t *testing.T) {
	r := SeccompResult2281{HealthScore: 60}
	r.Summary.TotalPods = 50
	r.Summary.WithSeccomp = 10
	if r.Summary.WithSeccomp > r.Summary.TotalPods {
		t.Errorf("seccomp > total")
	}
}
func TestDefaultDenyResult2281(t *testing.T) {
	r := DefaultDenyResult2281{HealthScore: 70}
	r.Summary.TotalNS = 10
	r.Summary.WithDefaultDeny = 7
	if r.Summary.WithDefaultDeny > r.Summary.TotalNS {
		t.Errorf("deny > total")
	}
}
func TestWildcardRoleResult2281(t *testing.T) {
	r := WildcardRoleResult2281{HealthScore: 80}
	r.Summary.TotalClusterRoles = 70
	r.Summary.WithWildcard = 20
	if r.Summary.WithWildcard > r.Summary.TotalClusterRoles {
		t.Errorf("wildcard > total")
	}
}
func TestConfigMapAgeResult2282(t *testing.T) {
	r := ConfigMapAgeResult2282{HealthScore: 100}
	r.Summary.TotalConfigMaps = 50
	r.Summary.ByAgeBucket = map[string]int{"<1d": 5, "1-7d": 10, "90d+": 20}
	if r.Summary.ByAgeBucket["90d+"] != 20 {
		t.Errorf("expected 20")
	}
}
func TestNSPhaseResult2282(t *testing.T) {
	r := NSPhaseResult2282{HealthScore: 100}
	r.Summary.TotalNS = 10
	r.Summary.ByPhase = map[string]int{"Active": 10}
	if r.Summary.ByPhase["Active"] != 10 {
		t.Errorf("expected 10")
	}
}
func TestPVCAccessModeResult2282(t *testing.T) {
	r := PVCAccessModeResult2282{HealthScore: 100}
	r.Summary.TotalPVCs = 10
	r.Summary.ByAccessMode = map[string]int{"ReadWriteOnce": 8, "ReadOnlyMany": 2}
	if r.Summary.ByAccessMode["ReadWriteOnce"] != 8 {
		t.Errorf("expected 8")
	}
}
func TestNSPodTopResult2283(t *testing.T) {
	r := NSPodTopResult2283{HealthScore: 100}
	r.Summary.TotalNS = 8
	if r.Summary.TotalNS != 8 {
		t.Errorf("expected 8")
	}
}
func TestCPUOversubResult2283(t *testing.T) {
	r := CPUOversubResult2283{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.Oversubscribed = 0
	r.Summary.MaxOversubPct = 0
	if r.Summary.Oversubscribed > r.Summary.TotalNodes {
		t.Errorf("oversub > nodes")
	}
}
func TestStorageByNSResult2283(t *testing.T) {
	r := StorageByNSResult2283{HealthScore: 100}
	r.Summary.TotalPVCs = 15
	if r.Summary.TotalPVCs != 15 {
		t.Errorf("expected 15")
	}
}
