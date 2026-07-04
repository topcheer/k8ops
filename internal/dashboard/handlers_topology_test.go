package dashboard

import (
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeTopologyNode(name, zoneLabel string) *corev1.Node {
	labels := map[string]string{
		"kubernetes.io/hostname": name,
	}
	if zoneLabel != "" {
		labels["topology.kubernetes.io/zone"] = zoneLabel
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func makeTopologyPod(name, ns, nodeName, ownerKind, ownerName string) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if ownerKind != "" {
		pod.OwnerReferences = []metav1.OwnerReference{
			{Kind: ownerKind, Name: ownerName},
		}
	}
	return pod
}

func TestGetNodeDomain(t *testing.T) {
	tests := []struct {
		name   string
		node   *corev1.Node
		key    string
		expect string
	}{
		{
			"zone-label",
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"topology.kubernetes.io/zone": "us-east-1a"}}},
			"topology.kubernetes.io/zone",
			"us-east-1a",
		},
		{
			"hostname-key",
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{}}},
			"kubernetes.io/hostname",
			"node-1",
		},
		{
			"missing-label",
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{}}},
			"topology.kubernetes.io/zone",
			"<unlabeled>",
		},
	}

	for _, tt := range tests {
		got := getNodeDomain(tt.node, tt.key)
		if got != tt.expect {
			t.Errorf("getNodeDomain(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestGroupPodsByWorkload(t *testing.T) {
	pods := []corev1.Pod{
		makeTopologyPod("p1", "default", "n1", "ReplicaSet", "app-rs"),
		makeTopologyPod("p2", "default", "n2", "ReplicaSet", "app-rs"),
		makeTopologyPod("p3", "default", "n1", "ReplicaSet", "other-rs"),
		makeTopologyPod("p4", "monitoring", "n1", "", ""), // standalone pod
	}

	groups := groupPodsByWorkload(pods)

	if len(groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}

	rsKey := "default/ReplicaSet/app-rs"
	if len(groups[rsKey]) != 2 {
		t.Errorf("Expected 2 pods in app-rs group, got %d", len(groups[rsKey]))
	}
}

func TestAnalyzeWorkloadTopologyBalanced(t *testing.T) {
	pods := []corev1.Pod{
		makeTopologyPod("p1", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p2", "default", "n2", "Deployment", "app"),
		makeTopologyPod("p3", "default", "n3", "Deployment", "app"),
		makeTopologyPod("p4", "default", "n4", "Deployment", "app"),
	}

	nodeDomain := map[string]string{
		"n1": "zone-a", "n2": "zone-b", "n3": "zone-c", "n4": "zone-d",
	}

	tw := analyzeWorkloadTopology("default/Deployment/app", pods, nodeDomain, "topology.kubernetes.io/zone", 4)

	if tw.Status != "balanced" {
		t.Errorf("Expected balanced, got %s (skew=%d)", tw.Status, tw.ActualSkew)
	}
	if tw.ActualSkew != 0 {
		t.Errorf("Expected skew 0, got %d", tw.ActualSkew)
	}
}

func TestAnalyzeWorkloadTopologySkewed(t *testing.T) {
	pods := []corev1.Pod{
		makeTopologyPod("p1", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p2", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p3", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p4", "default", "n2", "Deployment", "app"),
	}

	nodeDomain := map[string]string{
		"n1": "zone-a", "n2": "zone-b",
	}

	tw := analyzeWorkloadTopology("default/Deployment/app", pods, nodeDomain, "topology.kubernetes.io/zone", 2)

	if tw.ActualSkew != 2 {
		t.Errorf("Expected skew 2, got %d", tw.ActualSkew)
	}
}

func TestAnalyzeWorkloadTopologyWithConstraint(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "n1",
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "topology.kubernetes.io/zone",
						WhenUnsatisfiable: corev1.DoNotSchedule,
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "n1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "n1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p4", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "n2"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	nodeDomain := map[string]string{"n1": "zone-a", "n2": "zone-b"}

	tw := analyzeWorkloadTopology("default/Deployment/app", pods, nodeDomain, "topology.kubernetes.io/zone", 2)

	if !tw.HasConstraint {
		t.Error("Expected HasConstraint = true")
	}
	if tw.MaxSkew != 1 {
		t.Errorf("Expected MaxSkew 1, got %d", tw.MaxSkew)
	}
	if tw.WhenUnsatisfiable != "DoNotSchedule" {
		t.Errorf("Expected DoNotSchedule, got %s", tw.WhenUnsatisfiable)
	}
	// 3 pods on zone-a, 1 on zone-b -> skew = 2, exceeds maxSkew=1
	if tw.Status != "skewed" {
		t.Errorf("Expected skewed, got %s (actualSkew=%d, maxSkew=%d)", tw.Status, tw.ActualSkew, tw.MaxSkew)
	}
}

func TestAnalyzeWorkloadTopologySingleReplica(t *testing.T) {
	pods := []corev1.Pod{
		makeTopologyPod("p1", "default", "n1", "Deployment", "app"),
	}

	tw := analyzeWorkloadTopology("default/Deployment/app", pods, map[string]string{"n1": "zone-a"}, "topology.kubernetes.io/zone", 1)

	if tw.Status != "single-replica" {
		t.Errorf("Expected single-replica, got %s", tw.Status)
	}
}

func TestAnalyzeWorkloadTopologyNoConstraint(t *testing.T) {
	pods := []corev1.Pod{
		makeTopologyPod("p1", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p2", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p3", "default", "n1", "Deployment", "app"),
		makeTopologyPod("p4", "default", "n2", "Deployment", "app"),
	}

	nodeDomain := map[string]string{"n1": "zone-a", "n2": "zone-b"}

	tw := analyzeWorkloadTopology("default/Deployment/app", pods, nodeDomain, "topology.kubernetes.io/zone", 2)

	if tw.HasConstraint {
		t.Error("Expected HasConstraint = false")
	}
	if tw.Status != "no-constraint" {
		t.Errorf("Expected no-constraint, got %s", tw.Status)
	}
}

func TestGenerateTopologyRecommendations(t *testing.T) {
	result := TopologyResult{
		Summary: TopologySummary{
			SkewedWorkloads: 2,
			NoConstraintWL:  3,
			MaxSkew:         4,
			TotalDomains:    3,
			DomainKey:       "topology.kubernetes.io/zone",
		},
	}

	recs := generateTopologyRecommendations(result)

	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundSkew := false
	foundNoConstraint := false
	for _, r := range recs {
		if containsSubstr(r, "topology skew") {
			foundSkew = true
		}
		if containsSubstr(r, "topology spread constraints") {
			foundNoConstraint = true
		}
	}
	if !foundSkew {
		t.Error("Expected recommendation about skew")
	}
	if !foundNoConstraint {
		t.Error("Expected recommendation about missing constraints")
	}
}

func TestGenerateTopologyRecommendationsSingleDomain(t *testing.T) {
	result := TopologyResult{
		Summary: TopologySummary{
			TotalDomains: 1,
			DomainKey:    "topology.kubernetes.io/zone",
		},
	}

	recs := generateTopologyRecommendations(result)
	if len(recs) == 0 {
		t.Error("Expected recommendation about single domain")
	}
}

func TestGenerateTopologyRecommendationsCleanCluster(t *testing.T) {
	result := TopologyResult{
		Summary: TopologySummary{
			TotalDomains:    3,
			SkewedWorkloads: 0,
			NoConstraintWL:  0,
			MaxSkew:         0,
		},
	}

	recs := generateTopologyRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for balanced cluster, got %d", len(recs))
	}
}

func TestTopologyDistributionSorting(t *testing.T) {
	dists := []DomainDistribution{
		{Domain: "zone-b", PodCount: 1},
		{Domain: "zone-a", PodCount: 5},
		{Domain: "zone-c", PodCount: 3},
	}

	sort.Slice(dists, func(i, j int) bool {
		return dists[i].PodCount > dists[j].PodCount
	})

	if dists[0].Domain != "zone-a" || dists[0].PodCount != 5 {
		t.Errorf("Expected zone-a (5) first, got %s (%d)", dists[0].Domain, dists[0].PodCount)
	}
}
