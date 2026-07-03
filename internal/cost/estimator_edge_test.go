package cost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// ===========================================================================
// Estimator — Edge Cases: Zero/Negative Pricing
// ===========================================================================

func TestSummary_ZeroPricing(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "app", "c",
			resource.MustParse("4"), resource.MustParse("16Gi"),
			noLimit, noLimit),
	)
	zeroPricing := Pricing{CPUPricePerCore: 0, RAMPricePerGB: 0, Currency: "USD"}
	est := NewEstimator(cs, zeroPricing)

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0.0, summary.TotalMonthlyCost)
	require.Len(t, summary.Namespaces, 1)
	assert.Equal(t, 0.0, summary.Namespaces[0].MonthlyCost)
}

func TestSummary_NegativePricing_ProducesNegativeCost(t *testing.T) {
	// Negative pricing is a misconfiguration, but the estimator doesn't validate it.
	// It should still compute (negative cost), not panic.
	cs := fake.NewSimpleClientset(
		makePod("ns", "pod1", "c",
			resource.MustParse("1"), resource.MustParse("1Gi"),
			noLimit, noLimit),
	)
	negPricing := Pricing{CPUPricePerCore: -10, RAMPricePerGB: -5, Currency: "USD"}
	est := NewEstimator(cs, negPricing)

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	assert.Less(t, summary.TotalMonthlyCost, 0.0)
}

// ===========================================================================
// Estimator — Edge Cases: Multi-Container, Workload Tracking
// ===========================================================================

func TestSummary_MultiContainerPod(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				}},
				{Name: "sidecar", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				}},
			}},
		},
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	require.Len(t, summary.Namespaces, 1)

	// CPU should be 0.5 + 0.1 = 0.6 cores
	assert.InDelta(t, 0.6, summary.Namespaces[0].CPURequested, 0.01)
	assert.Equal(t, 1, summary.Namespaces[0].Pods)
}

func TestSummary_WorkloadTracking(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-abc", Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "web-rs"},
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				}},
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-xyz", Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "StatefulSet", Name: "worker-sts"},
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("200m"),
					},
				}},
			}},
		},
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	require.Len(t, summary.Namespaces, 1)

	top := summary.Namespaces[0].TopWorkloads
	assert.Contains(t, top, "web-rs")
	assert.Contains(t, top, "worker-sts")
}

func TestSummary_TopWorkloadsLimitedTo5(t *testing.T) {
	var pods []*corev1.Pod
	for i := 0; i < 10; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-" + string(rune('a'+i)),
				Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "dep-" + string(rune('a'+i))},
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("10m"),
					},
				}},
			}},
		})
	}

	// Convert to runtime.Object slice for fake clientset
	objPods := make([]runtime.Object, len(pods))
	for i, p := range pods {
		objPods[i] = p
	}
	cs := fake.NewSimpleClientset(objPods...)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	require.Len(t, summary.Namespaces, 1)
	assert.LessOrEqual(t, len(summary.Namespaces[0].TopWorkloads), 5)
}

func TestSummary_PodWithNoResourceRequests(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c"},
			}},
		},
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	require.Len(t, summary.Namespaces, 1)
	assert.Equal(t, 0.0, summary.Namespaces[0].CPURequested)
	assert.Equal(t, 0.0, summary.Namespaces[0].RAMRequested)
	assert.Equal(t, 0.0, summary.Namespaces[0].MonthlyCost)
}

// ===========================================================================
// Estimator — Edge Cases: Large Cluster
// ===========================================================================

func TestSummary_LargeCluster(t *testing.T) {
	const numPods = 500
	pods := make([]runtime.Object, numPods)
	for i := 0; i < numPods; i++ {
		ns := "ns-" + string(rune('a'+i%10))
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-" + itoaSimple(i), Namespace: ns,
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				}},
			}},
		}
	}

	cs := fake.NewSimpleClientset(pods...)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, numPods, summary.TotalPods)
	assert.Len(t, summary.Namespaces, 10)
	assert.Greater(t, summary.TotalMonthlyCost, 0.0)
}

// ===========================================================================
// Recommendations — Edge Cases
// ===========================================================================

func TestRecommendations_BothCPUAndRAMOverProvisioned(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "greedy", "c",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("2"), resource.MustParse("4Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	require.Len(t, recs.Recommendations, 1)

	r := recs.Recommendations[0]
	assert.Contains(t, r.Reason, "CPU limit")
	assert.Contains(t, r.Reason, "RAM limit")
	assert.Contains(t, r.Reason, ";")
}

func TestRecommendations_ZeroPricing_NoSavingsReported(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "over", "c",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("4"), resource.MustParse("8Gi")),
	)
	zeroPricing := Pricing{CPUPricePerCore: 0, RAMPricePerGB: 0, Currency: "USD"}
	est := NewEstimator(cs, zeroPricing)

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	// With zero pricing, savings = 0, and the code skips savings <= 0
	assert.Equal(t, 0, recs.Count)
}

func TestRecommendations_LimitEqualsRequest(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "tight", "c",
			resource.MustParse("500m"), resource.MustParse("1Gi"),
			resource.MustParse("500m"), resource.MustParse("1Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, recs.Count)
}

func TestRecommendations_NoLimits(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "nolimits", "c",
			resource.MustParse("500m"), resource.MustParse("1Gi"),
			noLimit, noLimit),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, recs.Count)
}

func TestRecommendations_PricingReflected(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "api", "c",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("2"), resource.MustParse("8Gi")),
	)
	expensivePricing := Pricing{CPUPricePerCore: 1000, RAMPricePerGB: 500, Currency: "USD"}
	est := NewEstimator(cs, expensivePricing)

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	require.Len(t, recs.Recommendations, 1)
	assert.Greater(t, recs.Recommendations[0].MonthlySavings, 100.0)
}

// ===========================================================================
// Helper Function Tests
// ===========================================================================

func TestResourceToFloat_MissingKey(t *testing.T) {
	rl := corev1.ResourceList{}
	assert.Equal(t, 0.0, resourceToFloat(rl, corev1.ResourceCPU))
	assert.Equal(t, 0.0, resourceToFloat(rl, corev1.ResourceMemory))
}

func TestResourceToFloat_PresentKey(t *testing.T) {
	rl := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("500m"),
		corev1.ResourceMemory: resource.MustParse("1Gi"),
	}
	assert.InDelta(t, 0.5, resourceToFloat(rl, corev1.ResourceCPU), 0.001)
	// 1Gi = 1073741824 bytes
	assert.InDelta(t, 1.073741824e9, resourceToFloat(rl, corev1.ResourceMemory), 1.0)
}

func TestRoundTo(t *testing.T) {
	tests := []struct {
		val      float64
		decimals int
		want     float64
	}{
		{0.0, 2, 0.0},
		{1.2345, 2, 1.23},
		{1.2355, 2, 1.24},
		{1.99999, 3, 2.0},
		{0.001, 0, 0.0},
		{0.5, 0, 1.0}, // rounds up
		{-1.5, 0, -1.0},
		{1234567.89, 2, 1234567.89},
	}
	for _, tt := range tests {
		got := roundTo(tt.val, tt.decimals)
		assert.InDelta(t, tt.want, got, 0.01)
	}
}

func TestRoundTo_ZeroDecimals(t *testing.T) {
	assert.Equal(t, 5.0, roundTo(5.4, 0))
	assert.Equal(t, 6.0, roundTo(5.6, 0))
}

// ===========================================================================
// Cancelled Context
// ===========================================================================

func TestSummary_CancelledContext(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("ns", "pod", "c",
			resource.MustParse("1"), resource.MustParse("1Gi"),
			noLimit, noLimit),
	)
	est := NewEstimator(cs, DefaultPricing())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := est.Summary(ctx)
	// fake clientset may or may not honor context cancellation,
	// but the function should not panic.
	_ = err
}

// itoaSimple converts an int to string without importing strconv (avoid unused import).
func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
