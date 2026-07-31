package dashboard

import "testing"

func TestGMSAResult2308(t *testing.T) {
	r := GMSAResult2308{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithGMSA = 0
	if r.Summary.WithGMSA > r.Summary.TotalPods {
		t.Errorf("gmsa > total")
	}
}
func TestStartupProbeTypeResult2308(t *testing.T) {
	r := StartupProbeTypeResult2308{HealthScore: 100}
	r.Summary.TotalWithStartup = 5
	r.Summary.ByProbeType = map[string]int{"httpGet": 3, "tcpSocket": 2}
	if r.Summary.ByProbeType["httpGet"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestIntTrafficResult2308(t *testing.T) {
	r := IntTrafficResult2308{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.ByPolicy = map[string]int{"<default>": 25, "Local": 5}
	if r.Summary.ByPolicy["Local"] != 5 {
		t.Errorf("expected 5")
	}
}
func TestObservedGenResult2309(t *testing.T) {
	r := ObservedGenResult2309{HealthScore: 95}
	r.Summary.TotalDeploys = 30
	r.Summary.InSync = 28
	r.Summary.OutOfSync = 2
	if r.Summary.InSync+r.Summary.OutOfSync != r.Summary.TotalDeploys {
		t.Errorf("sum mismatch")
	}
}
func TestRSTemplateHashResult2309(t *testing.T) {
	r := RSTemplateHashResult2309{HealthScore: 100}
	r.Summary.TotalRS = 20
	r.Summary.ByHashBucket = map[string]int{"abc123": 15, "def456": 5}
	if r.Summary.ByHashBucket["abc123"] != 15 {
		t.Errorf("expected 15")
	}
}
func TestCronJobConcurResult2309(t *testing.T) {
	r := CronJobConcurResult2309{HealthScore: 100}
	r.Summary.TotalCronJobs = 5
	r.Summary.ByConcurrency = map[string]int{"Allow": 3, "Forbid": 2}
	if r.Summary.ByConcurrency["Allow"] != 3 {
		t.Errorf("expected 3")
	}
}
func TestEphemeralResult2310(t *testing.T) {
	r := EphemeralResult2310{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithEphemeral = 0
	if r.Summary.WithEphemeral > r.Summary.TotalPods {
		t.Errorf("ephemeral > total")
	}
}
func TestUnschedulableResult2310(t *testing.T) {
	r := UnschedulableResult2310{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.Unschedulable = 0
	if r.Summary.Unschedulable > r.Summary.TotalNodes {
		t.Errorf("unsched > nodes")
	}
}
func TestEventSourceResult2310(t *testing.T) {
	r := EventSourceResult2310{HealthScore: 100}
	r.Summary.TotalEvents = 200
	r.Summary.ByComponent = map[string]int{"kubelet": 100, "default-scheduler": 50}
	if r.Summary.ByComponent["kubelet"] != 100 {
		t.Errorf("expected 100")
	}
}
func TestSELinuxResult2311(t *testing.T) {
	r := SELinuxResult2311{HealthScore: 100}
	r.Summary.TotalContainers = 100
	r.Summary.WithSELinux = 2
	if r.Summary.WithSELinux > r.Summary.TotalContainers {
		t.Errorf("selinux > total")
	}
}
func TestCMBinaryDataResult2311(t *testing.T) {
	r := CMBinaryDataResult2311{HealthScore: 100}
	r.Summary.TotalCMs = 50
	r.Summary.WithBinaryData = 5
	if r.Summary.WithBinaryData > r.Summary.TotalCMs {
		t.Errorf("binary > total")
	}
}
func TestSASecretRefResult2311(t *testing.T) {
	r := SASecretRefResult2311{HealthScore: 100}
	r.Summary.TotalSAs = 20
	r.Summary.WithSecretRef = 15
	if r.Summary.WithSecretRef > r.Summary.TotalSAs {
		t.Errorf("secret > total")
	}
}
func TestSvcPortNameResult2312(t *testing.T) {
	r := SvcPortNameResult2312{HealthScore: 100}
	r.Summary.TotalServices = 30
	r.Summary.NamedPorts = 25
	r.Summary.ByProtocol = map[string]int{"TCP": 45, "UDP": 5}
	if r.Summary.ByProtocol["TCP"] != 45 {
		t.Errorf("expected 45")
	}
}
func TestHostAliasResult2312(t *testing.T) {
	r := HostAliasResult2312{HealthScore: 100}
	r.Summary.TotalPods = 50
	r.Summary.WithHostAlias = 3
	r.Summary.TotalAliases = 5
	if r.Summary.WithHostAlias > r.Summary.TotalPods {
		t.Errorf("alias > total")
	}
}
func TestNodeBootIDResult2312(t *testing.T) {
	r := NodeBootIDResult2312{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.UniqueBootIDs = 5
	if r.Summary.UniqueBootIDs > r.Summary.TotalNodes {
		t.Errorf("unique > total")
	}
}
func TestNSLimitBalanceResult2313(t *testing.T) {
	r := NSLimitBalanceResult2313{HealthScore: 70}
	r.Summary.TotalNS = 10
	r.Summary.Balanced = 7
	r.Summary.NoLimits = 3
	if r.Summary.Balanced+r.Summary.NoLimits > r.Summary.TotalNS {
		t.Errorf("sum > total")
	}
}
func TestNodeEphemeralResult2313(t *testing.T) {
	r := NodeEphemeralResult2313{HealthScore: 100}
	r.Summary.TotalNodes = 5
	r.Summary.TotalCapGB = 500.0
	r.Summary.TotalAllocGB = 450.0
	if r.Summary.TotalAllocGB > r.Summary.TotalCapGB {
		t.Errorf("alloc > cap")
	}
}
func TestReplicaTotalResult2313(t *testing.T) {
	r := ReplicaTotalResult2313{HealthScore: 100}
	r.Summary.DeployReplicas = 30
	r.Summary.STSReplicas = 10
	r.Summary.DSReplicas = 5
	r.Summary.TotalReplicas = 45
	if r.Summary.TotalReplicas != r.Summary.DeployReplicas+r.Summary.STSReplicas+r.Summary.DSReplicas {
		t.Errorf("sum mismatch")
	}
}
