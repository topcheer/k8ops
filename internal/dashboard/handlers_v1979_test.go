package dashboard

import "testing"

func TestSAPrivScopeResult1979(t *testing.T) {
	r := SAPrivScopeResult1979{Summary: SAPrivScopeSummary1979{TotalSAs: 30, BroadScope: 5, ClusterScoped: 3}}
	if r.Summary.ClusterScoped != 3 {
		t.Errorf("expected 3")
	}
}
func TestSAPrivScopeEntry1979(t *testing.T) {
	e := SAPrivScopeEntry1979{Name: "admin-sa", Namespace: "kube-system", Bindings: 8, IsCluster: true}
	if !e.IsCluster {
		t.Errorf("expected true")
	}
}
func TestTokenAuditResult1979(t *testing.T) {
	r := TokenAuditResult1979{Summary: TokenAuditSummary1979{TotalPods: 80, WithAutoMount: 70, ExplicitDisable: 10}}
	if r.Summary.ExplicitDisable != 10 {
		t.Errorf("expected 10")
	}
}
func TestTokenAuditEntry1979(t *testing.T) {
	e := TokenAuditEntry1979{Pod: "app", Namespace: "prod", SAName: "default", Automount: true, Issue: "broad"}
	if e.Automount != true {
		t.Errorf("expected true")
	}
}
func TestSecretVolResult1979(t *testing.T) {
	r := SecretVolResult1979{Summary: SecretVolSummary1979{TotalPods: 50, VolumeMounts: 15, EnvVarRefs: 8}}
	if r.Summary.VolumeMounts != 15 {
		t.Errorf("expected 15")
	}
}
func TestSecretVolEntry1979(t *testing.T) {
	e := SecretVolEntry1979{Pod: "app", Namespace: "prod", Secret: "db-pass", MountType: "volume", ReadOnly: true}
	if e.MountType != "volume" {
		t.Errorf("expected volume")
	}
}
func TestSAPrivScopeSummary1979(t *testing.T) {
	s := SAPrivScopeSummary1979{WithWildcard: 2, DefaultSAs: 10}
	if s.DefaultSAs != 10 {
		t.Errorf("expected 10")
	}
}
func TestSecretVolSummary1979(t *testing.T) {
	s := SecretVolSummary1979{AllKeysMounted: 5, WritableMount: 2}
	if s.WritableMount != 2 {
		t.Errorf("expected 2")
	}
}
