package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestGoldenSignalsEmptyCluster verifies the handler works on an empty cluster.
func TestGoldenSignalsEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	// Should have exactly 4 signals
	if len(result.Signals) != 4 {
		t.Errorf("expected 4 signals, got %d", len(result.Signals))
	}
}

// TestGoldenSignalsFourSignals verifies all four golden signals are present.
func TestGoldenSignalsFourSignals(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expectedNames := map[string]bool{"latency": false, "traffic": false, "errors": false, "saturation": false}
	for _, sig := range result.Signals {
		if _, ok := expectedNames[sig.Name]; ok {
			expectedNames[sig.Name] = true
		}
		// Score should be 0-100
		if sig.Score < 0 || sig.Score > 100 {
			t.Errorf("Signal %s score should be 0-100, got %d", sig.Name, sig.Score)
		}
		// Status should be valid
		validStatus := map[string]bool{"healthy": true, "warning": true, "critical": true}
		if !validStatus[sig.Status] {
			t.Errorf("Signal %s status invalid: %s", sig.Name, sig.Status)
		}
		// Should have metrics
		if len(sig.Metrics) == 0 {
			t.Errorf("Signal %s should have metrics", sig.Name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Signal %s not found", name)
		}
	}
}

// TestGoldenSignalsWeakestLink verifies overall score = minimum signal score.
func TestGoldenSignalsWeakestLink(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		// Add a CrashLoopBackOff pod to lower error score
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashing", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						Ready:        false,
						RestartCount: 10,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Find minimum signal score
	minScore := 100
	for _, sig := range result.Signals {
		if sig.Score < minScore {
			minScore = sig.Score
		}
	}

	if result.OverallScore != minScore {
		t.Errorf("Overall score should be min signal score (%d), got %d", minScore, result.OverallScore)
	}

	// Error signal should be the weakest due to CrashLoopBackOff
	errorScore := 100
	for _, sig := range result.Signals {
		if sig.Name == "errors" {
			errorScore = sig.Score
		}
	}
	if errorScore >= 100 {
		t.Error("Error signal should be degraded with CrashLoopBackOff pod")
	}
}

// TestGoldenSignalsGrade verifies grade mapping.
func TestGoldenSignalsGrade(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
	if !validGrades[result.OverallGrade] {
		t.Errorf("Overall grade should be A-F, got %s", result.OverallGrade)
	}

	// Verify grade matches score
	expectedGrade := goldenScoreToGrade(result.OverallScore)
	if result.OverallGrade != expectedGrade {
		t.Errorf("Grade should be %s for score %d, got %s", expectedGrade, result.OverallScore, result.OverallGrade)
	}
}

// TestGoldenSignalsWithHealthyPods verifies signals are healthy with good pods.
func TestGoldenSignalsWithHealthyPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				}},
			}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true, RestartCount: 0},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// With healthy pods, overall score should be reasonably high
	if result.OverallScore < 70 {
		t.Errorf("Overall score should be >= 70 with healthy pods, got %d", result.OverallScore)
	}

	// Namespace analysis should show the namespace
	found := false
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "default" {
			found = true
			if ns.Overall < 70 {
				t.Errorf("Namespace default overall score should be >= 70, got %d", ns.Overall)
			}
		}
	}
	if !found {
		t.Error("default namespace should appear in byNamespace")
	}
}

// TestGoldenSignalsNamespaceAnalysis verifies per-namespace signal scoring.
func TestGoldenSignalsNamespaceAnalysis(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "good-ns"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "bad-ns"}},
		// Good namespace: healthy pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "good-ns"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				}},
			}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", Ready: true, RestartCount: 0},
				},
			},
		},
		// Bad namespace: crashing pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "bad-ns"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "c",
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
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Find namespaces in result
	goodNS := GoldenNS{}
	badNS := GoldenNS{}
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "good-ns" {
			goodNS = ns
		}
		if ns.Namespace == "bad-ns" {
			badNS = ns
		}
	}

	// Good namespace should score higher than bad namespace
	if goodNS.Overall <= badNS.Overall {
		t.Errorf("Good namespace should score higher: good=%d, bad=%d", goodNS.Overall, badNS.Overall)
	}

	// Bad namespace should have low error score (one crashloop + 15 restarts → warning level)
	if badNS.Errors >= 75 {
		t.Errorf("Bad namespace error score should be degraded, got %d", badNS.Errors)
	}
}

// TestGoldenSignalsCrossSignalIssues verifies issue detection.
func TestGoldenSignalsCrossSignalIssues(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "critical"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "critical"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "c",
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
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect at least one cross-signal issue
	if len(result.TopIssues) == 0 {
		t.Error("Should detect cross-signal issues with degraded cluster")
	}

	// Each issue should have valid fields
	for _, iss := range result.TopIssues {
		if len(iss.Signals) == 0 {
			t.Error("Issue should reference at least one signal")
		}
		if iss.Title == "" {
			t.Error("Issue should have a title")
		}
		if iss.Resolution == "" {
			t.Error("Issue should have a resolution")
		}
	}
}

// TestGoldenSignalsRecommendations verifies recommendations are generated.
func TestGoldenSignalsRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/golden-signals", clientset)
	w := httptest.NewRecorder()

	s.handleGoldenSignals(w, req)

	var result GoldenSignalsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have at least the weakest-signal recommendation
	if len(result.Recommendations) == 0 {
		t.Error("Should generate at least one recommendation")
	}
}
