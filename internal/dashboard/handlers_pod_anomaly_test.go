package dashboard

import (
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"

	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestPodAnomalyEmpty verifies empty cluster behavior.
func TestPodAnomalyEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-anomaly", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handlePodAnomaly(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result PodAnomalyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.AnalyzedPods != 0 {
		t.Errorf("expected 0 analyzed pods, got %d", result.Summary.AnalyzedPods)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100, got %d", result.HealthScore)
	}
}

// TestPodAnomalyHighRestart verifies high-restart detection.
func TestPodAnomalyHighRestart(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "crash-pod",
				Namespace:         "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "crash-app"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", RestartCount: 15, Ready: true},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "stable-pod",
				Namespace:         "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "stable-app"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-2"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", RestartCount: 0, Ready: true},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-anomaly", clientset)
	w := httptest.NewRecorder()
	s.handlePodAnomaly(w, req)

	var result PodAnomalyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect the crash pod as anomalous
	found := false
	for _, a := range result.AnomalousPods {
		if a.Name == "crash-pod" && a.RestartCount == 15 {
			found = true
			if a.Severity != "critical" && a.Severity != "warning" {
				t.Errorf("expected warning/critical for 15 restarts, got %s", a.Severity)
			}
		}
	}
	if !found {
		t.Error("expected crash-pod to be flagged as anomalous")
	}
}

// TestPodAnomalyOutlierDetection verifies peer comparison.
func TestPodAnomalyOutlierDetection(t *testing.T) {
	// Create a Deployment with 3 pods, one with much higher restarts
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
		},
		// 3 pods from same workload, one is an outlier
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-pod-1", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-rs"}},
			},
			Spec: corev1.PodSpec{NodeName: "node-a"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "api", RestartCount: 1}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-pod-2", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-rs"}},
			},
			Spec: corev1.PodSpec{NodeName: "node-b"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "api", RestartCount: 0}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-pod-3", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-rs"}},
			},
			Spec: corev1.PodSpec{NodeName: "node-c"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "api", RestartCount: 12}}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-anomaly", clientset)
	w := httptest.NewRecorder()
	s.handlePodAnomaly(w, req)

	var result PodAnomalyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect outlier (api-pod-3 with 12 restarts vs peers 0-1)
	foundOutlier := false
	for _, a := range result.AnomalousPods {
		if a.Name == "api-pod-3" && a.AnomalyType == "outlier" {
			foundOutlier = true
		}
	}
	if !foundOutlier {
		t.Error("expected api-pod-3 to be detected as outlier")
	}

	// Should have peer stats with outlier flag
	hasOutlierStats := false
	for _, ps := range result.WorkloadPeerStats {
		if ps.HasOutlier {
			hasOutlierStats = true
		}
	}
	if !hasOutlierStats {
		t.Error("expected workload peer stats to show outlier")
	}
}

// TestPodAnomalyNoisyNeighbor verifies noisy neighbor detection.
func TestPodAnomalyNoisyNeighbor(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		// Noisy pod with high restarts on node with other pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "noisy-pod", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			},
			Spec: corev1.PodSpec{
				NodeName: "shared-node",
				Containers: []corev1.Container{
					{Name: "c", Image: "app:v1", SecurityContext: &corev1.SecurityContext{Privileged: &privileged}},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", RestartCount: 8}}},
		},
		// Victim pods on same node
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "victim-1", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			},
			Spec: corev1.PodSpec{NodeName: "shared-node"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", RestartCount: 0}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "victim-2", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			},
			Spec: corev1.PodSpec{NodeName: "shared-node"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", RestartCount: 1}}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-anomaly", clientset)
	w := httptest.NewRecorder()
	s.handlePodAnomaly(w, req)

	var result PodAnomalyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect noisy neighbor
	if len(result.NoisyNeighbors) == 0 {
		t.Error("expected noisy neighbor detection")
	}
	found := false
	for _, nn := range result.NoisyNeighbors {
		if nn.PodName == "noisy-pod" {
			found = true
			if len(nn.AffectedPods) == 0 {
				t.Error("expected affected pods list")
			}
		}
	}
	if !found {
		t.Error("expected noisy-pod to be in noisy neighbors list")
	}
}

// TestPodAnomalyNodeHotspot verifies node hotspot detection.
func TestPodAnomalyNodeHotspot(t *testing.T) {
	// Create a node with multiple high-restart pods
	var pods []runtime.Object
	for i := 0; i < 5; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("bad-pod-%d", i),
				Namespace:         "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			},
			Spec: corev1.PodSpec{NodeName: "bad-node"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", RestartCount: int32(6 + i)},
				},
			},
		})
	}
	// Add 3 normal pods on same node
	for i := 0; i < 3; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("ok-pod-%d", i),
				Namespace:         "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			},
			Spec: corev1.PodSpec{NodeName: "bad-node"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", RestartCount: 0},
				},
			},
		})
	}

	clientset := k8sfake.NewSimpleClientset(pods...)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-anomaly", clientset)
	w := httptest.NewRecorder()
	s.handlePodAnomaly(w, req)

	var result PodAnomalyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect bad-node as hotspot
	found := false
	for _, hs := range result.NodeHotspots {
		if hs.NodeName == "bad-node" {
			found = true
			if hs.AnomalousCount < 2 {
				t.Errorf("expected >=2 anomalous pods on hotspot, got %d", hs.AnomalousCount)
			}
		}
	}
	if !found {
		t.Error("expected bad-node to be flagged as hotspot")
	}
}

// TestClassifyRestartSeverity verifies severity classification.
func TestClassifyRestartSeverity(t *testing.T) {
	tests := []struct {
		restarts int
		age      time.Duration
		expected string
	}{
		{0, time.Hour, "info"},
		{3, time.Hour, "info"},
		{6, time.Hour, "warning"},
		{15, time.Hour, "critical"},
		{25, 30 * time.Minute, "critical"},
	}

	for _, tt := range tests {
		got := classifyRestartSeverity(tt.restarts, tt.age)
		if got != tt.expected {
			t.Errorf("classifyRestartSeverity(%d, %v) = %s, want %s", tt.restarts, tt.age, got, tt.expected)
		}
	}
}

// TestPodAnomalyRecommendations verifies recommendation generation.
func TestPodAnomalyRecommendations(t *testing.T) {
	result := PodAnomalyResult{
		Summary: PodAnomalySummary{
			AnomalousPods:   5,
			HighRestartPods: 3,
			AnomalyRate:     15.0,
		},
		NoisyNeighbors: []NoisyNeighbor{{PodName: "noisy"}},
		NodeHotspots:   []NodeHotspot{{NodeName: "bad-node", AnomalousCount: 4, AnomalyRate: 50}},
		HealthScore:    45,
	}

	recs := generateAnomalyRecommendations(result)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundAnomaly := false
	foundHotspot := false
	foundNoisy := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "anomalous") {
			foundAnomaly = true
		}
		if strings.Contains(lower, "hotspot") || strings.Contains(lower, "bad-node") {
			foundHotspot = true
		}
		if strings.Contains(lower, "noisy") {
			foundNoisy = true
		}
	}
	if !foundAnomaly {
		t.Error("expected anomaly recommendation")
	}
	if !foundHotspot {
		t.Error("expected hotspot recommendation")
	}
	if !foundNoisy {
		t.Error("expected noisy neighbor recommendation")
	}
}
