package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// --- kindToResource tests ---

func TestCov_KindToResource_Simple(t *testing.T) {
	if r := kindToResource("Pod"); r != "pods" {
		t.Errorf("kindToResource(Pod) = %q, want 'pods'", r)
	}
}

func TestCov_KindToResource_EndsWithS(t *testing.T) {
	if r := kindToResource("Class"); r != "classes" {
		t.Errorf("kindToResource(Class) = %q, want 'classes'", r)
	}
}

func TestCov_KindToResource_EndsWithY(t *testing.T) {
	if r := kindToResource("Policy"); r != "policies" {
		t.Errorf("kindToResource(Policy) = %q, want 'policies'", r)
	}
}

func TestCov_KindToResource_Deployment(t *testing.T) {
	if r := kindToResource("Deployment"); r != "deployments" {
		t.Errorf("kindToResource(Deployment) = %q, want 'deployments'", r)
	}
}

func TestCov_KindToResource_EmptyString(t *testing.T) {
	if r := kindToResource(""); r != "s" {
		t.Errorf("kindToResource('') = %q, want 's'", r)
	}
}

// --- parseGroupVersionResource tests ---

func TestCov_ParseGVR_CoreAPI(t *testing.T) {
	kc := &KubeClient{}
	gvr, err := kc.parseGroupVersionResource("v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gvr.Group != "" || gvr.Version != "v1" {
		t.Errorf("GVR = %+v, want {Group:'', Version:'v1'}", gvr)
	}
}

func TestCov_ParseGVR_GroupVersion(t *testing.T) {
	kc := &KubeClient{}
	gvr, err := kc.parseGroupVersionResource("apps/v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gvr.Group != "apps" || gvr.Version != "v1" {
		t.Errorf("GVR = %+v, want {Group:'apps', Version:'v1'}", gvr)
	}
}

func TestCov_ParseGVR_TooManyParts(t *testing.T) {
	kc := &KubeClient{}
	_, err := kc.parseGroupVersionResource("a/b/c")
	if err == nil {
		t.Error("expected error for 3-part apiVersion")
	}
}

// --- GetResourceTool / ListResourcesTool / DescribeResourceTool metadata ---

func TestCov_GetResourceTool_Metadata(t *testing.T) {
	tool := &GetResourceTool{}
	if tool.Name() != "k8s_get_resource" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Error("type should be object")
	}
}

func TestCov_ListResourcesTool_Metadata(t *testing.T) {
	tool := &ListResourcesTool{}
	if tool.Name() != "k8s_list_resources" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters()["type"] != "object" {
		t.Error("type should be object")
	}
}

func TestCov_DescribeResourceTool_Metadata(t *testing.T) {
	tool := &DescribeResourceTool{}
	if tool.Name() != "k8s_describe_resource" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters()["type"] != "object" {
		t.Error("type should be object")
	}
}

func TestCov_GetPodLogsTool_Metadata(t *testing.T) {
	tool := &GetPodLogsTool{}
	if tool.Name() != "k8s_get_logs" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters()["type"] != "object" {
		t.Error("type should be object")
	}
}

func TestCov_ListAPIResourcesTool_Metadata(t *testing.T) {
	tool := &ListAPIResourcesTool{}
	if tool.Name() != "k8s_list_api_resources" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters()["type"] != "object" {
		t.Error("type should be object")
	}
}

func TestCov_GetNodesTool_Metadata(t *testing.T) {
	tool := &GetNodesTool{}
	if tool.Name() != "k8s_get_nodes" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters()["type"] != "object" {
		t.Error("type should be object")
	}
}

// --- Network tool metadata ---

func TestCov_GetServicesTool_Metadata(t *testing.T) {
	tool := &GetServicesTool{}
	if tool.Name() == "" {
		t.Error("Name empty")
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetIngressTool_Metadata(t *testing.T) {
	tool := &GetIngressTool{}
	if tool.Name() == "" {
		t.Error("Name empty")
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetConfigMapTool_Metadata(t *testing.T) {
	tool := &GetConfigMapTool{}
	if tool.Name() == "" {
		t.Error("Name empty")
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetNetworkPolicyTool_Metadata(t *testing.T) {
	tool := &GetNetworkPolicyTool{}
	if tool.Name() == "" {
		t.Error("Name empty")
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCov_GetPodStatusTool_Metadata(t *testing.T) {
	tool := &GetPodStatusTool{}
	if tool.Name() == "" {
		t.Error("Name empty")
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

// --- DynamicClient returns nil for empty KubeClient ---

func TestCov_DynamicClient_Nil(t *testing.T) {
	kc := &KubeClient{}
	if dc := kc.DynamicClient(); dc != nil {
		t.Error("expected nil DynamicClient")
	}
}

// --- Verify schema.GroupVersionResource unused warning doesn't happen
func TestCov_SchemaCompileCheck(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "pods"}
	if gvr.Group != "test" {
		t.Error("GVR Group mismatch")
	}
}
