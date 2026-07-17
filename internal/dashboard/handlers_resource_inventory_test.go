package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckLabelCoverage(t *testing.T) {
	cov := LabelsCoverage{}
	checkLabelCoverage(map[string]string{"app": "myapp"}, &cov)
	if cov.WithAppLabel != 1 {
		t.Errorf("expected WithAppLabel=1, got %d", cov.WithAppLabel)
	}

	cov2 := LabelsCoverage{}
	checkLabelCoverage(map[string]string{"foo": "bar"}, &cov2)
	if cov2.WithoutLabels != 1 {
		t.Errorf("expected WithoutLabels=1, got %d", cov2.WithoutLabels)
	}

	cov3 := LabelsCoverage{}
	checkLabelCoverage(map[string]string{"app.kubernetes.io/name": "svc", "team": "dev"}, &cov3)
	if cov3.WithAppLabel != 1 {
		t.Errorf("expected WithAppLabel=1 for app.kubernetes.io/name, got %d", cov3.WithAppLabel)
	}
	if cov3.WithTeamLabel != 1 {
		t.Errorf("expected WithTeamLabel=1, got %d", cov3.WithTeamLabel)
	}
}

func TestFindOrphanedResources(t *testing.T) {
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-svc", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "nonexistent"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "matched-svc", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "exists"}},
		},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", Labels: map[string]string{"app": "exists"}},
		},
	}
	cms := []corev1.ConfigMap{}
	pvcs := []corev1.PersistentVolumeClaim{}

	orphaned := findOrphanedResources(services, pods, cms, pvcs)
	if len(orphaned) == 0 {
		t.Error("expected at least 1 orphaned service")
	}
	found := false
	for _, o := range orphaned {
		if o.Name == "orphan-svc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected orphan-svc in orphaned list")
	}
}

func TestComputeInventoryScore(t *testing.T) {
	// Clean → perfect
	s0 := InventorySummary{}
	if score := computeInventoryScore(s0, InventoryHealth{}, LabelsCoverage{}, 0); score != 100 {
		t.Errorf("expected 100, got %d", score)
	}

	// With crash loops
	s1 := InventorySummary{TotalResources: 100, Pods: 50}
	h1 := InventoryHealth{CrashLoopPods: 5}
	if score := computeInventoryScore(s1, h1, LabelsCoverage{}, 0); score >= 100 {
		t.Errorf("expected lower score with crash loops, got %d", score)
	}

	// With not-ready nodes
	s2 := InventorySummary{Nodes: 3}
	h2 := InventoryHealth{NotReadyNodes: 2}
	if score := computeInventoryScore(s2, h2, LabelsCoverage{}, 0); score > 90 {
		t.Errorf("expected lower score with not-ready nodes, got %d", score)
	}
}
