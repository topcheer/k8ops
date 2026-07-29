package dashboard

import "testing"

func TestSecretAgeResult2058(t *testing.T) {
	r := SecretAgeResult2058{Summary: SecretAgeSummary2058{TotalSecrets: 100, OldSecrets: 20}}
	if r.Summary.OldSecrets != 20 {
		t.Errorf("expected 20")
	}
}
func TestRBACBindResult2058(t *testing.T) {
	r := RBACBindResult2058{Summary: RBACBindSummary2058{TotalRoleBindings: 50, ExcessiveBindings: 5}}
	if r.Summary.ExcessiveBindings != 5 {
		t.Errorf("expected 5")
	}
}
func TestEscSurfaceResult2058(t *testing.T) {
	r := EscSurfaceResult2058{Summary: EscSurfaceSummary2058{TotalPods: 100, AtRiskPods: 10}}
	if r.Summary.AtRiskPods != 10 {
		t.Errorf("expected 10")
	}
}
