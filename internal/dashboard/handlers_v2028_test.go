package dashboard

import "testing"

func TestHelmInvResult2028(t *testing.T) {
	r := HelmInvResult2028{Summary: HelmInvSummary2028{TotalNamespaces: 10, HelmNamespaces: 5, HelmWorkloads: 15, OrphanedReleases: 2}}
	if r.Summary.HelmWorkloads != 15 {
		t.Errorf("expected 15")
	}
}
func TestHelmInvEntry2028(t *testing.T) {
	e := HelmInvEntry2028{Namespace: "prod", Release: "myapp", Workloads: 3, ChartVer: "1.2.3"}
	if e.Release != "myapp" {
		t.Errorf("expected myapp")
	}
}
func TestPDBDocResult2028(t *testing.T) {
	r := PDBDocResult2028{Summary: PDBDocSummary2028{TotalWorkloads: 50, WithPDB: 20, WithoutPDB: 30}}
	if r.Summary.WithoutPDB != 30 {
		t.Errorf("expected 30")
	}
}
func TestPDBDocEntry2028(t *testing.T) {
	e := PDBDocEntry2028{Name: "api", Namespace: "prod", Kind: "Deployment", HasPDB: true, Replicas: 3}
	if !e.HasPDB {
		t.Errorf("expected HasPDB true")
	}
}
func TestSATokenAgeResult2028(t *testing.T) {
	r := SATokenAgeResult2028{Summary: SATokenAgeSummary2028{TotalSAs: 30, OldTokens: 10, AncientTokens: 5}}
	if r.Summary.AncientTokens != 5 {
		t.Errorf("expected 5")
	}
}
func TestSATokenAgeEntry2028(t *testing.T) {
	e := SATokenAgeEntry2028{Name: "old-sa", Namespace: "default", AgeDays: 800}
	if e.AgeDays != 800 {
		t.Errorf("expected 800")
	}
}
