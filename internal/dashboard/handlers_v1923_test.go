package dashboard

import "testing"

func TestAnnotationComplianceResult1923(t *testing.T) {
	r := AnnotationComplianceResult1923{
		Summary:     AnnotationComplianceSummary1923{TotalWorkloads: 30, FullyCompliant: 20, MissingCount: 15},
		HealthScore: 75,
	}
	if r.Summary.FullyCompliant != 20 {
		t.Errorf("expected 20, got %d", r.Summary.FullyCompliant)
	}
}

func TestAnnotationMissingEntry1923(t *testing.T) {
	e := AnnotationMissingEntry1923{Name: "api", Namespace: "prod", Missing: []string{"owner", "contact"}, Severity: "medium"}
	if len(e.Missing) != 2 {
		t.Errorf("expected 2 missing, got %d", len(e.Missing))
	}
}

func TestMultiArchResult1923(t *testing.T) {
	r := MultiArchResult1923{
		Summary: MultiArchSummary1923{TotalImages: 50, MultiArchImages: 30, SingleArchCount: 20},
	}
	if r.Summary.MultiArchImages != 30 {
		t.Errorf("expected 30, got %d", r.Summary.MultiArchImages)
	}
}

func TestMultiArchEntry1923(t *testing.T) {
	e := MultiArchEntry1923{Image: "nginx:1.25", ArchGuess: "multi-arch", IsMultiArch: true}
	if !e.IsMultiArch {
		t.Errorf("expected multi-arch")
	}
}

func TestContainerDepResult1923(t *testing.T) {
	r := ContainerDepResult1923{
		Summary: ContainerDepSummary1923{TotalPods: 80, MultiContainerPods: 15, WithInitContainers: 5, RiskCount: 3},
	}
	if r.Summary.MultiContainerPods != 15 {
		t.Errorf("expected 15, got %d", r.Summary.MultiContainerPods)
	}
}

func TestContainerDepEntry1923(t *testing.T) {
	e := ContainerDepEntry1923{PodName: "api-xxx", Namespace: "prod", ContainerCount: 3, HasDependencies: true}
	if e.ContainerCount != 3 {
		t.Errorf("expected 3, got %d", e.ContainerCount)
	}
}

func TestContainerDepRisk1923(t *testing.T) {
	r := ContainerDepRisk1923{RiskType: "many-containers", Severity: "medium"}
	if r.Severity != "medium" {
		t.Errorf("expected medium, got %s", r.Severity)
	}
}
