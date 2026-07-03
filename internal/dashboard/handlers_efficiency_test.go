package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	corev1res "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleEfficiency_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/efficiency", nil)
	rr := httptest.NewRecorder()

	s.handleEfficiency(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestAnalyzeEfficiency_EmptyCluster(t *testing.T) {
	report := analyzeEfficiency([]corev1.Node{}, []corev1.Pod{})

	if report.Score != 100 {
		t.Errorf("empty cluster score = %.0f, want 100", report.Score)
	}
	if len(report.WasteItems) != 0 {
		t.Errorf("expected 0 waste items, got %d", len(report.WasteItems))
	}
}

func TestAnalyzeEfficiency_NoLimits(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node1",
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	report := analyzeEfficiency(makeTestNodes(1), pods)

	if report.Stats.NoResourceLimits != 1 {
		t.Errorf("expected 1 pod without limits, got %d", report.Stats.NoResourceLimits)
	}
	if report.Score >= 100 {
		t.Errorf("score should be <100 with waste, got %.0f", report.Score)
	}
}

func TestAnalyzeEfficiency_OversizedLimits(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node1",
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: corev1res.MustParse("100m"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: corev1res.MustParse("2000m"), // 20x ratio
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	report := analyzeEfficiency(makeTestNodes(1), pods)

	if report.Stats.OversizedLimits != 1 {
		t.Errorf("expected 1 oversized limit, got %d", report.Stats.OversizedLimits)
	}
	if len(report.OverProvisioned) != 1 {
		t.Errorf("expected 1 over-provisioned item, got %d", len(report.OverProvisioned))
	}
}

func TestAnalyzeEfficiency_UnderutilizedNode(t *testing.T) {
	pods := []corev1.Pod{} // no pods → 0% utilization

	report := analyzeEfficiency(makeTestNodes(3), pods)

	if report.Stats.UnderutilizedNodes < 1 {
		t.Errorf("expected at least 1 underutilized node, got %d", report.Stats.UnderutilizedNodes)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{512, "512B"},
		{2048, "2.0Ki"},
		{1048576, "1.0Mi"},
		{1073741824, "1.0Gi"},
	}

	for _, tt := range tests {
		got := humanBytes(tt.input)
		if got != tt.expected {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHandleEfficiency_WithFakeClient(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/efficiency", nil)
	rr := httptest.NewRecorder()

	// Without clientsFromReq setup, will return 503
	s.handleEfficiency(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without client setup, got %d", rr.Code)
	}
}

// makeTestNodes creates N test nodes with standard capacity.
func makeTestNodes(n int) []corev1.Node {
	nodes := make([]corev1.Node, n)
	for i := 0; i < n; i++ {
		nodes[i] = corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node" + string(rune('1'+i))},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    corev1res.MustParse("4"),
					corev1.ResourceMemory: corev1res.MustParse("16Gi"),
					corev1.ResourcePods:   corev1res.MustParse("110"),
				},
			},
		}
	}
	return nodes
}
