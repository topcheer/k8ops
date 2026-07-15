package dashboard

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeCoreDNSHealth_NoCoreDNS(t *testing.T) {
	result := analyzeCoreDNSHealth(nil, nil, nil, []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}, nil)

	if result.Summary.CoreDNSFound {
		t.Error("expected CoreDNS not found")
	}
	if result.Score > 60 {
		t.Errorf("expected score <= 60 for no CoreDNS, got %d", result.Score)
	}
}

func TestAnalyzeCoreDNSHealth_HealthyDeployment(t *testing.T) {
	replicas := int32(2)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "registry.k8s.io/coredns:1.11.1"}}},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
		},
	}

	cms := []corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Data: map[string]string{
				"Corefile": ".:53 {\n    errors\n    health\n    ready\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n}\n",
			},
		},
	}

	result := analyzeCoreDNSHealth(deploys, nil, nil, nil, cms)

	if !result.Summary.CoreDNSFound {
		t.Error("expected CoreDNS found")
	}
	if result.Summary.ReadyReplicas != 2 {
		t.Errorf("expected 2 ready replicas, got %d", result.Summary.ReadyReplicas)
	}
	if !result.ConfigAnalysis.ErrorsPlugin {
		t.Error("expected errors plugin detected")
	}
	if !result.ConfigAnalysis.HealthPlugin {
		t.Error("expected health plugin detected")
	}
	if !result.ConfigAnalysis.ForwardPlugin {
		t.Error("expected forward plugin detected")
	}
	if !result.ConfigAnalysis.LoopPlugin {
		t.Error("expected loop plugin detected")
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90, got %d", result.Score)
	}
}

func TestAnalyzeCoreDNSHealth_MissingPlugins(t *testing.T) {
	replicas := int32(1)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
	}

	cms := []corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Data: map[string]string{
				"Corefile": ".:53 {\n    cache 30\n}\n",
			},
		},
	}

	result := analyzeCoreDNSHealth(deploys, nil, nil, nil, cms)

	// Should detect missing critical plugins
	foundForward := false
	foundLoop := false
	foundErrors := false
	for _, iss := range result.Issues {
		if iss.Type == "MissingPlugin" && strings.Contains(iss.Message, "forward") {
			foundForward = true
		}
		if iss.Type == "MissingPlugin" && strings.Contains(iss.Message, "loop") {
			foundLoop = true
		}
		if iss.Type == "MissingPlugin" && strings.Contains(iss.Message, "errors") {
			foundErrors = true
		}
	}
	if !foundForward {
		t.Error("expected missing forward plugin detection")
	}
	if !foundLoop {
		t.Error("expected missing loop plugin detection")
	}
	if !foundErrors {
		t.Error("expected missing errors plugin detection")
	}
}

func TestAnalyzeCoreDNSHealth_NodeLocalDNS(t *testing.T) {
	dss := []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-local-dns", Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "registry.k8s.io/dns/k8s-dns-node-cache:1.22.20"}}},
				},
			},
			Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
		},
	}

	result := analyzeCoreDNSHealth(nil, dss, nil, nil, nil)

	if !result.NodeLocalDNS.Deployed {
		t.Error("expected NodeLocal DNS detected")
	}
	if result.NodeLocalDNS.DesiredNodes != 3 {
		t.Errorf("expected 3 desired nodes, got %d", result.NodeLocalDNS.DesiredNodes)
	}
}
