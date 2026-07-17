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

// TestCostIntelligenceEmptyCluster verifies the handler works on an empty cluster.
func TestCostIntelligenceEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	if result.Summary.CPUHourlyRate <= 0 {
		t.Error("CPUHourlyRate should be positive")
	}
}

// TestCostIntelligenceWithWorkloads verifies cost analysis with real workloads.
func TestCostIntelligenceWithWorkloads(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "staging"}},
		// default namespace: 2 pods with moderate requests
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1000m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				}},
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-2", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				}},
			}},
		},
		// production namespace: 3 pods with higher requests
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "production"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2000m"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8000m"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				}},
			}},
		},
		// staging: pod with no requests (underutilized)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-1", Namespace: "staging"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app"},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have 3 namespaces
	if len(result.ByNamespace) != 3 {
		t.Errorf("expected 3 namespaces, got %d", len(result.ByNamespace))
	}

	// Total pods should be 4
	if result.Summary.TotalPods != 4 {
		t.Errorf("expected 4 total pods, got %d", result.Summary.TotalPods)
	}

	// Monthly spend should be positive (production has 2 CPU + 4GB = significant)
	if result.Summary.MonthlySpend <= 0 {
		t.Error("MonthlySpend should be positive with resource requests")
	}

	// production should be the most expensive namespace (2 CPU, 4GB)
	if len(result.ByNamespace) > 0 {
		top := result.ByNamespace[0]
		if top.Namespace != "production" {
			t.Errorf("expected production to be top spender, got %s", top.Namespace)
		}
	}

	// Daily spend should be monthly / 30
	if result.Summary.MonthlySpend > 0 {
		expected := result.Summary.MonthlySpend / 30
		if diff := result.Summary.DailySpend - expected; diff > 1 || diff < -1 {
			t.Errorf("DailySpend %.2f should be MonthlySpend/30 = %.2f", result.Summary.DailySpend, expected)
		}
	}

	// Annual projection = monthly * 12
	expected := result.Summary.MonthlySpend * 12
	if diff := result.Summary.AnnualProjection - expected; diff > 1 || diff < -1 {
		t.Errorf("AnnualProjection %.2f should be %.2f", result.Summary.AnnualProjection, expected)
	}
}

// TestCostIntelligenceFinOpsScore verifies FinOps maturity scoring.
func TestCostIntelligenceFinOpsScore(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "default",
			Labels: map[string]string{"team": "platform"},
		}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	score := result.FinOpsScore

	// Score should be 0-100
	if score.Score < 0 || score.Score > 100 {
		t.Errorf("FinOps score should be 0-100, got %d", score.Score)
	}

	// Grade should be A-F
	validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
	if !validGrades[score.Grade] {
		t.Errorf("FinOps grade should be A-F, got %s", score.Grade)
	}

	// Sub-scores should be 0-100
	for _, sub := range []int{score.VisibilityScore, score.OptimizationScore, score.BudgetScore, score.EfficiencyScore, score.AllocationScore} {
		if sub < 0 || sub > 100 {
			t.Errorf("sub-score should be 0-100, got %d", sub)
		}
	}

	// With team labels, allocation score should be high
	if score.AllocationScore < 50 {
		t.Errorf("AllocationScore should be >= 50 with team labels, got %d", score.AllocationScore)
	}
}

// TestCostIntelligenceNamespaceSorting verifies namespaces sorted by cost descending.
func TestCostIntelligenceNamespaceSorting(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c"}},
		// ns-a: 1 CPU
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pa", Namespace: "ns-a"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1000m"),
					},
				}},
			}},
		},
		// ns-b: 3 CPU (should be most expensive)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pb", Namespace: "ns-b"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("3000m"),
					},
				}},
			}},
		},
		// ns-c: 0.5 CPU (cheapest)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pc", Namespace: "ns-c"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("500m"),
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should be sorted: ns-b (3 CPU) > ns-a (1 CPU) > ns-c (0.5 CPU)
	if len(result.ByNamespace) >= 3 {
		if result.ByNamespace[0].Namespace != "ns-b" {
			t.Errorf("expected ns-b to be #1, got %s", result.ByNamespace[0].Namespace)
		}
		if result.ByNamespace[1].Namespace != "ns-a" {
			t.Errorf("expected ns-a to be #2, got %s", result.ByNamespace[1].Namespace)
		}
		if result.ByNamespace[2].Namespace != "ns-c" {
			t.Errorf("expected ns-c to be #3, got %s", result.ByNamespace[2].Namespace)
		}
	}

	// PctOfSpend should sum to ~100%
	totalPct := 0.0
	for _, ns := range result.ByNamespace {
		totalPct += ns.PctOfSpend
	}
	if totalPct > 0 && (totalPct < 99 || totalPct > 101) {
		t.Errorf("PctOfSpend should sum to ~100%%, got %.1f%%", totalPct)
	}
}

// TestCostIntelligenceForecast verifies spend forecast structure.
func TestCostIntelligenceForecast(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("500m"),
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Forecast confidence should be valid
	validConf := map[string]bool{"high": true, "medium": true, "low": true}
	if !validConf[result.Forecast.Confidence] {
		t.Errorf("Forecast.Confidence invalid: %s", result.Forecast.Confidence)
	}

	// Growth rate should be bounded
	if result.Forecast.GrowthRate < 0 || result.Forecast.GrowthRate > 20 {
		t.Errorf("GrowthRate should be 0-20, got %.1f", result.Forecast.GrowthRate)
	}

	// Budget recommendation should include buffer
	if result.Forecast.ProjectedMonthly > 0 {
		if result.Forecast.BudgetRecommendation < result.Forecast.ProjectedMonthly {
			t.Error("BudgetRecommendation should include 10% buffer over ProjectedMonthly")
		}
	}
}

// TestCostIntelligenceSystemNSExclusion verifies system namespaces are excluded.
func TestCostIntelligenceSystemNSExclusion(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-pod", Namespace: "kube-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2000m"),
					},
				}},
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// kube-system should NOT appear in namespace list
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "kube-system" {
			t.Error("kube-system should be excluded from cost intelligence")
		}
	}

	// Total pods should only count non-system pods
	if result.Summary.TotalPods != 1 {
		t.Errorf("expected 1 pod (excluding kube-system), got %d", result.Summary.TotalPods)
	}
}

// TestCostIntelligenceAnomalies verifies anomaly detection with over-request pattern.
func TestCostIntelligenceAnomalies(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "expensive"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "big-app", Namespace: "expensive"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2000m"), // 20x ratio
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleCostIntelligence(w, req)

	var result CostIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Each anomaly should have valid fields
	validSev := map[string]bool{"critical": true, "warning": true, "info": true}
	for _, a := range result.Anomalies {
		if a.Type == "" {
			t.Error("Anomaly should have a type")
		}
		if !validSev[a.Severity] {
			t.Errorf("Anomaly severity invalid: %s", a.Severity)
		}
		if a.Detail == "" {
			t.Error("Anomaly should have a detail")
		}
	}
}

// TestGetRSSuffix tests ReplicaSet name suffix extraction.
func TestGetRSSuffix(t *testing.T) {
	tests := []struct {
		name   string
		expect string
	}{
		{"myapp-abc123def", "abc123def"},
		{"nginx-deployment-7f8b9c6d5", "7f8b9c6d5"},
		{"simple-pod", ""}, // no hash suffix
		{"test", ""},       // too short
		{"app-xyz", ""},    // too short suffix
		{"app-1a2b3c4d5e", "1a2b3c4d5e"},
	}

	for _, tc := range tests {
		got := getRSSuffix(tc.name)
		if got != tc.expect {
			t.Errorf("getRSSuffix(%q) = %q, expected %q", tc.name, got, tc.expect)
		}
	}
}

// TestRoundCost verifies rounding helper.
func TestRoundCost(t *testing.T) {
	tests := []struct {
		input  float64
		expect float64
	}{
		{123.456789, 123.46},
		{0.001, 0},
		{99.999, 100},
		{0, 0},
	}
	for _, tc := range tests {
		got := roundCost(tc.input)
		if got != tc.expect {
			t.Errorf("roundCost(%f) = %f, expected %f", tc.input, got, tc.expect)
		}
	}
}

// TestSafeDivCost verifies division helper.
func TestSafeDivCost(t *testing.T) {
	if safeDiv(10, 2) != 5 {
		t.Error("safeDiv(10, 2) should be 5")
	}
	if safeDiv(10, 0) != 0 {
		t.Error("safeDiv(10, 0) should be 0")
	}
}
