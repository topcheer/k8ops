package dashboard

import "testing"

func TestWkldAgeResult2035(t *testing.T) {
	r := WkldAgeResult2035{Summary: WkldAgeSummary2035{TotalWorkloads: 50, Fresh: 10, Stale: 20, Ancient: 20}}
	if r.Summary.Ancient != 20 {
		t.Errorf("expected 20")
	}
}
func TestWkldAgeEntry2035(t *testing.T) {
	e := WkldAgeEntry2035{Name: "api", Namespace: "prod", AgeDays: 300}
	if e.AgeDays != 300 {
		t.Errorf("expected 300")
	}
}
func TestSvcMeshResult2035(t *testing.T) {
	r := SvcMeshResult2035{Summary: SvcMeshSummary2035{TotalServices: 80, WithEndpoints: 60, NoEndpoints: 15, Headless: 5}}
	if r.Summary.NoEndpoints != 15 {
		t.Errorf("expected 15")
	}
}
func TestSvcMeshEntry2035(t *testing.T) {
	e := SvcMeshEntry2035{Name: "api", Namespace: "prod", Type: "ClusterIP"}
	if e.Type != "ClusterIP" {
		t.Errorf("expected ClusterIP")
	}
}
func TestTLSForecastResult2035(t *testing.T) {
	r := TLSForecastResult2035{Summary: TLSForecastSummary2035{TotalSecrets: 100, TLSSecrets: 20, ExpiringSoon: 5, NoExpiry: 3}}
	if r.Summary.ExpiringSoon != 5 {
		t.Errorf("expected 5")
	}
}
func TestTLSForecastEntry2035(t *testing.T) {
	e := TLSForecastEntry2035{Name: "tls-cert", Namespace: "prod", Type: "kubernetes.io/tls"}
	if e.Type != "kubernetes.io/tls" {
		t.Errorf("expected tls")
	}
}
