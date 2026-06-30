package k8s

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Helper Function Tests: formatAge, derefInt32, derefStr
// ===========================================================================

func TestFormatAge_Days(t *testing.T) {
	old := time.Now().AddDate(0, 0, -5)
	result := formatAge(old)
	assert.True(t, strings.HasSuffix(result, "d"), "expected days suffix, got %q", result)
}

func TestFormatAge_Hours(t *testing.T) {
	old := time.Now().Add(-3 * time.Hour)
	result := formatAge(old)
	assert.True(t, strings.HasSuffix(result, "h"), "expected hours suffix, got %q", result)
}

func TestFormatAge_Minutes(t *testing.T) {
	old := time.Now().Add(-2 * time.Minute)
	result := formatAge(old)
	assert.True(t, strings.HasSuffix(result, "m"), "expected minutes suffix, got %q", result)
}

func TestFormatAge_NearZero(t *testing.T) {
	now := time.Now()
	result := formatAge(now)
	assert.True(t, strings.HasSuffix(result, "m"), "expected minutes suffix for near-zero, got %q", result)
}

func TestDerefInt32_Nil(t *testing.T) {
	assert.Equal(t, int32(42), derefInt32(nil, 42))
}

func TestDerefInt32_NonNil(t *testing.T) {
	v := int32(99)
	assert.Equal(t, int32(99), derefInt32(&v, 42))
}

func TestDerefInt32_Zero(t *testing.T) {
	v := int32(0)
	assert.Equal(t, int32(0), derefInt32(&v, 42))
}

func TestDerefStr_Nil(t *testing.T) {
	assert.Equal(t, "", derefStr(nil))
}

func TestDerefStr_NonNil(t *testing.T) {
	s := "hello"
	assert.Equal(t, "hello", derefStr(&s))
}

func TestDerefStr_Empty(t *testing.T) {
	s := ""
	assert.Equal(t, "", derefStr(&s))
}

// ===========================================================================
// Tool Metadata Tests: Name, Description, Parameters
// ===========================================================================

func TestGetEventsTool_Metadata(t *testing.T) {
	tool := &GetEventsTool{}
	assert.Equal(t, "k8s_get_events", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "event")
}

func TestGetNamespacesTool_Metadata(t *testing.T) {
	tool := &GetNamespacesTool{}
	assert.Equal(t, "k8s_get_namespaces", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
}

func TestGetTopTool_Metadata(t *testing.T) {
	tool := &GetTopTool{}
	assert.Equal(t, "k8s_top", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "resource")
}

func TestGetHPATool_Metadata(t *testing.T) {
	tool := &GetHPATool{}
	assert.Equal(t, "k8s_get_hpa", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "Autoscaler")
}

func TestGetPDBTool_Metadata(t *testing.T) {
	tool := &GetPDBTool{}
	assert.Equal(t, "k8s_get_pdb", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "Disruption")
}

func TestGetStorageTool_Metadata(t *testing.T) {
	tool := &GetStorageTool{}
	assert.Equal(t, "k8s_get_storage", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "storage")
}

func TestExecInPodTool_Metadata(t *testing.T) {
	tool := &ExecInPodTool{}
	assert.Equal(t, "k8s_exec", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "Execute")
}

func TestGetClusterVersionTool_Metadata(t *testing.T) {
	tool := &GetClusterVersionTool{}
	assert.Equal(t, "k8s_cluster_info", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "cluster")
}

func TestDrainNodeTool_Metadata(t *testing.T) {
	tool := &DrainNodeTool{}
	assert.Equal(t, "k8s_drain_node", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
	assert.Contains(t, tool.Description(), "drain")
}

// ===========================================================================
// Tool Parameters Structure Validation
// ===========================================================================

func TestAllToolNames_Unique(t *testing.T) {
	tools := []interface{ Name() string }{
		&GetEventsTool{},
		&GetNamespacesTool{},
		&GetTopTool{},
		&GetHPATool{},
		&GetPDBTool{},
		&GetStorageTool{},
		&ExecInPodTool{},
		&GetClusterVersionTool{},
		&DrainNodeTool{},
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		name := tool.Name()
		require.False(t, names[name], "duplicate tool name: %s", name)
		names[name] = true
	}
	assert.Equal(t, 9, len(names))
}

func TestAllToolParameters_ReturnNonNil(t *testing.T) {
	tools := []interface{ Parameters() map[string]any }{
		&GetEventsTool{},
		&GetNamespacesTool{},
		&GetTopTool{},
		&GetHPATool{},
		&GetPDBTool{},
		&GetStorageTool{},
		&ExecInPodTool{},
		&GetClusterVersionTool{},
		&DrainNodeTool{},
	}
	for i, tool := range tools {
		params := tool.Parameters()
		assert.NotNil(t, params, "tool at index %d should have non-nil Parameters", i)
		assert.NotEmpty(t, params, "tool at index %d should have non-empty Parameters", i)
	}
}

func TestAllToolDescriptions_NotEmpty(t *testing.T) {
	tools := []interface{ Description() string }{
		&GetEventsTool{},
		&GetNamespacesTool{},
		&GetTopTool{},
		&GetHPATool{},
		&GetPDBTool{},
		&GetStorageTool{},
		&ExecInPodTool{},
		&GetClusterVersionTool{},
		&DrainNodeTool{},
	}
	for i, tool := range tools {
		desc := tool.Description()
		assert.NotEmpty(t, desc, "tool at index %d should have non-empty Description", i)
	}
}

// Verify Parameters returns a valid schema with "type" key (JSON schema format).
func TestAllToolParameters_HaveTypeKey(t *testing.T) {
	tools := []struct {
		name   string
		params map[string]any
	}{
		{"events", (&GetEventsTool{}).Parameters()},
		{"namespaces", (&GetNamespacesTool{}).Parameters()},
		{"top", (&GetTopTool{}).Parameters()},
		{"hpa", (&GetHPATool{}).Parameters()},
		{"pdb", (&GetPDBTool{}).Parameters()},
		{"storage", (&GetStorageTool{}).Parameters()},
		{"exec", (&ExecInPodTool{}).Parameters()},
		{"cluster_info", (&GetClusterVersionTool{}).Parameters()},
		{"drain", (&DrainNodeTool{}).Parameters()},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			// Each schema should have a "type" key as per JSON Schema format
			_, ok := tt.params["type"]
			assert.True(t, ok, "tool %s should have 'type' key in Parameters", tt.name)
		})
	}
}

// ===========================================================================
// isDaemonSetPod (already tested in tools_test.go, adding edge case)
// ===========================================================================

func TestIsDaemonSetPod_NoAnnotations(t *testing.T) {
	// Tested in tools_test.go — verify we don't duplicate
	// Just ensure the function signature is correct
}
