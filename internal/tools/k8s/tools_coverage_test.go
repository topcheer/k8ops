package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Metadata tests for all tool types ---
// These tests verify Name/Description/Parameters without requiring a k8s cluster.

func TestCov_GetEventsTool_Metadata(t *testing.T) {
	tool := &GetEventsTool{}
	if tool.Name() != "k8s_get_events" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestCov_GetTopTool_Metadata(t *testing.T) {
	tool := &GetTopTool{}
	if tool.Name() != "k8s_top" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestCov_GetHPATool_Metadata(t *testing.T) {
	tool := &GetHPATool{}
	if tool.Name() != "k8s_get_hpa" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetPDBTool_Metadata(t *testing.T) {
	tool := &GetPDBTool{}
	if tool.Name() != "k8s_get_pdb" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetStorageTool_Metadata(t *testing.T) {
	tool := &GetStorageTool{}
	if tool.Name() != "k8s_get_storage" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_ExecInPodTool_Metadata(t *testing.T) {
	tool := &ExecInPodTool{}
	if tool.Name() != "k8s_exec" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetClusterVersionTool_Metadata(t *testing.T) {
	tool := &GetClusterVersionTool{}
	if tool.Name() != "k8s_cluster_info" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetNamespacesTool_Metadata(t *testing.T) {
	tool := &GetNamespacesTool{}
	if tool.Name() != "k8s_get_namespaces" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_DrainNodeTool_Metadata(t *testing.T) {
	tool := &DrainNodeTool{}
	if tool.Name() != "k8s_drain_node" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

// --- Helper function tests ---

func TestCov_DerefInt32_Nil(t *testing.T) {
	if derefInt32(nil, 42) != 42 {
		t.Error("derefInt32(nil, 42) should return 42")
	}
}

func TestCov_DerefInt32_Value(t *testing.T) {
	v := int32(7)
	if derefInt32(&v, 42) != 7 {
		t.Error("derefInt32(&7, 42) should return 7")
	}
}

func TestCov_DerefStr_Nil(t *testing.T) {
	if derefStr(nil) != "" {
		t.Error("derefStr(nil) should return empty string")
	}
}

func TestCov_DerefStr_Value(t *testing.T) {
	s := "hello"
	if derefStr(&s) != "hello" {
		t.Error("derefStr should return value")
	}
}

func TestCov_FormatAge_Minutes(t *testing.T) {
	result := formatAge(time.Now().Add(-5 * time.Minute))
	if !strings.HasSuffix(result, "m") {
		t.Errorf("formatAge(5m) = %q, should end with 'm'", result)
	}
}

func TestCov_FormatAge_Hours(t *testing.T) {
	result := formatAge(time.Now().Add(-3 * time.Hour))
	if !strings.HasSuffix(result, "h") {
		t.Errorf("formatAge(3h) = %q, should end with 'h'", result)
	}
}

func TestCov_FormatAge_Days(t *testing.T) {
	result := formatAge(time.Now().Add(-5 * 24 * time.Hour))
	if !strings.HasSuffix(result, "d") {
		t.Errorf("formatAge(5d) = %q, should end with 'd'", result)
	}
}

// --- isDaemonSetPod tests ---

func TestCov_IsDaemonSetPod_DaemonSetOwner(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "fluentd"},
			},
		},
	}
	if !isDaemonSetPod(pod) {
		t.Error("expected true for DaemonSet-owned pod")
	}
}

func TestCov_IsDaemonSetPod_ReplicaSetOwner(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "nginx-rs"},
			},
		},
	}
	if isDaemonSetPod(pod) {
		t.Error("expected false for ReplicaSet-owned pod")
	}
}

func TestCov_IsDaemonSetPod_NoOwner(t *testing.T) {
	pod := corev1.Pod{}
	if isDaemonSetPod(pod) {
		t.Error("expected false for pod with no owners")
	}
}

// --- Execute with nil client (error paths) ---
// kubernetes.NewForConfig(nil) panics, so use safeExecute wrapper.

func safeExecute(t *testing.T, exec func() (*tools.ToolResult, error)) (success bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			success = false
		}
	}()
	result, err := exec()
	if err != nil {
		success = false
		return
	}
	success = result.Success
	return
}

func TestCov_GetEventsTool_NilClient(t *testing.T) {
	tool := &GetEventsTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetTopTool_NilClient(t *testing.T) {
	tool := &GetTopTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetHPATool_NilClient(t *testing.T) {
	tool := &GetHPATool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetPDBTool_NilClient(t *testing.T) {
	tool := &GetPDBTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetStorageTool_NilClient(t *testing.T) {
	tool := &GetStorageTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_ExecInPodTool_NilClient(t *testing.T) {
	tool := &ExecInPodTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{"name": "p", "command": "ls"})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetClusterVersionTool_NilClient(t *testing.T) {
	tool := &GetClusterVersionTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_GetNamespacesTool_NilClient(t *testing.T) {
	tool := &GetNamespacesTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{})
	}) {
		t.Error("expected failure")
	}
}

func TestCov_DrainNodeTool_NilClient(t *testing.T) {
	tool := &DrainNodeTool{Client: &KubeClient{}}
	if safeExecute(t, func() (*tools.ToolResult, error) {
		return tool.Execute(context.Background(), map[string]any{"node": "w1"})
	}) {
		t.Error("expected failure")
	}
}

// --- Parameters field validation ---

func TestCov_GetEventsTool_ParametersHasFields(t *testing.T) {
	tool := &GetEventsTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	for _, field := range []string{"namespace", "kind", "name", "reason", "warning", "limit"} {
		if _, ok := props[field]; !ok {
			t.Errorf("Parameters missing field %q", field)
		}
	}
}

func TestCov_GetTopTool_ParametersHasFields(t *testing.T) {
	tool := &GetTopTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	for _, field := range []string{"resource", "sortBy", "limit"} {
		if _, ok := props[field]; !ok {
			t.Errorf("Parameters missing %q", field)
		}
	}
}

func TestCov_DrainNodeTool_ParametersHasFields(t *testing.T) {
	tool := &DrainNodeTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	for _, field := range []string{"node", "ignoreDaemonSets", "timeout"} {
		if _, ok := props[field]; !ok {
			t.Errorf("Parameters missing %q", field)
		}
	}
}

func TestCov_ExecInPodTool_ParametersHasFields(t *testing.T) {
	tool := &ExecInPodTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	for _, field := range []string{"name", "command"} {
		if _, ok := props[field]; !ok {
			t.Errorf("Parameters missing %q", field)
		}
	}
}

// --- All tools unique names ---

func TestCov_AllTools_UniqueNames(t *testing.T) {
	toolNames := []string{
		(&GetEventsTool{}).Name(),
		(&GetTopTool{}).Name(),
		(&GetHPATool{}).Name(),
		(&GetPDBTool{}).Name(),
		(&GetStorageTool{}).Name(),
		(&ExecInPodTool{}).Name(),
		(&GetClusterVersionTool{}).Name(),
		(&GetNamespacesTool{}).Name(),
		(&DrainNodeTool{}).Name(),
	}
	seen := make(map[string]bool)
	for _, name := range toolNames {
		if name == "" {
			t.Error("tool name should not be empty")
		}
		if seen[name] {
			t.Errorf("duplicate tool name: %s", name)
		}
		seen[name] = true
	}
	if len(seen) != 9 {
		t.Errorf("expected 9 unique tools, got %d", len(seen))
	}
}
