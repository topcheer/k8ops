package dashboard

import "testing"

func TestClusterConfigResult1950(t *testing.T) {
	r := ClusterConfigResult1950{Summary: ClusterConfigSummary1950{TotalNodes: 1, K8sVersion: "v1.30.0", TotalNamespaces: 29}}
	if r.Summary.K8sVersion != "v1.30.0" {
		t.Errorf("expected v1.30.0")
	}
}
func TestClusterConfigNode1950(t *testing.T) {
	e := ClusterConfigNode1950{Name: "node-1", KubeletVer: "v1.30.0", Arch: "amd64"}
	if e.Arch != "amd64" {
		t.Errorf("expected amd64")
	}
}
func TestEventHistoryDocResult1950(t *testing.T) {
	r := EventHistoryDocResult1950{Summary: EventHistoryDocSummary1950{TotalEvents: 146, WarningCount: 2, UniqueReasons: 18}}
	if r.Summary.UniqueReasons != 18 {
		t.Errorf("expected 18")
	}
}
func TestEventHistoryDocEntry1950(t *testing.T) {
	e := EventHistoryDocEntry1950{Name: "api", Kind: "Pod", Reason: "Pulled", Type: "Normal"}
	if e.Type != "Normal" {
		t.Errorf("expected Normal")
	}
}
func TestQuotaDocResult1950(t *testing.T) {
	r := QuotaDocResult1950{Summary: QuotaDocSummary1950{TotalNamespaces: 29, WithQuota: 0, WithoutQuota: 29}}
	if r.Summary.WithoutQuota != 29 {
		t.Errorf("expected 29")
	}
}
func TestQuotaDocEntry1950(t *testing.T) {
	e := QuotaDocEntry1950{Namespace: "prod", HasCPU: true, HasMem: true, HasPod: false}
	if !e.HasCPU {
		t.Errorf("expected CPU")
	}
}
func TestQuotaDocNSEntry1950(t *testing.T) {
	e := QuotaDocNSEntry1950{Namespace: "dev", Severity: "medium"}
	if e.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
func TestEventHistoryKindStat1950(t *testing.T) {
	s := EventHistoryKindStat1950{ResourceKind: "Pod", Count: 50}
	if s.Count != 50 {
		t.Errorf("expected 50")
	}
}
