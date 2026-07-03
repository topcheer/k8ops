package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// readyTrue returns a pointer to true for endpoint conditions.
func readyTrue() *bool {
	v := true
	return &v
}

func TestAnalyzeServiceHealth_Healthy(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "web"},
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	pods := []*corev1.Pod{
		makeNetPod("web-1", "default", map[string]string{"app": "web"}, true, true),
		makeNetPod("web-2", "default", map[string]string{"app": "web"}, true, true),
	}

	h := analyzeServiceHealth(svc, 2, 2, 0, pods)
	if h.Status != SvcHealthHealthy {
		t.Errorf("expected healthy, got %s", h.Status)
	}
	if h.MatchingPods != 2 {
		t.Errorf("expected 2 matching pods, got %d", h.MatchingPods)
	}
	if h.HealthyPods != 2 {
		t.Errorf("expected 2 healthy pods, got %d", h.HealthyPods)
	}
	if len(h.Ports) != 1 {
		t.Errorf("expected 1 port, got %d", len(h.Ports))
	}
	if h.Ports[0].TargetPort != "8080" {
		t.Errorf("expected targetPort 8080, got %s", h.Ports[0].TargetPort)
	}
}

func TestAnalyzeServiceHealth_Degraded(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "api"},
		},
	}

	h := analyzeServiceHealth(svc, 3, 2, 1, nil)
	if h.Status != SvcHealthDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}
	if len(h.Issues) == 0 {
		t.Error("expected at least one issue for degraded service")
	}
}

func TestAnalyzeServiceHealth_NoEndpoints(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "db"},
		},
	}

	pods := []*corev1.Pod{
		makeNetPod("db-1", "default", map[string]string{"app": "db"}, false, true),
		makeNetPod("db-2", "default", map[string]string{"app": "db"}, false, false),
	}

	h := analyzeServiceHealth(svc, 0, 0, 0, pods)
	if h.Status != SvcHealthNoEndpoints {
		t.Errorf("expected no-endpoints, got %s", h.Status)
	}
	if h.MatchingPods != 2 {
		t.Errorf("expected 2 matching pods, got %d", h.MatchingPods)
	}
	if h.HealthyPods != 0 {
		t.Errorf("expected 0 healthy pods, got %d", h.HealthyPods)
	}
}

func TestAnalyzeServiceHealth_Misconfigured(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "nonexistent"},
		},
	}

	h := analyzeServiceHealth(svc, 0, 0, 0, nil)
	if h.Status != SvcHealthMisconfigured {
		t.Errorf("expected misconfigured, got %s", h.Status)
	}
}

func TestAnalyzeServiceHealth_ExternalName(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ext", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "example.com",
		},
	}

	h := analyzeServiceHealth(svc, 0, 0, 0, nil)
	if h.Status != SvcHealthExternal {
		t.Errorf("expected external, got %s", h.Status)
	}
}

func TestAnalyzeServiceHealth_LoadBalancerPending(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "web"},
		},
		Status: corev1.ServiceStatus{},
	}

	pods := []*corev1.Pod{
		makeNetPod("web-1", "default", map[string]string{"app": "web"}, true, true),
	}

	h := analyzeServiceHealth(svc, 1, 1, 0, pods)
	if h.LoadBalancerIP != "" {
		t.Error("expected empty LoadBalancerIP")
	}
	found := false
	for _, issue := range h.Issues {
		if issue == "LoadBalancer service — no external IP assigned yet" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected issue about pending LB")
	}
}

func TestAnalyzeServiceHealth_LoadBalancerReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "web"},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "203.0.113.5"},
				},
			},
		},
	}

	pods := []*corev1.Pod{
		makeNetPod("web-1", "default", map[string]string{"app": "web"}, true, true),
	}

	h := analyzeServiceHealth(svc, 1, 1, 0, pods)
	if h.LoadBalancerIP != "203.0.113.5" {
		t.Errorf("expected LB IP 203.0.113.5, got %s", h.LoadBalancerIP)
	}
}

func TestAnalyzeServiceHealth_AllEndpointsNotReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "svc"},
		},
	}

	h := analyzeServiceHealth(svc, 3, 0, 3, nil)
	if h.Status != SvcHealthNoEndpoints {
		t.Errorf("expected no-endpoints (all not ready), got %s", h.Status)
	}
}

func TestAnalyzeServiceHealth_NoSelectorWithEndpoints(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "manual", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	h := analyzeServiceHealth(svc, 2, 2, 0, nil)
	if h.Status != SvcHealthHealthy {
		t.Errorf("expected healthy for no-selector service with endpoints, got %s", h.Status)
	}
}

func TestAnalyzeServiceHealth_PodsMatchButNoEndpoints(t *testing.T) {
	// Pods match selector and are ready, but endpoint controller hasn't created endpoints
	// This could happen with targetPort mismatch
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": "broken"},
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(9999)}, // pod doesn't listen on this port
			},
		},
	}

	pods := []*corev1.Pod{
		makeNetPod("broken-1", "default", map[string]string{"app": "broken"}, true, true),
	}

	h := analyzeServiceHealth(svc, 0, 0, 0, pods)
	if h.Status != SvcHealthMisconfigured {
		t.Errorf("expected misconfigured (pods match but no endpoints), got %s", h.Status)
	}
	if h.MatchingPods != 1 || h.HealthyPods != 1 {
		t.Errorf("expected 1 matching+healthy pod, got matching=%d healthy=%d", h.MatchingPods, h.HealthyPods)
	}
}

func TestAnalyzeIngressHealth_Healthy(t *testing.T) {
	portNum := int32(80)
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "web",
											Port: networkingv1.ServiceBackendPort{Number: portNum},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svcKeySet := map[string]bool{"default/web": true}
	endpointsBySvc := map[string]int{"default/web": 3}

	h := analyzeIngressHealth(ing, svcKeySet, endpointsBySvc)
	if !h.Healthy {
		t.Errorf("expected healthy ingress, issues: %v", h.Issues)
	}
	if len(h.Hosts) != 1 || h.Hosts[0] != "example.com" {
		t.Errorf("expected host example.com, got %v", h.Hosts)
	}
}

func TestAnalyzeIngressHealth_ServiceMissing(t *testing.T) {
	portNum := int32(80)
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "bad.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "missing-svc",
											Port: networkingv1.ServiceBackendPort{Number: portNum},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	h := analyzeIngressHealth(ing, map[string]bool{}, map[string]int{})
	if h.Healthy {
		t.Error("expected unhealthy ingress for missing service")
	}
	if len(h.Issues) == 0 {
		t.Error("expected issues for missing service backend")
	}
}

func TestAnalyzeIngressHealth_ServiceExistsNoEndpoints(t *testing.T) {
	portNum := int32(80)
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "ns"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/api",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api",
											Port: networkingv1.ServiceBackendPort{Number: portNum},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svcKeySet := map[string]bool{"ns/api": true}
	endpointsBySvc := map[string]int{}

	h := analyzeIngressHealth(ing, svcKeySet, endpointsBySvc)
	if h.Healthy {
		t.Error("expected unhealthy ingress for service with no endpoints")
	}
}

func TestAnalyzeIngressHealth_DefaultBackend(t *testing.T) {
	portNum := int32(8080)
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "default-ing", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "default-svc",
					Port: networkingv1.ServiceBackendPort{Number: portNum},
				},
			},
		},
	}

	h := analyzeIngressHealth(ing, map[string]bool{"default/default-svc": true}, map[string]int{"default/default-svc": 2})
	if !h.Healthy {
		t.Errorf("expected healthy, issues: %v", h.Issues)
	}
}

func TestSvcStatusRank(t *testing.T) {
	tests := []struct {
		status   SvcHealthStatus
		expected int
	}{
		{SvcHealthMisconfigured, 0},
		{SvcHealthNoEndpoints, 1},
		{SvcHealthDegraded, 2},
		{SvcHealthExternal, 3},
		{SvcHealthHealthy, 4},
	}

	for _, tt := range tests {
		got := svcStatusRank(tt.status)
		if got != tt.expected {
			t.Errorf("svcStatusRank(%s) = %d, want %d", tt.status, got, tt.expected)
		}
	}
}

func TestHasMatchingSelector(t *testing.T) {
	selector := map[string]string{"app": "web", "tier": "frontend"}

	if !hasMatchingSelector(selector, map[string]string{"app": "web", "tier": "frontend"}) {
		t.Error("should match when all labels present")
	}
	if hasMatchingSelector(selector, map[string]string{"app": "web"}) {
		t.Error("should not match when tier missing")
	}
	if hasMatchingSelector(selector, map[string]string{"app": "web", "tier": "backend"}) {
		t.Error("should not match when tier differs")
	}
	if !hasMatchingSelector(map[string]string{}, map[string]string{"app": "x"}) {
		t.Error("empty selector matches everything")
	}
}

func TestHandleNetworkingHealth_Integration(t *testing.T) {
	// Build fake clientset with services, endpoint slices, pods
	clientset := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "healthy"},
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(8080)}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dead-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "nonexistent"},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "ext-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "external.example.com",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-pod", Namespace: "default", Labels: map[string]string{"app": "healthy"}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	// Add endpoint slices
	_, err := clientset.DiscoveryV1().EndpointSlices("default").Create(
		context.Background(),
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "healthy-svc-abc",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "healthy-svc",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: readyTrue()}},
				{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: readyTrue()}},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create endpoint slice: %v", err)
	}

	// Test the endpoint slice indexing logic directly
	// (mimics what handleNetworkingHealth does internally)
	svcList, _ := clientset.CoreV1().Services("default").List(context.Background(), metav1.ListOptions{})
	epList, _ := clientset.DiscoveryV1().EndpointSlices("default").List(context.Background(), metav1.ListOptions{})
	podList, _ := clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})

	endpointsBySvc := make(map[string]int)
	readyBySvc := make(map[string]int)
	for i := range epList.Items {
		eps := &epList.Items[i]
		svcName := eps.Labels[discoveryv1.LabelServiceName]
		if svcName == "" {
			continue
		}
		for _, ep := range eps.Endpoints {
			endpointsBySvc[svcName]++
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				readyBySvc[svcName]++
			}
		}
	}

	podsByNs := make(map[string][]*corev1.Pod)
	for i := range podList.Items {
		podsByNs[podList.Items[i].Namespace] = append(podsByNs[podList.Items[i].Namespace], &podList.Items[i])
	}

	var healthyCount, deadCount, extCount int
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		h := analyzeServiceHealth(svc, endpointsBySvc[svc.Name], readyBySvc[svc.Name], 0, podsByNs[svc.Namespace])
		switch h.Status {
		case SvcHealthHealthy:
			healthyCount++
		case SvcHealthMisconfigured:
			deadCount++
		case SvcHealthExternal:
			extCount++
		}
	}

	if healthyCount != 1 {
		t.Errorf("expected 1 healthy service, got %d", healthyCount)
	}
	if deadCount != 1 {
		t.Errorf("expected 1 misconfigured service, got %d", deadCount)
	}
	if extCount != 1 {
		t.Errorf("expected 1 external service, got %d", extCount)
	}
}

func TestNetHealthResult_JSON(t *testing.T) {
	result := NetHealthResult{
		Summary: NetHealthSummary{
			TotalServices:    3,
			ByStatus:         map[string]int{"healthy": 2, "no-endpoints": 1},
			TotalIngresses:   2,
			UnhealthyIngress: 1,
			NoEndpointSvcs:   1,
		},
		Services: []ServiceHealth{
			{Name: "svc1", Namespace: "default", Status: SvcHealthHealthy, EndpointCount: 3},
			{Name: "svc2", Namespace: "default", Status: SvcHealthNoEndpoints, Issues: []string{"No pods match selector"}},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded NetHealthResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Summary.TotalServices != 3 {
		t.Errorf("expected 3 total services, got %d", decoded.Summary.TotalServices)
	}
	if len(decoded.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(decoded.Services))
	}
}

func TestJoinIssues(t *testing.T) {
	if got := joinIssues([]string{}); got != "" {
		t.Errorf("empty should return empty, got %q", got)
	}
	if got := joinIssues([]string{"a", "b"}); got != "a; b" {
		t.Errorf("expected 'a; b', got %q", got)
	}
}

func TestIsPodReady(t *testing.T) {
	readyPod := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	if !isPodReady(readyPod) {
		t.Error("pod with Ready=True should be ready")
	}

	notReadyPod := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	if isPodReady(notReadyPod) {
		t.Error("pod with Ready=False should not be ready")
	}

	noCondPod := &corev1.Pod{}
	if isPodReady(noCondPod) {
		t.Error("pod with no conditions should not be ready")
	}
}

func TestFormatSvcKey(t *testing.T) {
	if got := formatSvcKey("default", "web"); got != "default/web" {
		t.Errorf("expected 'default/web', got %q", got)
	}
}

func TestHandleNetworkingHealthHandler(t *testing.T) {
	// Verify the handler is wired to Server properly
	srv := &Server{}

	// Test that the method exists and is callable
	_ = http.HandlerFunc(srv.handleNetworkingHealth)
}

// makeNetPod creates a pod with the given labels and readiness state.
func makeNetPod(name, namespace string, labels map[string]string, ready, running bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
	}
	if running {
		pod.Status.Phase = corev1.PodRunning
	} else {
		pod.Status.Phase = corev1.PodPending
	}
	podReadyStatus := corev1.ConditionFalse
	if ready {
		podReadyStatus = corev1.ConditionTrue
	}
	pod.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodReady, Status: podReadyStatus},
	}
	return pod
}

// Ensure no unused imports
var _ = fmt.Sprintf
