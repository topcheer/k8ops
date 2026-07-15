package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func mkResList(cpu, mem, pods string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
		corev1.ResourcePods:   resource.MustParse(pods),
	}
}

func TestAnalyzeReservations_Normal(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"}},
			Status: corev1.NodeStatus{
				Capacity:    mkResList("2", "8Gi", "50"),
				Allocatable: mkResList("1.9", "7.2Gi", "48"),
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	}

	result := analyzeReservations(nodes)

	if result.Summary.TotalNodes != 1 {
		t.Errorf("expected 1 node, got %d", result.Summary.TotalNodes)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Nodes))
	}
	if result.Nodes[0].Status != "normal" {
		t.Errorf("expected normal status, got %s", result.Nodes[0].Status)
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90, got %d", result.Score)
	}
}

func TestAnalyzeReservations_OverReserved(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "over"},
			Status: corev1.NodeStatus{
				Capacity:    mkResList("4", "16Gi", "100"),
				Allocatable: mkResList("2.5", "10Gi", "80"), // ~37% reserved
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	}

	result := analyzeReservations(nodes)

	if result.Nodes[0].Status != "over-reserved" {
		t.Errorf("expected over-reserved, got %s", result.Nodes[0].Status)
	}
	if result.Nodes[0].ResvPctCPU < 30 {
		t.Errorf("expected >30%% CPU reserved, got %.1f%%", result.Nodes[0].ResvPctCPU)
	}
	found := false
	for _, iss := range result.Issues {
		if iss.Type == "OverReserved" {
			found = true
		}
	}
	if !found {
		t.Error("expected OverReserved issue")
	}
}

func TestAnalyzeReservations_UnderReserved(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "under"},
			Status: corev1.NodeStatus{
				Capacity:    mkResList("4", "16Gi", "100"),
				Allocatable: mkResList("3.98", "15.9Gi", "99"), // ~0.5% reserved
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	}

	result := analyzeReservations(nodes)

	if result.Nodes[0].Status != "under-reserved" {
		t.Errorf("expected under-reserved, got %s", result.Nodes[0].Status)
	}
	found := false
	for _, iss := range result.Issues {
		if iss.Type == "UnderReserved" {
			found = true
		}
	}
	if !found {
		t.Error("expected UnderReserved issue")
	}
}

func TestAnalyzeReservations_ByType(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"}},
			Status: corev1.NodeStatus{
				Capacity: mkResList("2", "8Gi", "50"), Allocatable: mkResList("1.9", "7.5Gi", "48"),
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"}},
			Status: corev1.NodeStatus{
				Capacity: mkResList("2", "8Gi", "50"), Allocatable: mkResList("1.85", "7.3Gi", "48"),
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	}

	result := analyzeReservations(nodes)

	if len(result.ByNodeType) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result.ByNodeType))
	}
	if result.ByNodeType[0].NodeCount != 2 {
		t.Errorf("expected 2 nodes of type, got %d", result.ByNodeType[0].NodeCount)
	}
}
