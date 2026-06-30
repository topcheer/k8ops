package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Verify that all extra tools implement the Tool interface and have valid schemas.

func TestExtraTools_Schemas(t *testing.T) {
	// We can't test actual execution without a kube client, but we can validate schemas.
	// Each tool needs a non-nil KubeClient to execute, but Parameters() should work standalone.
	type schemaProvider interface {
		Name() string
		Description() string
		Parameters() map[string]any
	}

	// Test GetEventsTool schema
	tools2 := []schemaProvider{
		&GetEventsTool{},
		&GetNamespacesTool{},
		&GetTopTool{},
		&GetHPATool{},
		&GetPDBTool{},
		&GetStorageTool{},
		&GetClusterVersionTool{},
		&DrainNodeTool{},
		&GetServicesTool{},
		&GetConfigMapTool{},
		&GetIngressTool{},
		&GetNetworkPolicyTool{},
		&GetPodStatusTool{},
	}

	for _, tool := range tools2 {
		t.Run(tool.Name(), func(t *testing.T) {
			if tool.Name() == "" {
				t.Error("tool should have non-empty name")
			}
			if tool.Description() == "" {
				t.Error("tool should have non-empty description")
			}
			params := tool.Parameters()
			if params == nil {
				t.Error("parameters should not be nil")
			}
			if params["type"] != "object" {
				t.Errorf("expected type 'object', got %v", params["type"])
			}
		})
	}
}

func TestExtraTools_Description_Quality(t *testing.T) {
	tools2 := []interface{ Description() string }{
		&GetEventsTool{},
		&GetNamespacesTool{},
		&GetTopTool{},
		&GetHPATool{},
		&GetPDBTool{},
		&GetStorageTool{},
		&GetClusterVersionTool{},
		&DrainNodeTool{},
		&GetServicesTool{},
		&GetConfigMapTool{},
		&GetIngressTool{},
		&GetNetworkPolicyTool{},
		&GetPodStatusTool{},
	}

	for _, tool := range tools2 {
		desc := tool.Description()
		if len(desc) < 20 {
			t.Errorf("description too short: %s", desc)
		}
	}
}

func TestExtraTools_ParameterValidation(t *testing.T) {
	// Verify GetEventsTool has proper required fields
	eventsTool := &GetEventsTool{}
	params := eventsTool.Parameters()
	if _, ok := params["properties"].(map[string]any); !ok {
		t.Error("expected properties map in schema")
	}

	// Verify GetTopTool has enum
	topTool := &GetTopTool{}
	topParams := topTool.Parameters()
	props := topParams["properties"].(map[string]any)
	if _, ok := props["resource"]; !ok {
		t.Error("expected 'resource' property")
	}

	// Verify GetConfigMapTool has required field
	cmTool := &GetConfigMapTool{}
	cmParams := cmTool.Parameters()
	cmRequired, ok := cmParams["required"].([]string)
	if !ok || len(cmRequired) == 0 {
		t.Error("expected GetConfigMapTool to have required parameters")
	}
}

func TestDerefHelpers(t *testing.T) {
	// Test derefStr
	s := "hello"
	if v := derefStr(&s); v != "hello" {
		t.Errorf("expected 'hello', got '%s'", v)
	}
	if v := derefStr(nil); v != "" {
		t.Errorf("expected '', got '%s'", v)
	}

	// Test derefInt32
	i := int32(42)
	if v := derefInt32(&i, 0); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if v := derefInt32(nil, 99); v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestFormatAge(t *testing.T) {
	// Test that formatAge produces a string
	result := formatAge(time.Now())
	if result == "" {
		t.Error("expected non-empty age string")
	}
}

func TestIsDaemonSetPod(t *testing.T) {
	// Test with DaemonSet owner reference
	pod := corev1.Pod{}
	pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "fluentd"},
	}
	if !isDaemonSetPod(pod) {
		t.Error("expected pod to be identified as DaemonSet pod")
	}

	// Test with non-DaemonSet owner reference
	pod2 := corev1.Pod{}
	pod2.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "app-rs"},
	}
	if isDaemonSetPod(pod2) {
		t.Error("expected pod to NOT be identified as DaemonSet pod")
	}
}
