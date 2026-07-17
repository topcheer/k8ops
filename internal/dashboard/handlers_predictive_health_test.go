package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestPredictiveHealthBasic verifies basic handler behavior with a simple cluster.
func TestPredictiveHealthBasic(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3500m"),
				corev1.ResourceMemory: resource.MustParse("15Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Containers: []corev1.Container{
				{
					Name:  "web",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:    corev1.PodRunning,
			QOSClass: corev1.PodQOSGuaranteed,
			ContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 0},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(node, pod)
	s := &Server{}

	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Summary.TotalNodes != 1 {
		t.Errorf("expected 1 node, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.TotalPods != 1 {
		t.Errorf("expected 1 pod, got %d", result.Summary.TotalPods)
	}
	if result.ForecastHorizon != "30 days" {
		t.Errorf("expected forecast horizon '30 days', got '%s'", result.ForecastHorizon)
	}
	if result.OverallRiskLevel != "low" {
		t.Errorf("expected low risk for healthy cluster, got '%s'", result.OverallRiskLevel)
	}
	if result.ConfidenceScore < 50 || result.ConfidenceScore > 100 {
		t.Errorf("expected confidence 50-100, got %d", result.ConfidenceScore)
	}
}

// TestPredictiveHealthMemoryPressure verifies prediction when a node has memory pressure.
func TestPredictiveHealthMemoryPressure(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "pressured-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("7Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(node)
	s := &Server{}

	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.NodeRisks) != 1 {
		t.Fatalf("expected 1 node risk, got %d", len(result.NodeRisks))
	}
	if result.NodeRisks[0].MemoryRisk != "critical" {
		t.Errorf("expected critical memory risk, got '%s'", result.NodeRisks[0].MemoryRisk)
	}
	if result.NodeRisks[0].RiskScore < 25 {
		t.Errorf("expected risk score >= 25 for pressured node, got %d", result.NodeRisks[0].RiskScore)
	}

	// Should have at least one prediction for this node
	hasPrediction := false
	for _, p := range result.Predictions {
		if p.Resource == "pressured-node" {
			hasPrediction = true
			if p.Severity != "critical" && p.Severity != "high" {
				t.Errorf("expected critical or high severity, got '%s'", p.Severity)
			}
		}
	}
	if !hasPrediction {
		t.Error("expected prediction for pressured node")
	}

	// Overall risk should be elevated
	if result.OverallRiskLevel == "low" {
		t.Error("expected elevated overall risk level for node with memory pressure")
	}
}

// TestPredictiveHealthRestartLoop verifies prediction for pods with high restart counts.
func TestPredictiveHealthRestartLoop(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
		},
	}

	crashingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "crashloop", Namespace: "prod"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 15},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(node, crashingPod)
	s := &Server{}

	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	found := false
	for _, pr := range result.PodRisks {
		if pr.Name == "crashloop" && pr.RiskType == "restart-loop" {
			found = true
			if pr.Severity != "critical" && pr.Severity != "high" {
				t.Errorf("expected critical/high severity for 15 restarts, got '%s'", pr.Severity)
			}
			if pr.RiskScore < 80 {
				t.Errorf("expected risk score >= 80 for 15 restarts, got %d", pr.RiskScore)
			}
		}
	}
	if !found {
		t.Error("expected restart-loop risk prediction for pod with 15 restarts")
	}

	if result.Summary.PodsAtRisk == 0 {
		t.Error("expected pods at risk > 0")
	}
}

// TestPredictiveHealthEmptyCluster verifies handler works with empty cluster.
func TestPredictiveHealthEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty cluster, got %d", w.Code)
	}

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Summary.TotalNodes != 0 {
		t.Errorf("expected 0 nodes, got %d", result.Summary.TotalNodes)
	}
	if result.OverallRiskLevel != "low" {
		t.Errorf("expected low risk for empty cluster, got '%s'", result.OverallRiskLevel)
	}
}

// TestPredictiveHealthDiskPressure verifies disk pressure detection.
func TestPredictiveHealthDiskPressure(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "disk-full"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(node)
	s := &Server{}

	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.NodeRisks) != 1 || result.NodeRisks[0].DiskRisk != "critical" {
		t.Errorf("expected critical disk risk, got %+v", result.NodeRisks)
	}
}

// TestPredictiveHealthHighPodDensity verifies capacity prediction for dense nodes.
func TestPredictiveHealthHighPodDensity(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "dense-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("100"),
			},
		},
	}

	// Create 90 pods to push density to 90%
	var pods []*corev1.Pod
	for i := 0; i < 90; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "dense-node",
				Containers: []corev1.Container{
					{
						Name:  "c",
						Image: "busybox",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		})
	}

	objects := []runtime.Object{node}
	for _, p := range pods {
		objects = append(objects, p)
	}
	clientset := k8sfake.NewSimpleClientset(objects...)
	s := &Server{}

	req := newReqWithClients("GET", "/api/operations/predictive-health", clientset)
	w := httptest.NewRecorder()

	s.handlePredictiveHealth(w, req)

	var result PredictiveHealthResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have a pod density trend
	foundTrend := false
	for _, rt := range result.ResourceTrends {
		if rt.Resource == "pod-density:dense-node" {
			foundTrend = true
			if rt.CurrentUsage < 85 {
				t.Errorf("expected density >= 85%%, got %.1f%%", rt.CurrentUsage)
			}
		}
	}
	if !foundTrend {
		t.Error("expected pod-density trend for node with 90 pods")
	}

	// Should have capacity prediction
	hasCapacity := false
	for _, p := range result.Predictions {
		if p.Category == "capacity-exhaustion" {
			hasCapacity = true
		}
	}
	if !hasCapacity {
		t.Error("expected capacity-exhaustion prediction for cluster with high pod density")
	}
}

// TestRiskTimeline verifies the timeline bucketing logic.
func TestRiskTimeline(t *testing.T) {
	predictions := []RiskPrediction{
		{Category: "node-failure", Resource: "n1", ETADays: 0.5, Severity: "critical"},
		{Category: "oom-kill", Resource: "p1", ETADays: 2, Severity: "high"},
		{Category: "cert-expiry", Resource: "secret/tls", ETADays: 15, Severity: "medium"},
		{Category: "capacity", Resource: "cluster", ETADays: 45, Severity: "low"},
	}

	timeline := buildRiskTimeline(predictions)

	if len(timeline) != 4 {
		t.Fatalf("expected 4 timeline buckets, got %d", len(timeline))
	}

	if timeline[0].When != "< 24h" || timeline[0].Count != 1 {
		t.Errorf("first bucket should be <24h with count 1, got %s/%d", timeline[0].When, timeline[0].Count)
	}
}

// TestSeverityFromScore verifies score-to-severity conversion.
func TestSeverityFromScore(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{10, "low"},
		{25, "medium"},
		{50, "high"},
		{70, "critical"},
		{100, "critical"},
		{0, "low"},
	}
	for _, tt := range tests {
		got := severityFromScore(tt.score)
		if got != tt.want {
			t.Errorf("severityFromScore(%d) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

// TestFormatDays verifies ETA formatting.
func TestFormatDays(t *testing.T) {
	tests := []struct {
		days float64
		want string
	}{
		{0.5, "< 1 day"},
		{1, "~1 day"},
		{3, "~3 days"},
		{10, "~10 days (1.4 weeks)"},
		{45, "~45 days (1.5 months)"},
	}
	for _, tt := range tests {
		got := formatDays(tt.days)
		if got != tt.want {
			t.Errorf("formatDays(%.1f) = %s, want %s", tt.days, got, tt.want)
		}
	}
}
