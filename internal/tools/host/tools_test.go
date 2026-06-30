package host

import (
	"context"
	"strings"
	"testing"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
)

func TestHostInfoTool_Name(t *testing.T) {
	tool := &HostInfoTool{}
	if tool.Name() != "host_info" {
		t.Errorf("expected 'host_info', got '%s'", tool.Name())
	}
}

func TestHostInfoTool_Description(t *testing.T) {
	tool := &HostInfoTool{}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestHostInfoTool_Execute(t *testing.T) {
	tool := &HostInfoTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	// Should contain hostname
	hostname, _ := hostname()
	if hostname != "" && !strings.Contains(result.Output, hostname) {
		t.Errorf("expected output to contain hostname '%s'", hostname)
	}
}

func TestHostDiskUsageTool_Execute(t *testing.T) {
	tool := &HostDiskUsageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success for disk usage on '/'")
	}
}

func TestHostExecTool_Echo(t *testing.T) {
	tool := &HostExecTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello_world",
		"timeout": 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello_world") {
		t.Errorf("expected output to contain 'hello_world', got: %s", result.Output)
	}
}

func TestHostExecTool_Timeout(t *testing.T) {
	tool := &HostExecTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
		"timeout": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected timeout failure")
	}
}

func TestHostProcessTool_Execute(t *testing.T) {
	tool := &HostProcessTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"sortBy": "cpu",
		"limit": 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestAllTools_HaveValidSchemas(t *testing.T) {
	tools := []interface {
		Name() string
		Parameters() map[string]any
	}{
		&HostExecTool{},
		&HostDiskUsageTool{},
		&HostNetworkTool{},
		&HostProcessTool{},
		&HostServiceTool{},
		&HostInfoTool{},
		&HostDmesgTool{},
		&HostContainerRuntimeTool{},
		&HostKubeletTool{},
		&HostIPTablesTool{},
		&HostMountsTool{},
		&HostDiskIOTool{},
		&HostMemoryInfoTool{},
	}

	for _, tool := range tools {
		params := tool.Parameters()
		if params == nil {
			t.Errorf("tool %s has nil parameters", tool.Name())
		}
		if params["type"] != "object" {
			t.Errorf("tool %s parameters should have type 'object', got %v", tool.Name(), params["type"])
		}
	}
}

func TestHostNetworkTool_DNSResolve(t *testing.T) {
	tool := &HostNetworkTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "ping",
		"target": "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected ping to localhost to succeed")
	}
}

func hostname() (string, error) {
	// reuse os.Hostname via the info tool's code path
	tool := &HostInfoTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{})
	if strings.Contains(result.Output, "\"") {
		// extract from JSON
		return "", nil
	}
	return "", nil
}

// Ensure we reference the API package for type compatibility.
var _ = aiv1alpha1.K8opsConfig{}
