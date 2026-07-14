package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTrafficPolicyScore(t *testing.T) {
	tests := []struct {
		name     string
		s        TrafficPolicySummary
		minScore int
		maxScore int
	}{
		{"no services", TrafficPolicySummary{}, 100, 100},
		{"all optimal", TrafficPolicySummary{TotalServices: 10, ExtTrafficLocal: 5, ClusterIP: 5}, 95, 100},
		{"some issues", TrafficPolicySummary{TotalServices: 10, ExtTrafficCluster: 3, HasExternalIPs: 2, OverExposed: 2}, 75, 90},
		{"many issues", TrafficPolicySummary{TotalServices: 10, ExtTrafficCluster: 8, HasExternalIPs: 5, PublishNotReady: 3}, 40, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := trafficPolicyScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestTrafficPolicyRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		recs := trafficPolicyRecommendations(TrafficPolicySummary{TotalServices: 10, ExtTrafficLocal: 5, ClusterIP: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := trafficPolicyRecommendations(TrafficPolicySummary{
			ExtTrafficCluster: 3,
			HasExternalIPs:    2,
			OverExposed:       1,
			PublishNotReady:   1,
			ExternalName:      1,
		})
		if len(recs) < 5 {
			t.Errorf("expected at least 5 recommendations, got %d", len(recs))
		}
	})
}

func TestIsSystemNamespace(t *testing.T) {
	if !isSystemNamespace("kube-system") {
		t.Error("kube-system should be system namespace")
	}
	if isSystemNamespace("production") {
		t.Error("production should not be system namespace")
	}
}

func TestTrafficPolicyAuditCore(t *testing.T) {
	services := []corev1.Service{
		// ClusterIP — optimal
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-internal", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				SessionAffinity: corev1.ServiceAffinityNone,
				Selector:        map[string]string{"app": "web"},
			},
		},
		// LoadBalancer with externalTrafficPolicy=Cluster — suboptimal
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-lb-cluster", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:                  corev1.ServiceTypeLoadBalancer,
				ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
				SessionAffinity:       corev1.ServiceAffinityNone,
				Selector:              map[string]string{"app": "api"},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}},
				},
			},
		},
		// LoadBalancer with externalTrafficPolicy=Local — optimal
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-lb-local", Namespace: "kube-system"},
			Spec: corev1.ServiceSpec{
				Type:                  corev1.ServiceTypeLoadBalancer,
				ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				SessionAffinity:       corev1.ServiceAffinityClientIP,
				Selector:              map[string]string{"app": "ingress"},
			},
		},
		// Service with external IPs
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-ext-ip", Namespace: "dev"},
			Spec: corev1.ServiceSpec{
				Type:        corev1.ServiceTypeClusterIP,
				ExternalIPs: []string{"192.168.1.100"},
				Selector:    map[string]string{"app": "test"},
			},
		},
		// ExternalName service
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-ext-name", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "db.example.com",
			},
		},
		// Service with publishNotReadyAddresses
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-not-ready", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:                     corev1.ServiceTypeClusterIP,
				PublishNotReadyAddresses: true,
				Selector:                 map[string]string{"app": "stateful"},
			},
		},
		// Service without selector
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-no-selector", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
			},
		},
	}

	result := trafficPolicyAuditCore(services)

	if result.Summary.TotalServices != 7 {
		t.Errorf("expected totalServices=7, got %d", result.Summary.TotalServices)
	}
	if result.Summary.ClusterIP != 4 {
		t.Errorf("expected clusterIP=4, got %d", result.Summary.ClusterIP)
	}
	if result.Summary.LoadBalancer != 2 {
		t.Errorf("expected loadBalancer=2, got %d", result.Summary.LoadBalancer)
	}
	if result.Summary.ExternalName != 1 {
		t.Errorf("expected externalName=1, got %d", result.Summary.ExternalName)
	}
	if result.Summary.ExtTrafficCluster != 1 {
		t.Errorf("expected extTrafficCluster=1, got %d", result.Summary.ExtTrafficCluster)
	}
	if result.Summary.ExtTrafficLocal != 1 {
		t.Errorf("expected extTrafficLocal=1, got %d", result.Summary.ExtTrafficLocal)
	}
	if result.Summary.HasExternalIPs != 1 {
		t.Errorf("expected hasExternalIPs=1, got %d", result.Summary.HasExternalIPs)
	}
	if result.Summary.SessionAffinityIP != 1 {
		t.Errorf("expected sessionAffinityIP=1, got %d", result.Summary.SessionAffinityIP)
	}
	if result.Summary.NoSelector != 2 {
		t.Errorf("expected noSelector=2, got %d", result.Summary.NoSelector)
	}
	if result.Summary.PublishNotReady != 1 {
		t.Errorf("expected publishNotReady=1, got %d", result.Summary.PublishNotReady)
	}
	// OverExposed: LB in non-system ns with external IP = 1
	if result.Summary.OverExposed != 1 {
		t.Errorf("expected overExposed=1, got %d", result.Summary.OverExposed)
	}
	// Should have issues
	if len(result.Issues) < 5 {
		t.Errorf("expected at least 5 issues, got %d", len(result.Issues))
	}
	// Should have namespace stats
	if len(result.ByNamespace) < 3 {
		t.Errorf("expected at least 3 namespace stats, got %d", len(result.ByNamespace))
	}
	// Should have recommendations
	if len(result.Recommendations) < 4 {
		t.Errorf("expected at least 4 recommendations, got %d", len(result.Recommendations))
	}
	// Health score should be degraded
	if result.HealthScore > 95 {
		t.Errorf("expected health score <= 95 due to issues, got %d", result.HealthScore)
	}
}
