// Package host provides tools for managing the host node directly.
// These tools run commands and checks on the node where the daemonset pod is running.
package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ggai/k8ops/internal/tools"
)

// --- HostExecTool: Execute commands on the node ---

type HostExecTool struct{}

func (t *HostExecTool) Name() string { return "host_exec" }
func (t *HostExecTool) Description() string {
	return "Execute a shell command on the host node. Use for node-level diagnostics: " +
		"disk usage, network, processes, systemctl status, dmesg, journalctl, docker, containerd, etc. " +
		"Commands run via 'nsenter' to access the host namespace when running in a pod."
}
func (t *HostExecTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"command": {Type: "string", Description: "The shell command to execute"},
		"timeout": {Type: "integer", Description: "Timeout in seconds", Default: 30},
	}, []string{"command"})
}
func (t *HostExecTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	command, _ := tools.GetString(args, "command")
	timeoutSec := tools.GetIntDefault(args, "timeout", 30)

	timeout := time.Duration(timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use nsenter to run in host namespace when in a container
	nsenterPath := "/usr/bin/nsenter"
	var cmd *exec.Cmd
	if _, err := os.Stat(nsenterPath); err == nil {
		// We're in a container with host PID access
		cmd = exec.CommandContext(ctx, nsenterPath, "-m", "-u", "-i", "-n", "-p", "--", "/bin/sh", "-c", command)
	} else {
		// Running directly on the node
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if output == "" && stderr.Len() > 0 {
		output = stderr.String()
	}

	// Truncate very long output
	if len(output) > 50000 {
		output = output[:50000] + "\n... (truncated)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("command timed out after %ds", timeoutSec), Output: output}, nil
		}
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("command failed (exit %d): %s", exitCode, stderr.String()),
			Output:  output,
		}, nil
	}

	return &tools.ToolResult{Success: true, Output: output}, nil
}

// --- HostDiskUsageTool: Check disk usage ---

type HostDiskUsageTool struct{}

func (t *HostDiskUsageTool) Name() string { return "host_disk_usage" }
func (t *HostDiskUsageTool) Description() string {
	return "Check disk usage on the host node. Shows mount points, total/used/available space, and inode usage."
}
func (t *HostDiskUsageTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"path": {Type: "string", Description: "Specific path to check (empty for all mounts)", Default: ""},
	}, []string{})
}
func (t *HostDiskUsageTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	path := tools.GetStringDefault(args, "path", "")
	if path == "" {
		path = "/"
	}

	result, err := diskUsage(path)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to stat %s: %v", path, err)}, nil
	}

	// Also get df -h output for human-readable overview
	execTool := &HostExecTool{}
	if dfResult, err := execTool.Execute(ctx, map[string]any{"command": "df -h", "timeout": 10}); err == nil {
		result["df_output"] = dfResult.Output
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- HostNetworkTool: Check network connectivity ---

type HostNetworkTool struct{}

func (t *HostNetworkTool) Name() string { return "host_network_check" }
func (t *HostNetworkTool) Description() string {
	return "Check network status on the host node: interfaces, routes, connections, DNS, and connectivity tests."
}
func (t *HostNetworkTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"action": {Type: "string", Description: "What to check", Enum: []string{"interfaces", "routes", "connections", "dns_test", "ping", "port_check"}},
		"target": {Type: "string", Description: "Target host/IP for ping/dns/port check", Default: ""},
		"port":   {Type: "integer", Description: "Port number for port_check", Default: 0},
	}, []string{"action"})
}
func (t *HostNetworkTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	action, _ := tools.GetString(args, "action")
	target := tools.GetStringDefault(args, "target", "")

	var command string
	switch action {
	case "interfaces":
		command = "ip addr show 2>/dev/null || ifconfig 2>/dev/null"
	case "routes":
		command = "ip route show 2>/dev/null || route -n 2>/dev/null"
	case "connections":
		command = "ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null"
	case "dns_test":
		if target == "" {
			target = "kubernetes.default"
		}
		command = fmt.Sprintf("nslookup %s 2>/dev/null || dig %s 2>/dev/null || host %s", target, target, target)
	case "ping":
		if target == "" {
			return &tools.ToolResult{Success: false, Error: "target required for ping"}, nil
		}
		command = fmt.Sprintf("ping -c 4 -W 2 %s", target)
	case "port_check":
		if target == "" {
			return &tools.ToolResult{Success: false, Error: "target required for port_check"}, nil
		}
		port := tools.GetIntDefault(args, "port", 80)
		command = fmt.Sprintf("timeout 5 bash -c 'echo > /dev/tcp/%s/%d' 2>/dev/null && echo OPEN || echo CLOSED", target, port)
	default:
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": command, "timeout": 15})
}

// --- HostProcessTool: Check running processes ---

type HostProcessTool struct{}

func (t *HostProcessTool) Name() string { return "host_processes" }
func (t *HostProcessTool) Description() string {
	return "List top processes on the host node by CPU or memory usage."
}
func (t *HostProcessTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"sortBy": {Type: "string", Description: "Sort by", Enum: []string{"cpu", "memory"}, Default: "cpu"},
		"limit":  {Type: "integer", Description: "Number of processes to show", Default: 20},
	}, []string{})
}
func (t *HostProcessTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	sortBy := tools.GetStringDefault(args, "sortBy", "cpu")
	limit := tools.GetIntDefault(args, "limit", 20)

	var command string
	if sortBy == "memory" {
		command = fmt.Sprintf("ps aux --sort=-%%mem | head -%d", limit)
	} else {
		command = fmt.Sprintf("ps aux --sort=-%%cpu | head -%d", limit)
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": command, "timeout": 10})
}

// --- HostServiceTool: Check systemd services ---

type HostServiceTool struct{}

func (t *HostServiceTool) Name() string { return "host_service_status" }
func (t *HostServiceTool) Description() string {
	return "Check systemd service status on the host node (e.g. kubelet, containerd, docker)."
}
func (t *HostServiceTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"service": {Type: "string", Description: "Service name (e.g. 'kubelet', 'containerd'). Empty for all."},
		"action":  {Type: "string", Description: "Action", Enum: []string{"status", "list", "logs"}, Default: "status"},
		"lines":   {Type: "integer", Description: "Log lines for 'logs' action", Default: 50},
	}, []string{"service"})
}
func (t *HostServiceTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	service, _ := tools.GetString(args, "service")
	action := tools.GetStringDefault(args, "action", "status")
	lines := tools.GetIntDefault(args, "lines", 50)

	var command string
	switch action {
	case "status":
		command = fmt.Sprintf("systemctl status %s", service)
	case "list":
		command = "systemctl list-units --type=service --state=running | head -30"
	case "logs":
		command = fmt.Sprintf("journalctl -u %s --no-pager -n %d", service, lines)
	default:
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": command, "timeout": 15})
}

// --- HostInfoTool: Get host system info ---

type HostInfoTool struct{}

func (t *HostInfoTool) Name() string { return "host_info" }
func (t *HostInfoTool) Description() string {
	return "Get host system information: hostname, OS, kernel, CPU, memory, uptime."
}
func (t *HostInfoTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *HostInfoTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	hostname, _ := os.Hostname()

	info := map[string]any{
		"hostname": hostname,
		"goOS":     runtime.GOOS,
		"goArch":   runtime.GOARCH,
		"numCPU":   runtime.NumCPU(),
	}

	// Get uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) > 0 {
			info["uptime_seconds"] = parts[0]
		}
	}

	// Get kernel info via exec
	execTool := &HostExecTool{}
	info["cmd_uname"] = ""
	info["cmd_os_release"] = ""
	for _, cmd := range []string{"uname -a", "cat /etc/os-release | head -5"} {
		if result, err := execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 5}); err == nil {
			key := cmd
			if len(key) > 15 {
				key = key[:15]
			}
			info[fmt.Sprintf("cmd_%s", key)] = result.Output
		}
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
