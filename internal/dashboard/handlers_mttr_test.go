package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestMTTREmptyCluster verifies handler works on empty cluster.
func TestMTTREmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	// No failures = high stability
	if result.Summary.StabilityScore < 90 {
		t.Errorf("Empty cluster should have high stability, got %d", result.Summary.StabilityScore)
	}
}

// TestMTTRWithRestarts verifies restart tracking.
func TestMTTRWithRestarts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true, RestartCount: 5},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalRestarts != 5 {
		t.Errorf("Expected 5 total restarts, got %d", result.Summary.TotalRestarts)
	}
	if result.Summary.AffectedPods != 1 {
		t.Errorf("Expected 1 affected pod, got %d", result.Summary.AffectedPods)
	}
}

// TestMTTRWithCrashLoop verifies CrashLoopBackOff detection.
func TestMTTRWithCrashLoop(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashing", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						Ready:        false,
						RestartCount: 15,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalCrashLoops != 1 {
		t.Errorf("Expected 1 crash loop, got %d", result.Summary.TotalCrashLoops)
	}

	// Should detect burst (15 > 10 restarts)
	if !result.IncidentFrequency.BurstDetected {
		t.Error("Should detect restart burst with 15 restarts")
	}

	// Should have low stability
	if result.Summary.StabilityScore >= 50 {
		t.Errorf("Stability should be low with crash loops, got %d", result.Summary.StabilityScore)
	}
}

// TestMTTRESTimate verifies MTTR estimation structure.
func TestMTTRESTimate(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Confidence should be valid
	validConf := map[string]bool{"high": true, "medium": true, "low": true}
	if !validConf[result.MTTREstimate.Confidence] {
		t.Errorf("MTTR confidence invalid: %s", result.MTTREstimate.Confidence)
	}

	// Method should be non-empty
	if result.MTTREstimate.Method == "" {
		t.Error("MTTR method should not be empty")
	}
}

// TestMTTRNamespaceAnalysis verifies per-namespace stats.
func TestMTTRNamespaceAnalysis(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns-a"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", Ready: true, RestartCount: 10},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns-b"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", Ready: true, RestartCount: 2},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have 2 namespaces
	if len(result.ByNamespace) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(result.ByNamespace))
	}

	// ns-a should be first (more restarts)
	if len(result.ByNamespace) > 0 {
		if result.ByNamespace[0].Namespace != "ns-a" {
			t.Errorf("Expected ns-a first (10 restarts), got %s", result.ByNamespace[0].Namespace)
		}
	}

	// ns-a should have lower stability than ns-b
	nsA := NSMTTR{}
	nsB := NSMTTR{}
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "ns-a" {
			nsA = ns
		}
		if ns.Namespace == "ns-b" {
			nsB = ns
		}
	}
	if nsA.Stability >= nsB.Stability {
		t.Errorf("ns-a stability (%d) should be lower than ns-b (%d)", nsA.Stability, nsB.Stability)
	}
}

// TestMTTRRecommendations verifies recommendations generation.
func TestMTTRRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						Ready:        false,
						RestartCount: 20,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/mttr", clientset)
	w := httptest.NewRecorder()

	s.handleMTTR(w, req)

	var result MTTRResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have recommendations mentioning crash loops
	foundCrashLoopRec := false
	for _, rec := range result.Recommendations {
		if rec != "" && (containsStr(rec, "CrashLoop") || containsStr(rec, "crash")) {
			foundCrashLoopRec = true
		}
	}
	if !foundCrashLoopRec {
		t.Error("Should have recommendation mentioning crash loops")
	}
}

// TestMttrFormatDuration verifies duration formatting.
func TestMttrFormatDuration(t *testing.T) {
	tests := []struct {
		seconds float64
	}{
		{0},
		{30},
		{90},
		{3600},
		{7200},
	}
	for _, tc := range tests {
		got := mttrFormatDuration(tc.seconds)
		if got == "" {
			t.Errorf("mttrFormatDuration(%f) should not be empty", tc.seconds)
		}
	}
}


