package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNodeUpgradeEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/node-upgrade-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleNodeUpgrade(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result NodeUpgradeResult
	json.Unmarshal(w.Body.Bytes(), &result)
}

func TestNodeUpgradeVersionSkew(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.28.0"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.29.0"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.27.0"}}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/node-upgrade-audit", clientset)
	w := httptest.NewRecorder()
	s.handleNodeUpgrade(w, req)
	var result NodeUpgradeResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Summary.VersionSkew {
		t.Error("expected version skew")
	}
	if result.Summary.MaxSkewVersions != 3 {
		t.Errorf("expected 3 versions, got %d", result.Summary.MaxSkewVersions)
	}
}

func TestNodeUpgradeAligned(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.29.4"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.29.4"}}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/node-upgrade-audit", clientset)
	w := httptest.NewRecorder()
	s.handleNodeUpgrade(w, req)
	var result NodeUpgradeResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.VersionSkew {
		t.Error("expected no version skew")
	}
	if result.ReadinessScore < 80 {
		t.Errorf("expected >=80 readiness, got %d", result.ReadinessScore)
	}
}

func TestNodeUpgradeNodePressure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "stressed"},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.29.0"},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/node-upgrade-audit", clientset)
	w := httptest.NewRecorder()
	s.handleNodeUpgrade(w, req)
	var result NodeUpgradeResult
	json.Unmarshal(w.Body.Bytes(), &result)
	found := false
	for _, b := range result.Blockers {
		if b.Type == "resource-pressure" {
			found = true
		}
	}
	if !found {
		t.Error("expected resource-pressure blocker")
	}
}

func TestNodeUpgradeRecs(t *testing.T) {
	result := NodeUpgradeResult{
		Summary:        NodeUpgradeSummary{VersionSkew: true, MaxSkewVersions: 3},
		Blockers:       []UpgradeBlocker{{Severity: "high"}},
		ReadinessScore: 35,
	}
	recs := generateNodeUpgradeRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundSkew := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "skew") {
			foundSkew = true
		}
	}
	if !foundSkew {
		t.Error("expected skew recommendation")
	}
}
