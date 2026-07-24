package dashboard

import "testing"

func TestCanaryResult1953(t *testing.T) {
	r := CanaryResult1953{Summary: CanarySummary1953{TotalDeployments: 66, CanaryEnabled: 2}}
	if r.Summary.CanaryEnabled != 2 {
		t.Errorf("expected 2")
	}
}
func TestCanaryEntry1953(t *testing.T) {
	e := CanaryEntry1953{Name: "api", Mechanism: "canary", CanaryPct: "10"}
	if e.Mechanism != "canary" {
		t.Errorf("expected canary")
	}
}
func TestInitContainerResult1953(t *testing.T) {
	r := InitContainerResult1953{Summary: InitContainerSummary1953{TotalPods: 79, PodsWithInit: 15, TotalInitConts: 20}}
	if r.Summary.TotalInitConts != 20 {
		t.Errorf("expected 20")
	}
}
func TestInitContainerHeavy1953(t *testing.T) {
	e := InitContainerHeavy1953{Container: "db-migrate", Issue: "Heavy: 2 CPU"}
	if e.Container != "db-migrate" {
		t.Errorf("expected db-migrate")
	}
}
func TestLifecycleHookResult1953(t *testing.T) {
	r := LifecycleHookResult1953{Summary: LifecycleHookSummary1953{TotalContainers: 89, WithPreStop: 5, WithoutPreStop: 84}}
	if r.Summary.WithoutPreStop != 84 {
		t.Errorf("expected 84")
	}
}
func TestLifecycleHookEntry1953(t *testing.T) {
	e := LifecycleHookEntry1953{Container: "main", Missing: []string{"preStop", "postStart"}}
	if len(e.Missing) != 2 {
		t.Errorf("expected 2 missing")
	}
}
func TestCanaryCandidate1953(t *testing.T) {
	e := CanaryCandidate1953{Name: "web", Replicas: 3, Reason: "no progressive"}
	if e.Replicas != 3 {
		t.Errorf("expected 3")
	}
}
func TestInitContainerEntry1953(t *testing.T) {
	e := InitContainerEntry1953{PodName: "web-abc", InitCount: 2}
	if e.InitCount != 2 {
		t.Errorf("expected 2")
	}
}
