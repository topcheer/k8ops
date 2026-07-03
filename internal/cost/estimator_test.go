package cost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ===========================================================================
// Pricing Tests
// ===========================================================================

func TestDefaultPricing(t *testing.T) {
	p := DefaultPricing()
	assert.Equal(t, 28.0, p.CPUPricePerCore)
	assert.Equal(t, 3.5, p.RAMPricePerGB)
	assert.Equal(t, "USD", p.Currency)
}

func TestCloudPresets(t *testing.T) {
	tests := []struct {
		name string
		fn   func() Pricing
	}{
		{"AWS", AWSPricing},
		{"Azure", AzurePricing},
		{"GCP", GCPPricing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.fn()
			assert.Greater(t, p.CPUPricePerCore, 0.0)
			assert.Greater(t, p.RAMPricePerGB, 0.0)
			assert.Equal(t, "USD", p.Currency)
		})
	}
}

func TestAWSDiffersFromAzure(t *testing.T) {
	aws := AWSPricing()
	azure := AzurePricing()
	assert.NotEqual(t, aws.CPUPricePerCore, azure.CPUPricePerCore,
		"AWS and Azure should have different CPU prices")
}

// ===========================================================================
// Estimator — Summary Tests
// ===========================================================================

func TestSummary_EmptyCluster(t *testing.T) {
	cs := fake.NewSimpleClientset()
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0.0, summary.TotalMonthlyCost)
	assert.Empty(t, summary.Namespaces)
	assert.Equal(t, 0, summary.TotalPods)
}

func TestSummary_SingleNamespace(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("default", "app-1", "main",
			resource.MustParse("500m"), resource.MustParse("512Mi"),
			resource.MustParse("1"), resource.MustParse("2Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)

	require.Len(t, summary.Namespaces, 1)
	ns := summary.Namespaces[0]
	assert.Equal(t, "default", ns.Namespace)
	assert.Equal(t, 1, ns.Pods)
	assert.InDelta(t, 0.5, ns.CPURequested, 0.001) // 500m = 0.5 core
	assert.InDelta(t, 0.5, ns.RAMRequested, 0.05)  // 512Mi ≈ 0.537 GB

	// Expected: 0.5 * 28 + 0.537 * 3.5 ≈ 14 + 1.88 = 15.88
	assert.InDelta(t, 16.0, ns.MonthlyCost, 0.5)
	assert.InDelta(t, 15.75, summary.TotalMonthlyCost, 0.5)
	assert.Equal(t, 100.0, ns.Percentage)
}

func TestSummary_MultipleNamespaces(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("prod", "api", "main",
			resource.MustParse("1000m"), resource.MustParse("1Gi"),
			noLimit, noLimit),
		makePod("prod", "worker", "main",
			resource.MustParse("500m"), resource.MustParse("2Gi"),
			noLimit, noLimit),
		makePod("dev", "test", "main",
			resource.MustParse("100m"), resource.MustParse("256Mi"),
			noLimit, noLimit),
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)

	require.Len(t, summary.Namespaces, 2)

	// prod should be first (higher cost)
	assert.Equal(t, "prod", summary.Namespaces[0].Namespace)
	assert.Equal(t, 2, summary.Namespaces[0].Pods)
	assert.Equal(t, "dev", summary.Namespaces[1].Namespace)
	assert.Equal(t, 1, summary.Namespaces[1].Pods)

	// prod: (1+0.5)*28 + (1+2)*3.5 = 42 + 10.5 = 52.5
	assert.InDelta(t, 52.5, summary.Namespaces[0].MonthlyCost, 1.0)
	// dev: 0.1*28 + 0.25*3.5 = 2.8 + 0.875 = 3.675
	assert.InDelta(t, 3.7, summary.Namespaces[1].MonthlyCost, 0.5)

	// Percentages should sum to 100
	totalPct := summary.Namespaces[0].Percentage + summary.Namespaces[1].Percentage
	assert.InDelta(t, 100.0, totalPct, 0.5)
}

func TestSummary_SortedByCostDesc(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("ns-cheap", "cheap", "c",
			resource.MustParse("10m"), resource.MustParse("32Mi"),
			noLimit, noLimit),
		makePod("ns-expensive", "expensive", "c",
			resource.MustParse("4"), resource.MustParse("8Gi"),
			noLimit, noLimit),
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)

	// ns-expensive should be first despite being added second
	assert.Equal(t, "ns-expensive", summary.Namespaces[0].Namespace)
	assert.Equal(t, "ns-cheap", summary.Namespaces[1].Namespace)
}

func TestSummary_SkipsSystemNamespaces(t *testing.T) {
	// Pods with empty namespace should be skipped
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "no-ns", Namespace: ""},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				}},
			}},
		},
	)
	est := NewEstimator(cs, DefaultPricing())

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)
	assert.Empty(t, summary.Namespaces)
	assert.Equal(t, 0.0, summary.TotalMonthlyCost)
}

func TestSummary_PricingReflected(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("default", "app", "c",
			resource.MustParse("1"), resource.MustParse("1Gi"),
			noLimit, noLimit),
	)
	expensive := Pricing{CPUPricePerCore: 100, RAMPricePerGB: 10, Currency: "USD"}
	est := NewEstimator(cs, expensive)

	summary, err := est.Summary(context.Background())
	require.NoError(t, err)

	// 1*100 + 1.073GB*10 ≈ 110.74
	assert.InDelta(t, 110.0, summary.TotalMonthlyCost, 1.0)
	assert.Equal(t, 100.0, summary.Pricing.CPUPricePerCore)
}

// ===========================================================================
// Estimator — Recommendations Tests
// ===========================================================================

func TestRecommendations_NoOverProvisioning(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePod("default", "app", "c",
			resource.MustParse("500m"), resource.MustParse("512Mi"),
			resource.MustParse("1"), resource.MustParse("1Gi")), // limit = 2x request, OK
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, recs.Count)
	assert.Empty(t, recs.Recommendations)
	assert.Equal(t, 0.0, recs.TotalPotentialSavings)
}

func TestRecommendations_OverProvisionedCPU(t *testing.T) {
	// request 100m, limit 1 core (10x → over-provisioned)
	cs := fake.NewSimpleClientset(
		makePod("prod", "api", "main",
			resource.MustParse("100m"), resource.MustParse("256Mi"),
			resource.MustParse("1"), resource.MustParse("512Mi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)

	require.Len(t, recs.Recommendations, 1)
	r := recs.Recommendations[0]
	assert.Equal(t, "prod/api", r.Workload)
	assert.Equal(t, "main", r.ContainerName)
	assert.Greater(t, r.MonthlySavings, 0.0)
	assert.Greater(t, r.SavingsPct, 0.0)
	assert.Contains(t, r.Reason, "CPU limit")

	assert.Equal(t, recs.TotalPotentialSavings, r.MonthlySavings)
}

func TestRecommendations_OverProvisionedRAM(t *testing.T) {
	// request 128Mi, limit 4Gi (32x → over-provisioned)
	cs := fake.NewSimpleClientset(
		makePod("prod", "worker", "main",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("200m"), resource.MustParse("4Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)

	require.Len(t, recs.Recommendations, 1)
	assert.Contains(t, recs.Recommendations[0].Reason, "RAM limit")
}

func TestRecommendations_SortedBySavingsDesc(t *testing.T) {
	cs := fake.NewSimpleClientset(
		// Small savings: request 100m, limit 400m (4x)
		makePod("ns-a", "small", "c",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("400m"), resource.MustParse("512Mi")),
		// Big savings: request 100m, limit 2 cores (20x)
		makePod("ns-b", "big", "c",
			resource.MustParse("100m"), resource.MustParse("128Mi"),
			resource.MustParse("2"), resource.MustParse("8Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	require.Len(t, recs.Recommendations, 2)

	// "big" should be first (higher savings)
	assert.Equal(t, "ns-b/big", recs.Recommendations[0].Workload)
	assert.Equal(t, "ns-a/small", recs.Recommendations[1].Workload)
	assert.Greater(t,
		recs.Recommendations[0].MonthlySavings,
		recs.Recommendations[1].MonthlySavings,
	)
}

func TestRecommendations_NoRequests(t *testing.T) {
	// Pod with no resource requests — should be skipped
	cs := fake.NewSimpleClientset(
		makePod("default", "no-reqs", "c",
			resource.MustParse("0"), resource.MustParse("0"),
			resource.MustParse("1"), resource.MustParse("1Gi")),
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, recs.Count)
}

func TestRecommendations_MultipleContainers(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					}},
					{Name: "c2", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("400m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					}},
				},
			},
		},
	)
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)

	// Only c1 is over-provisioned (c2 has limit = 2x request)
	require.Len(t, recs.Recommendations, 1)
	assert.Equal(t, "c1", recs.Recommendations[0].ContainerName)
}

func TestRecommendations_EmptyCluster(t *testing.T) {
	cs := fake.NewSimpleClientset()
	est := NewEstimator(cs, DefaultPricing())

	recs, err := est.Recommendations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, recs.Count)
	assert.Empty(t, recs.Recommendations)
}

// ===========================================================================
// Helpers
// ===========================================================================

// makePod creates a pod with a single container with the given resource requests/limits.
// Pass resource.Quantity{} (zero value) to omit a request or limit.
func makePod(namespace, name, containerName string,
	reqCPU, reqRAM, limCPU, limRAM resource.Quantity,
) *corev1.Pod {
	containers := []corev1.Container{{
		Name:      containerName,
		Resources: corev1.ResourceRequirements{},
	}}

	if !reqCPU.IsZero() || !reqRAM.IsZero() {
		containers[0].Resources.Requests = corev1.ResourceList{}
		if !reqCPU.IsZero() {
			containers[0].Resources.Requests[corev1.ResourceCPU] = reqCPU
		}
		if !reqRAM.IsZero() {
			containers[0].Resources.Requests[corev1.ResourceMemory] = reqRAM
		}
	}
	if !limCPU.IsZero() || !limRAM.IsZero() {
		containers[0].Resources.Limits = corev1.ResourceList{}
		if !limCPU.IsZero() {
			containers[0].Resources.Limits[corev1.ResourceCPU] = limCPU
		}
		if !limRAM.IsZero() {
			containers[0].Resources.Limits[corev1.ResourceMemory] = limRAM
		}
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.PodSpec{Containers: containers},
	}
}

// noLimit is a zero-value resource.Quantity indicating no limit/request.
var noLimit = resource.Quantity{}
