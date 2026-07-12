package dashboard

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAPILoadScore(t *testing.T) {
	tests := []struct {
		name     string
		s        APILoadSummary
		minScore int
		maxScore int
	}{
		{"no namespaces", APILoadSummary{}, 100, 100},
		{"all healthy", APILoadSummary{TotalNamespaces: 5, TotalPods: 30}, 90, 100},
		{"dense namespaces", APILoadSummary{TotalNamespaces: 5, DenseNamespaces: 2}, 75, 85},
		{"high activity", APILoadSummary{TotalNamespaces: 5, HighActivityNS: 3}, 80, 90},
		{"many warnings", APILoadSummary{TotalNamespaces: 5, TotalEvents: 100, WarningEvents: 40}, 75, 85},
		{"all bad", APILoadSummary{TotalNamespaces: 10, DenseNamespaces: 3, HighActivityNS: 5, EmptyNamespaces: 5, TotalEvents: 200, WarningEvents: 80}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := apiLoadScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAPILoadRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := apiLoadRecommendations(APILoadSummary{TotalNamespaces: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := apiLoadRecommendations(APILoadSummary{
			DenseNamespaces: 2,
			HighActivityNS:  3,
			EmptyNamespaces: 1,
			TotalEvents:     100,
			WarningEvents:   30,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestAPILoadAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Dense namespace (>100 pods)
		{},
	}
	// Create 110 pods in "dense-ns"
	for i := 0; i < 110; i++ {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: "dense-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "deploy-1"},
				},
			},
		})
	}
	// 5 pods in normal-ns
	for i := 0; i < 5; i++ {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("normal-pod-%d", i),
				Namespace: "normal-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "StatefulSet", Name: "sts-1"},
				},
			},
		})
	}
	// empty-ns has no pods but has events
	events := []corev1.Event{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "empty-ns"}, Type: "Normal"},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "dense-ns"}, Type: "Warning"},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "dense-ns"}, Type: "Warning"},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "dense-ns"}, Type: "Normal"},
	}
	// Add more events to dense-ns to push it into high activity
	for i := 0; i < 25; i++ {
		events = append(events, corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Namespace: "dense-ns"},
			Type:       "Warning",
		})
	}

	result := apiLoadAuditCore(pods, events)

	if result.Summary.TotalNamespaces < 2 {
		t.Errorf("expected at least 2 namespaces, got %d", result.Summary.TotalNamespaces)
	}
	if result.Summary.TotalPods < 115 {
		t.Errorf("expected at least 115 pods, got %d", result.Summary.TotalPods)
	}
	if result.Summary.DenseNamespaces == 0 {
		t.Error("expected at least 1 dense namespace")
	}
	if result.Summary.WarningEvents < 27 {
		t.Errorf("expected at least 27 warning events, got %d", result.Summary.WarningEvents)
	}
	if len(result.ByNamespace) == 0 {
		t.Error("expected namespace entries")
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations")
	}
}
