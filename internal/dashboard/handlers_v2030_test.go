package dashboard

import "testing"

func TestHPAHeadroomResult2030(t *testing.T) {
	r := HPAHeadroomResult2030{Summary: HPAHeadroomSummary2030{TotalHPAs: 10, NearMaxReplicas: 3, NoMaxSet: 1, AtMinReplicas: 4}}
	if r.Summary.NearMaxReplicas != 3 {
		t.Errorf("expected 3")
	}
}
func TestHPAHeadroomEntry2030(t *testing.T) {
	e := HPAHeadroomEntry2030{Name: "api-hpa", Namespace: "prod", CurrentReps: 8, MaxReps: 10, HeadroomPct: 20}
	if e.HeadroomPct != 20 {
		t.Errorf("expected 20")
	}
}
func TestAZSpreadResult2030(t *testing.T) {
	r := AZSpreadResult2030{Summary: AZSpreadSummary2030{TotalDeployments: 50, WithAntiAffinity: 20, WithTopologySpread: 15, PoorSpread: 10}}
	if r.Summary.PoorSpread != 10 {
		t.Errorf("expected 10")
	}
}
func TestAZSpreadEntry2030(t *testing.T) {
	e := AZSpreadEntry2030{Name: "api", Namespace: "prod", Issue: "multi-replica without anti-affinity"}
	if e.Issue == "" {
		t.Errorf("expected non-empty issue")
	}
}
func TestLeaderElectionResult2030(t *testing.T) {
	r := LeaderElectionResult2030{Summary: LeaderElectionSummary2030{TotalLeases: 5, WithHolder: 4, StaleLeases: 1}}
	if r.Summary.StaleLeases != 1 {
		t.Errorf("expected 1")
	}
}
func TestLeaderElectionEntry2030(t *testing.T) {
	e := LeaderElectionEntry2030{Name: "kube-controller-manager", Namespace: "kube-system", Holder: "node-1", AgeSeconds: 30}
	if e.AgeSeconds != 30 {
		t.Errorf("expected 30")
	}
}
