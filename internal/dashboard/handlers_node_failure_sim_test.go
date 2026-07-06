package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNFSAssessRisk(t *testing.T) {
	// Single replica on node = critical
	entry := NFSEntry{SingleReplicaWL: 1}
	if level := nfsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for single replica, got %s", level)
	}

	// Many unschedulable = critical
	entry = NFSEntry{Unschedulable: 6}
	if level := nfsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for >5 unschedulable, got %s", level)
	}

	// Many affected = high
	entry = NFSEntry{AffectedPods: 12, Unschedulable: 0}
	if level := nfsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for >10 affected, got %s", level)
	}

	// Some unschedulable = medium
	entry = NFSEntry{AffectedPods: 5, Unschedulable: 2}
	if level := nfsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for some unschedulable, got %s", level)
	}

	// Clean = low
	entry = NFSEntry{AffectedPods: 3, Unschedulable: 0}
	if level := nfsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestNFSScore(t *testing.T) {
	// Empty
	if score := nfsScore(NFSSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := NFSSummary{TotalNodes: 5}
	if score := nfsScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = NFSSummary{
		TotalNodes:         5,
		SingleReplicaNodes: 2, // -24
		CriticalNodes:      2, // -12
		UnschedulableAvg:   3, // -12
	}
	// 100 - 24 - 12 - 12 = 52
	if score := nfsScore(s); score != 52 {
		t.Errorf("Expected 52, got %d", score)
	}

	// Heavily broken
	s = NFSSummary{
		TotalNodes:         3,
		SingleReplicaNodes: 3, // -36
		CriticalNodes:      3, // -18
		UnschedulableAvg:   5, // -20
	}
	// 100 - 36 - 18 - 20 = 26
	if score := nfsScore(s); score != 26 {
		t.Errorf("Expected 26, got %d", score)
	}
}

func TestNFSGenRecs(t *testing.T) {
	s := NFSSummary{
		TotalNodes:         5,
		SingleReplicaNodes: 2,
		CriticalNodes:      1,
		MaxAffected:        18,
		UnschedulableAvg:   4,
		ResilienceScore:    40,
	}

	recs := nfsGenRecs(s, nil, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundSingleReplica := false
	foundCritical := false
	foundMaxAffected := false
	for _, r := range recs {
		if strContains(r, "single-replica") {
			foundSingleReplica = true
		}
		if strContains(r, ">10 pods") {
			foundCritical = true
		}
		if strContains(r, "Worst-case") {
			foundMaxAffected = true
		}
	}
	if !foundSingleReplica {
		t.Error("Expected recommendation about single-replica nodes")
	}
	if !foundCritical {
		t.Error("Expected recommendation about critical nodes")
	}
	if !foundMaxAffected {
		t.Error("Expected recommendation about worst-case")
	}
}

func TestNFSGenRecsClean(t *testing.T) {
	s := NFSSummary{TotalNodes: 5}
	recs := nfsGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestNFSIssueRank(t *testing.T) {
	if nfsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if nfsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if nfsIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestIsDaemonSetPod(t *testing.T) {
	// Regular pod (ReplicaSet owner)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "rs-123"},
			},
		},
	}
	if isDaemonSetPod(pod) {
		t.Error("Expected false for ReplicaSet pod")
	}

	// DaemonSet pod
	pod.OwnerReferences = []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "ds-123"},
	}
	if !isDaemonSetPod(pod) {
		t.Error("Expected true for DaemonSet pod")
	}

	// No owner references
	pod.OwnerReferences = nil
	if isDaemonSetPod(pod) {
		t.Error("Expected false for pod with no owner")
	}
}
