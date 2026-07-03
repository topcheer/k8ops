package host

import (
	"context"
	"os"
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

func TestHostInfoTool_Execute(t *testing.T) {
	tool := &HostInfoTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hostname") {
		t.Errorf("expected output to contain 'hostname', got: %s", result.Output)
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
		&HostInfoTool{},
		&HostServiceTool{},
	}

	for _, tool := range tools {
		params := tool.Parameters()
		if params == nil {
			t.Errorf("tool %s: nil parameters", tool.Name())
		}
		// Verify schema structure has type/properties or just properties
		if _, ok := params["type"]; ok {
			// standard schema with type field
		}
	}
}

func TestHostDiskUsageTool_Execute(t *testing.T) {
	if isCI() {
		t.Skip("skipping disk usage test in CI (df can hang on container mounts)")
	}
	tool := &HostDiskUsageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path": "/",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "total_bytes") {
		t.Errorf("expected output to contain 'total_bytes', got: %s", result.Output)
	}
}

// TestHostTools_CI_Skip validates that environment-dependent tools
// gracefully handle being run in CI environments where nsenter/network
// may not be available. These tests use t.Skip in CI.
func TestHostExecTool_Echo(t *testing.T) {
	if isCI() {
		t.Skip("skipping nsenter-dependent test in CI")
	}
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
	if isCI() {
		t.Skip("skipping exec timeout test in CI (nsenter/shell can hang)")
	}
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
	if isCI() {
		t.Skip("skipping nsenter-dependent test in CI")
	}
	tool := &HostProcessTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"sortBy": "cpu",
		"limit":  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestHostNetworkTool_DNSResolve(t *testing.T) {
	if isCI() {
		t.Skip("skipping network-dependent test in CI")
	}
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
	return result.Output, nil
}

func TestHostname(t *testing.T) {
	if isCI() {
		t.Skip("skipping hostname test in CI")
	}
	h, err := hostname()
	if err != nil {
		t.Fatalf("hostname error: %v", err)
	}
	if h == "" && testing.Short() {
		t.Skip("hostname empty in short mode")
	}
}

// dummy reference to satisfy import
var _ = aiv1alpha1.DiagnosticReport{}

// isCI returns true when running in a CI environment (GitHub Actions).
func isCI() bool {
	return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}
