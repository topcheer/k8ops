package dashboard

import "testing"

func TestImageTagResult2033(t *testing.T) {
	r := ImageTagResult2033{Summary: ImageTagSummary2033{TotalImages: 30, MutableTags: 5, DigestPinned: 10, NoTag: 2}}
	if r.Summary.MutableTags != 5 {
		t.Errorf("expected 5")
	}
}
func TestImageTagEntry2033(t *testing.T) {
	e := ImageTagEntry2033{Pod: "app", Namespace: "prod", Image: "nginx:latest", Tag: "latest"}
	if e.Tag != "latest" {
		t.Errorf("expected latest")
	}
}
func TestRBACWildcardResult2033(t *testing.T) {
	r := RBACWildcardResult2033{Summary: RBACWildcardSummary2033{TotalClusterRoles: 50, WildcardVerbs: 3, WildcardResources: 2}}
	if r.Summary.WildcardVerbs != 3 {
		t.Errorf("expected 3")
	}
}
func TestRBACWildcardEntry2033(t *testing.T) {
	e := RBACWildcardEntry2033{Name: "cluster-admin", Issue: "wildcard verb", Verbs: "*", Resources: "*"}
	if e.Issue == "" {
		t.Errorf("expected non-empty issue")
	}
}
func TestSecCtxBaselineResult2033(t *testing.T) {
	r := SecCtxBaselineResult2033{Summary: SecCtxBaselineSummary2033{TotalContainers: 100, WithNonRoot: 40, WithReadOnlyFS: 15, NoSecurityCtx: 50}}
	if r.Summary.NoSecurityCtx != 50 {
		t.Errorf("expected 50")
	}
}
func TestSecCtxBaselineEntry2033(t *testing.T) {
	e := SecCtxBaselineEntry2033{Pod: "app", Namespace: "prod", Container: "web"}
	if e.Container != "web" {
		t.Errorf("expected web")
	}
}
