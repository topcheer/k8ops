package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNodeTrend_HealthyNodes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
				},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion:          "v1.28.0",
					ContainerRuntimeVersion: "containerd://1.7.0",
					KernelVersion:           "5.15.0-25-generic",
					OSImage:                 "Ubuntu 22.04",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
				},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion:          "v1.28.0",
					ContainerRuntimeVersion: "containerd://1.7.0",
					KernelVersion:           "5.15.0-25-generic",
					OSImage:                 "Ubuntu 22.04",
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/node-trend", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeTrend(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NodeTrendResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalNodes != 2 {
		t.Errorf("expected 2 nodes, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.NotReady != 0 {
		t.Errorf("expected 0 not ready, got %d", result.Summary.NotReady)
	}
	if result.HealthScore < 95 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestNodeTrend_NotReadyNode(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-down"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "KubeletNotReady"},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-up"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/node-trend", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeTrend(rec, req)

	var result NodeTrendResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NotReady != 1 {
		t.Errorf("expected 1 not ready, got %d", result.Summary.NotReady)
	}
	found := false
	for _, nc := range result.NodeConditions {
		if nc.NodeName == "node-down" && nc.RiskLevel == "critical" {
			found = true
		}
	}
	if !found {
		t.Error("expected node-down to have critical risk level")
	}
}

func TestNodeTrend_DiskPressure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-pressure"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
					{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Reason: "NoSpaceLeft"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/node-trend", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeTrend(rec, req)

	var result NodeTrendResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.DiskPressure != 1 {
		t.Errorf("expected 1 disk pressure, got %d", result.Summary.DiskPressure)
	}
	foundDisk := false
	for _, ar := range result.AtRiskNodes {
		for _, rf := range ar.RiskFactors {
			if rf == "DiskPressure" {
				foundDisk = true
			}
		}
	}
	if !foundDisk {
		t.Error("expected to find DiskPressure risk factor")
	}
}

func TestNodeTrend_MemoryPressure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-mem-pressure"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasInsufficientMemory"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/node-trend", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeTrend(rec, req)

	var result NodeTrendResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MemoryPressure != 1 {
		t.Errorf("expected 1 memory pressure, got %d", result.Summary.MemoryPressure)
	}
}

func TestNodeTrend_StaleHeartbeat(t *testing.T) {
	staleTime := metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-stale"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: staleTime},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/node-trend", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeTrend(rec, req)

	var result NodeTrendResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Stale heartbeat should trigger critical risk
	found := false
	for _, ar := range result.AtRiskNodes {
		for _, rf := range ar.RiskFactors {
			if len(rf) > 15 && rf[:5] == "Stale" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected to find stale heartbeat risk factor")
	}
}
