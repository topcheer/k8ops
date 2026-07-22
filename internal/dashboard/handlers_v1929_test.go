package dashboard

import "testing"

func TestStatefulSetHealthResult1929(t *testing.T) {
	r := StatefulSetHealthResult1929{
		Summary: StatefulSetHealthSummary1929{TotalSTS: 10, HealthySTS: 8, IssuesCount: 4},
	}
	if r.Summary.HealthySTS != 8 {
		t.Errorf("expected 8, got %d", r.Summary.HealthySTS)
	}
}

func TestStatefulSetEntry1929(t *testing.T) {
	e := StatefulSetEntry1929{Name: "mysql", Replicas: 3, ReadyReplicas: 3, UpdateStrategy: "RollingUpdate", HasPVC: true}
	if !e.HasPVC {
		t.Errorf("expected PVC=true")
	}
}

func TestStatefulSetIssue1929(t *testing.T) {
	i := StatefulSetIssue1929{Name: "redis", IssueType: "single-replica", Severity: "medium"}
	if i.Severity != "medium" {
		t.Errorf("expected medium")
	}
}

func TestImagePullSecretResult1929(t *testing.T) {
	r := ImagePullSecretResult1929{
		Summary: ImagePullSecretSummary1929{TotalNamespaces: 29, WithPullSecret: 5, PodsMissingSecret: 12},
	}
	if r.Summary.PodsMissingSecret != 12 {
		t.Errorf("expected 12, got %d", r.Summary.PodsMissingSecret)
	}
}

func TestImagePullSecretGap1929(t *testing.T) {
	g := ImagePullSecretGap1929{PodName: "api-1", Image: "registry.iot2.win/app:v1", Severity: "high"}
	if g.Severity != "high" {
		t.Errorf("expected high")
	}
}

func TestTopologyDistResult1929(t *testing.T) {
	r := TopologyDistResult1929{
		Summary: TopologyDistSummary1929{TotalWorkloads: 30, WellDistributed: 20, Concentrated: 5, TotalNodes: 3},
	}
	if r.Summary.Concentrated != 5 {
		t.Errorf("expected 5, got %d", r.Summary.Concentrated)
	}
}

func TestNodeSpreadEntry1929(t *testing.T) {
	e := NodeSpreadEntry1929{NodeName: "node-1", PodCount: 25, Workloads: 10}
	if e.PodCount != 25 {
		t.Errorf("expected 25")
	}
}

func TestTopologyRisk1929(t *testing.T) {
	r := TopologyRisk1929{RiskType: "poor-spread", Severity: "high"}
	if r.RiskType != "poor-spread" {
		t.Errorf("expected poor-spread")
	}
}
