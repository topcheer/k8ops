package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	corev1res "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func makeForecastNode(name string, cpu, mem, pods string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    corev1res.MustParse(cpu),
				corev1.ResourceMemory: corev1res.MustParse(mem),
				corev1.ResourcePods:   corev1res.MustParse(pods),
			},
		},
	}
}

func makeForecastPod(name, ns string, cpu, mem string, createdAgo time.Duration) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-createdAgo)},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    corev1res.MustParse(cpu),
							corev1.ResourceMemory: corev1res.MustParse(mem),
						},
					},
				},
			},
		},
	}
}

// --- computeForecast tests ---

func TestComputeForecast_EmptyCluster(t *testing.T) {
	result := computeForecast(nil, nil, nil, nil)
	if result.NodeCount != 0 {
		t.Errorf("NodeCount = %d, want 0", result.NodeCount)
	}
	if len(result.Forecasts) != 3 { // cpu, memory, pods (no storage without PVCs)
		t.Errorf("Forecasts count = %d, want 3", len(result.Forecasts))
	}
}

func TestComputeForecast_HealthyCluster(t *testing.T) {
	nodes := []corev1.Node{
		makeForecastNode("node-1", "8", "16Gi", "110"),
		makeForecastNode("node-2", "8", "16Gi", "110"),
	}
	pods := []corev1.Pod{
		makeForecastPod("web-1", "default", "500m", "512Mi", 10*24*time.Hour),
		makeForecastPod("web-2", "default", "500m", "512Mi", 10*24*time.Hour),
		makeForecastPod("api-1", "default", "1000m", "1Gi", 5*24*time.Hour),
	}

	result := computeForecast(nodes, pods, nil, nil)

	if result.NodeCount != 2 {
		t.Errorf("NodeCount = %d, want 2", result.NodeCount)
	}
	if result.PodCount != 3 {
		t.Errorf("PodCount = %d, want 3", result.PodCount)
	}

	// CPU: 2000m allocated / 16000m capacity = 12.5%
	cpuForecast := findForecast(result.Forecasts, "cpu")
	if cpuForecast == nil {
		t.Fatal("cpu forecast missing")
	}
	if cpuForecast.RiskLevel != "safe" {
		t.Errorf("CPU risk = %s, want safe (12.5%% allocated)", cpuForecast.RiskLevel)
	}
	if cpuForecast.AllocatedPct < 10 || cpuForecast.AllocatedPct > 15 {
		t.Errorf("CPU allocated%% = %.1f, want ~12.5", cpuForecast.AllocatedPct)
	}
}

func TestComputeForecast_CriticalCluster(t *testing.T) {
	nodes := []corev1.Node{
		makeForecastNode("node-1", "2", "4Gi", "110"),
	}
	// Create many pods to saturate capacity
	pods := make([]corev1.Pod, 0)
	for i := 0; i < 100; i++ {
		pods = append(pods, makeForecastPod(
			"pod-"+string(rune(i)),
			"default",
			"19m",  // ~1900m total
			"38Mi", // ~3.7Gi total
			30*24*time.Hour,
		))
	}

	result := computeForecast(nodes, pods, nil, nil)

	cpuForecast := findForecast(result.Forecasts, "cpu")
	if cpuForecast == nil {
		t.Fatal("cpu forecast missing")
	}
	// 100 pods * 19m = 1900m, capacity = 2000m → 95% → critical
	if cpuForecast.RiskLevel != "critical" {
		t.Errorf("CPU risk = %s, want critical (95%%)", cpuForecast.RiskLevel)
	}
	if result.OverallRisk != "critical" {
		t.Errorf("OverallRisk = %s, want critical", result.OverallRisk)
	}
}

func TestComputeForecast_WithPVCs(t *testing.T) {
	nodes := []corev1.Node{
		makeForecastNode("node-1", "4", "8Gi", "110"),
	}
	pods := []corev1.Pod{
		makeForecastPod("app-1", "default", "500m", "512Mi", 5*24*time.Hour),
	}
	pvcs := []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "data-1", Namespace: "default"},
			Spec:       corev1.PersistentVolumeClaimSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "data-2", Namespace: "default"},
			Spec:       corev1.PersistentVolumeClaimSpec{},
		},
	}
	// Set storage requests via the correct API (VolumeResourceRequirements in k8s 0.36+)
	for i := range pvcs {
		pvcs[i].Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: corev1res.MustParse("15Gi"),
		}
	}

	result := computeForecast(nodes, pods, pvcs, nil)

	// Should have 4 forecasts: cpu, memory, pods, storage
	if len(result.Forecasts) != 4 {
		t.Errorf("Forecasts count = %d, want 4 (with storage)", len(result.Forecasts))
	}

	storage := findForecast(result.Forecasts, "storage")
	if storage == nil {
		t.Fatal("storage forecast missing")
	}
	// 30Gi PVCs used, ~90Gi capacity (3x estimate)
	if storage.Allocated == 0 {
		t.Error("storage allocated should be non-zero")
	}
}

// --- buildResourceForecast tests ---

func TestBuildResourceForecast_Safe(t *testing.T) {
	f := buildResourceForecast("cpu", 1000, 8000, 8000, 50.0)
	if f.RiskLevel != "safe" {
		t.Errorf("risk = %s, want safe", f.RiskLevel)
	}
	if f.AllocatedPct != 12.5 {
		t.Errorf("pct = %.1f, want 12.5", f.AllocatedPct)
	}
}

func TestBuildResourceForecast_Moderate(t *testing.T) {
	f := buildResourceForecast("memory", 6000, 10000, 10000, 100.0)
	if f.RiskLevel != "moderate" {
		t.Errorf("risk = %s, want moderate (60%%)", f.RiskLevel)
	}
}

func TestBuildResourceForecast_High(t *testing.T) {
	f := buildResourceForecast("cpu", 8500, 10000, 10000, 200.0)
	if f.RiskLevel != "high" {
		t.Errorf("risk = %s, want high (85%%)", f.RiskLevel)
	}
}

func TestBuildResourceForecast_Critical(t *testing.T) {
	f := buildResourceForecast("pods", 9600, 10000, 10000, 50.0)
	if f.RiskLevel != "critical" {
		t.Errorf("risk = %s, want critical (96%%)", f.RiskLevel)
	}
}

func TestBuildResourceForecast_Exhausted(t *testing.T) {
	f := buildResourceForecast("cpu", 10000, 10000, 10000, 10.0)
	if f.DaysToExhaust != 0 {
		t.Errorf("daysToExhaust = %d, want 0", f.DaysToExhaust)
	}
	if f.RiskLevel != "critical" {
		t.Errorf("risk = %s, want critical", f.RiskLevel)
	}
}

func TestBuildResourceForecast_NoGrowth(t *testing.T) {
	f := buildResourceForecast("memory", 5000, 10000, 10000, 0)
	if f.DaysToExhaust != -1 {
		t.Errorf("daysToExhaust = %d, want -1 (no growth)", f.DaysToExhaust)
	}
}

func TestBuildResourceForecast_FastExhaustion(t *testing.T) {
	// 100m remaining, 20m/day growth → 5 days to exhaustion
	f := buildResourceForecast("cpu", 7900, 8000, 8000, 20.0)
	if f.DaysToExhaust > 7 {
		t.Errorf("daysToExhaust = %d, want <=7", f.DaysToExhaust)
	}
	if f.RiskLevel != "critical" {
		t.Errorf("risk = %s, want critical (<7 days)", f.RiskLevel)
	}
	if f.ExhaustionDate == "" {
		t.Error("exhaustionDate should be set for fast exhaustion")
	}
}

// --- estimateGrowthRate tests ---

func TestEstimateGrowthRate_SufficientData(t *testing.T) {
	now := time.Now()
	times := make([]time.Time, 50)
	for i := range times {
		times[i] = now.Add(-time.Duration(50-i) * 24 * time.Hour)
	}

	rate := estimateGrowthRate(times)
	if rate.PodsPerDay <= 0 {
		t.Error("podsPerDay should be positive with growth data")
	}
	if rate.CPUPerDay <= 0 {
		t.Error("cpuPerDay should be positive")
	}
}

func TestEstimateGrowthRate_InsufficientData(t *testing.T) {
	rate := estimateGrowthRate([]time.Time{})
	if rate.PodsPerDay <= 0 {
		t.Error("should return conservative default even with no data")
	}
}

// --- Recommendation tests ---

func TestGenerateRecommendation_CriticalCPU(t *testing.T) {
	rec := generateRecommendation("cpu", "critical", 98, 3)
	if rec == "" {
		t.Error("recommendation should not be empty")
	}
}

func TestGenerateRecommendation_SafeMemory(t *testing.T) {
	rec := generateRecommendation("memory", "safe", 30, -1)
	if rec == "" {
		t.Error("recommendation should not be empty")
	}
}

func TestGenerateRecommendation_HighPods(t *testing.T) {
	rec := generateRecommendation("pods", "high", 85, 15)
	if rec == "" {
		t.Error("recommendation should not be empty")
	}
}

// --- Handler integration ---

func TestHandleCapacityForecast_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/capacity/forecast", clientset)
	rr := httptest.NewRecorder()

	s := &Server{}
	s.handleCapacityForecast(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHandleCapacityForecast_WithNodes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    corev1res.MustParse("4"),
					corev1.ResourceMemory: corev1res.MustParse("8Gi"),
					corev1.ResourcePods:   corev1res.MustParse("110"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app", Namespace: "default",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    corev1res.MustParse("500m"),
								corev1.ResourceMemory: corev1res.MustParse("512Mi"),
							},
						},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/capacity/forecast", clientset)
	rr := httptest.NewRecorder()

	s := &Server{}
	s.handleCapacityForecast(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "\"forecasts\"") {
		t.Error("response missing forecasts array")
	}
	if !strings.Contains(body, "\"overallRisk\"") {
		t.Error("response missing overallRisk")
	}
}

// --- Utility tests ---

func TestRoundTo2(t *testing.T) {
	if got := roundTo2(12.567); got != 12.57 {
		t.Errorf("roundTo2(12.567) = %.2f, want 12.57", got)
	}
	if got := roundTo2(0); got != 0 {
		t.Errorf("roundTo2(0) = %.2f, want 0", got)
	}
}

func TestForecastRiskScore(t *testing.T) {
	if forecastRiskScore(ResourceForecast{RiskLevel: "critical"}) != 0 {
		t.Error("critical should be 0")
	}
	if forecastRiskScore(ResourceForecast{RiskLevel: "safe"}) != 3 {
		t.Error("safe should be 3")
	}
}

func TestSortedForecasts(t *testing.T) {
	forecasts := []ResourceForecast{
		{Resource: "safe-res", RiskLevel: "safe"},
		{Resource: "critical-res", RiskLevel: "critical"},
		{Resource: "high-res", RiskLevel: "high"},
	}
	sorted := SortedForecasts(forecasts)
	if sorted[0].Resource != "critical-res" {
		t.Errorf("first should be critical, got %s", sorted[0].Resource)
	}
}

// --- Helper ---

func findForecast(forecasts []ResourceForecast, resource string) *ResourceForecast {
	for i := range forecasts {
		if forecasts[i].Resource == resource {
			return &forecasts[i]
		}
	}
	return nil
}
