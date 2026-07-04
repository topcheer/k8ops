package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseSchedulingFailure(t *testing.T) {
	tests := []struct {
		msg    string
		expect string
	}{
		{"0/3 nodes are available: 1 Insufficient cpu.", "insufficient-cpu"},
		{"0/3 nodes are available: 1 Insufficient memory.", "insufficient-memory"},
		{"0/3 nodes are available: 3 node(s) had untolerated taint.", "untolerated-taint"},
		{"0/3 nodes are available: 3 node(s) didn't match node selector.", "node-selector-mismatch"},
		{"0/3 nodes are available: 3 node(s) had volume node affinity conflict.", "volume-binding-failure"},
		{"0/3 nodes are available.", "no-nodes-available"},
		{"some random message", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		got := parseSchedulingFailure(tt.msg)
		if got != tt.expect {
			t.Errorf("parseSchedulingFailure(%q) = %q, want %q", tt.msg, got, tt.expect)
		}
	}
}

func TestParseEvictionReason(t *testing.T) {
	tests := []struct {
		msg    string
		expect string
	}{
		{"The node was low on resource: memory.", "low-memory"},
		{"The node was low on resource: ephemeral-storage.", "low-disk-space"},
		{"The node had PID pressure", "pid-pressure"},
	}

	for _, tt := range tests {
		got := parseEvictionReason(tt.msg)
		if got != tt.expect {
			t.Errorf("parseEvictionReason(%q) = %q, want %q", tt.msg, got, tt.expect)
		}
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name   string
		conds  []corev1.NodeCondition
		expect bool
	}{
		{"ready", []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, true},
		{"notready", []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}, false},
		{"noconds", nil, false},
	}

	for _, tt := range tests {
		node := &corev1.Node{Status: corev1.NodeStatus{Conditions: tt.conds}}
		got := isNodeReady(node)
		if got != tt.expect {
			t.Errorf("isNodeReady(%s) = %v, want %v", tt.name, got, tt.expect)
		}
	}
}

func TestAnalyzeSchedulingNode(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
			Taints: []corev1.Taint{
				{Key: "gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	alloc := &nodeAllocationData{
		cpuM:  2000,
		memGB: 8.0,
		pods:  40,
	}

	sn := analyzeSchedulingNode(node, alloc)

	if sn.Name != "worker-1" {
		t.Errorf("Expected name worker-1, got %s", sn.Name)
	}
	if !sn.Schedulable {
		t.Error("Expected schedulable = true")
	}
	if !sn.Ready {
		t.Error("Expected ready = true")
	}
	if sn.UnderPressure {
		t.Error("Expected underPressure = false")
	}
	if len(sn.Taints) != 1 {
		t.Errorf("Expected 1 taint, got %d", len(sn.Taints))
	}
	if sn.CPUAllocatable != 4000 {
		t.Errorf("Expected CPU 4000m, got %d", sn.CPUAllocatable)
	}
	if sn.CPUAvailable != 2000 {
		t.Errorf("Expected available CPU 2000m, got %d", sn.CPUAvailable)
	}
	if sn.PodAvailable != 70 {
		t.Errorf("Expected 70 pods available, got %d", sn.PodAvailable)
	}
}

func TestAnalyzeSchedulingNodeCordoned(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cordon-1"},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	sn := analyzeSchedulingNode(node, nil)

	if sn.Schedulable {
		t.Error("Expected schedulable = false for cordoned node")
	}
}

func TestAnalyzeSchedulingNodeUnderPressure(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "pressured-1"},
		Spec:       corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	sn := analyzeSchedulingNode(node, nil)

	if !sn.UnderPressure {
		t.Error("Expected underPressure = true")
	}
	if len(sn.PressureTypes) != 2 {
		t.Errorf("Expected 2 pressure types, got %d", len(sn.PressureTypes))
	}
}

func TestBuildNodeAllocation(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1000m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod3"},
			Spec:       corev1.PodSpec{NodeName: ""},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod4"},
			Spec:       corev1.PodSpec{NodeName: "node-2"},
			Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	}

	alloc := buildNodeAllocation(nil, pods)

	data, ok := alloc["node-1"]
	if !ok {
		t.Fatal("Expected allocation data for node-1")
	}
	if data.cpuM != 1500 {
		t.Errorf("Expected 1500m CPU allocated, got %d", data.cpuM)
	}
	if data.memGB < 2.9 || data.memGB > 3.1 {
		t.Errorf("Expected ~3GB memory allocated, got %.2f", data.memGB)
	}
	if data.pods != 2 {
		t.Errorf("Expected 2 pods, got %d", data.pods)
	}

	// node-2 should not have allocation (pod succeeded)
	if _, ok := alloc["node-2"]; ok {
		t.Error("Should not have allocation for node-2 with only succeeded pods")
	}
}

func TestBuildPendingPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pending-1",
			Namespace:         "default",
			CreationTimestamp: metav1.Time{},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"disktype": "ssd"},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}

	pp := buildPendingPod(pod)

	if pp.Name != "pending-1" {
		t.Errorf("Expected name pending-1, got %s", pp.Name)
	}
	if pp.CPURequest != 2000 {
		t.Errorf("Expected CPU 2000m, got %d", pp.CPURequest)
	}
	if pp.MemRequestGB < 3.9 || pp.MemRequestGB > 4.1 {
		t.Errorf("Expected ~4GB memory, got %.2f", pp.MemRequestGB)
	}
	if pp.NodeSelector["disktype"] != "ssd" {
		t.Errorf("Expected node selector disktype=ssd")
	}
}

func TestAnalyzeFragmentation(t *testing.T) {
	nodes := []SchedulingNode{
		{
			Name: "n1", Schedulable: true, Ready: true,
			CPUAllocatable: 4000, CPUAvailable: 3000, CPUAvailablePct: 75,
			MemAllocatableGB: 16, MemAvailableGB: 12, MemAvailablePct: 75,
		},
		{
			Name: "n2", Schedulable: true, Ready: true,
			CPUAllocatable: 4000, CPUAvailable: 1000, CPUAvailablePct: 25,
			MemAllocatableGB: 16, MemAvailableGB: 4, MemAvailablePct: 25,
		},
		{
			Name: "n3", Schedulable: false, Ready: false,
		},
	}

	frag := analyzeFragmentation(nodes)

	if frag.AvgCPUFragmentPct != 50 {
		t.Errorf("Expected avg CPU frag 50%%, got %.1f", frag.AvgCPUFragmentPct)
	}
	if frag.WorstFragmentNode != "n1" {
		t.Errorf("Expected worst fragment node n1, got %s", frag.WorstFragmentNode)
	}
}

func TestComputeSchedulingScore(t *testing.T) {
	// Perfect cluster
	score := computeSchedulingScore(
		SchedulingSummary{TotalNodes: 3, SchedulableNodes: 3, UnschedulableNodes: 0},
		EffectiveCapacity{},
		FragmentationInfo{},
	)
	if score != 100 {
		t.Errorf("Expected score 100 for perfect cluster, got %d", score)
	}

	// Degraded cluster
	score = computeSchedulingScore(
		SchedulingSummary{TotalNodes: 5, UnschedulableNodes: 2, PendingPods: 3, NodesUnderPressure: 1},
		EffectiveCapacity{CPULostPct: 40, MemLostPct: 40},
		FragmentationInfo{AvgCPUFragmentPct: 60, OversizedPodCount: 2},
	)
	if score >= 50 {
		t.Errorf("Expected low score for degraded cluster, got %d", score)
	}

	// Ensure score doesn't go negative
	score = computeSchedulingScore(
		SchedulingSummary{TotalNodes: 10, UnschedulableNodes: 10, PendingPods: 50, NodesUnderPressure: 10},
		EffectiveCapacity{CPULostPct: 100},
		FragmentationInfo{},
	)
	if score < 0 {
		t.Errorf("Score should not go negative, got %d", score)
	}
}

func TestGenerateSchedulingRecommendations(t *testing.T) {
	result := SchedulingResult{
		Summary: SchedulingSummary{
			PendingPods:        2,
			CordonedNodes:      1,
			NodesUnderPressure: 1,
			RecentEvictions:    3,
		},
		EffectiveCapacity: EffectiveCapacity{
			CPULostPct: 30,
			MemLostPct: 20,
		},
		Fragmentation: FragmentationInfo{
			OversizedPodCount: 1,
		},
		LargestFittablePod: FittablePod{MaxCPUm: 500},
	}

	recs := generateSchedulingRecommendations(result)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	// Check for specific recommendations
	foundPending := false
	foundCordon := false
	foundCPULost := false
	for _, r := range recs {
		if containsSubstr(r, "Pending") {
			foundPending = true
		}
		if containsSubstr(r, "cordoned") {
			foundCordon = true
		}
		if containsSubstr(r, "CPU capacity") {
			foundCPULost = true
		}
	}
	if !foundPending {
		t.Error("Expected recommendation about pending pods")
	}
	if !foundCordon {
		t.Error("Expected recommendation about cordoned nodes")
	}
	if !foundCPULost {
		t.Error("Expected recommendation about CPU capacity lost")
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b"}
	slice = appendUnique(slice, "a") // should not add
	if len(slice) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(slice))
	}
	slice = appendUnique(slice, "c") // should add
	if len(slice) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(slice))
	}
}

func TestPct(t *testing.T) {
	if pct(50, 100) != 50 {
		t.Errorf("Expected 50, got %f", pct(50, 100))
	}
	if pct(0, 100) != 0 {
		t.Errorf("Expected 0, got %f", pct(0, 100))
	}
	if pct(10, 0) != 0 {
		t.Errorf("Expected 0 for total=0, got %f", pct(10, 0))
	}
}

// helper
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
