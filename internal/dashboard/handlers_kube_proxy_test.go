package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeKubeProxyHealth_NoKubeProxy(t *testing.T) {
	result := analyzeKubeProxyHealth(nil, nil, []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			}}},
	}, nil, nil)

	if result.Summary.KubeProxyFound {
		t.Error("expected kube-proxy not found")
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90 (eBPF replacement), got %d", result.Score)
	}
	if len(result.NodeCoverage) != 1 {
		t.Errorf("expected 1 node coverage entry, got %d", len(result.NodeCoverage))
	}
}

func TestAnalyzeKubeProxyHealth_HealthyDS(t *testing.T) {
	ds := []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy", Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "registry.k8s.io/kube-proxy:v1.29.0"}}},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				CurrentNumberScheduled: 3,
				NumberReady:            3,
				UpdatedNumberScheduled: 3,
				NumberAvailable:        3,
			},
		},
	}

	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "n2"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "n3"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy-n1"}, Spec: corev1.PodSpec{NodeName: "n1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy-n2"}, Spec: corev1.PodSpec{NodeName: "n2"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy-n3"}, Spec: corev1.PodSpec{NodeName: "n3"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}

	result := analyzeKubeProxyHealth(ds, pods, nodes, nil, nil)

	if !result.Summary.KubeProxyFound {
		t.Error("expected kube-proxy found")
	}
	if result.Summary.ReadyNodes != 3 {
		t.Errorf("expected 3 ready nodes, got %d", result.Summary.ReadyNodes)
	}
	if result.Summary.MissingNodes != 0 {
		t.Errorf("expected 0 missing nodes, got %d", result.Summary.MissingNodes)
	}
	if result.Score < 95 {
		t.Errorf("expected score >= 95, got %d", result.Score)
	}
}

func TestAnalyzeKubeProxyHealth_MissingPods(t *testing.T) {
	ds := []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy", Namespace: "kube-system"},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				CurrentNumberScheduled: 2,
				NumberReady:            2,
			},
		},
	}

	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "n2"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "n3"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy-n1"}, Spec: corev1.PodSpec{NodeName: "n1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy-n2"}, Spec: corev1.PodSpec{NodeName: "n2"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}

	result := analyzeKubeProxyHealth(ds, pods, nodes, nil, nil)

	if result.Summary.MissingNodes != 1 {
		t.Errorf("expected 1 missing node, got %d", result.Summary.MissingNodes)
	}
	// Check for MissingProxy issue
	foundMissing := false
	for _, iss := range result.ConfigIssues {
		if iss.Type == "MissingProxy" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Error("expected MissingProxy issue for node n3")
	}
}

func TestAnalyzeKubeProxyHealth_ServiceRoutingTypes(t *testing.T) {
	svcs := []corev1.Service{
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1", Selector: map[string]string{"app": "test"}}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "None", Selector: map[string]string{"app": "headless"}}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"app": "np"}}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "lb"}}},
		{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName, ExternalName: "example.com"}},
	}

	result := analyzeKubeProxyHealth(nil, nil, nil, svcs, nil)

	if result.ServiceRouting.TotalServices != 5 {
		t.Errorf("expected 5 total services, got %d", result.ServiceRouting.TotalServices)
	}
	if result.ServiceRouting.ClusterIPServices != 1 {
		t.Errorf("expected 1 ClusterIP service, got %d", result.ServiceRouting.ClusterIPServices)
	}
	if result.ServiceRouting.HeadlessServices != 1 {
		t.Errorf("expected 1 headless service, got %d", result.ServiceRouting.HeadlessServices)
	}
	if result.ServiceRouting.NodePortServices != 1 {
		t.Errorf("expected 1 NodePort service, got %d", result.ServiceRouting.NodePortServices)
	}
	if result.ServiceRouting.LoadBalancerSVcs != 1 {
		t.Errorf("expected 1 LoadBalancer service, got %d", result.ServiceRouting.LoadBalancerSVcs)
	}
	if result.ServiceRouting.ExternalNameSVcs != 1 {
		t.Errorf("expected 1 ExternalName service, got %d", result.ServiceRouting.ExternalNameSVcs)
	}
}
