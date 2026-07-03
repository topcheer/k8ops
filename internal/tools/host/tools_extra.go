package host

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ggai/k8ops/internal/tools"
)

// --- HostDmesgTool: Kernel ring buffer ---

type HostDmesgTool struct{}

func (t *HostDmesgTool) Name() string { return "host_dmesg" }
func (t *HostDmesgTool) Description() string {
	return "Read kernel ring buffer messages (dmesg). Critical for diagnosing: " +
		"OOM killer events, hardware errors, filesystem corruption, driver issues, " +
		"network errors, and kernel panics."
}
func (t *HostDmesgTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"lines":  {Type: "integer", Description: "Last N lines", Default: 100},
		"filter": {Type: "string", Description: "Grep filter (e.g. 'oom', 'error')", Default: ""},
	}, []string{})
}
func (t *HostDmesgTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	lines := tools.GetIntDefault(args, "lines", 100)
	filter := tools.GetStringDefault(args, "filter", "")

	cmd := fmt.Sprintf("dmesg --time-format iso | tail -%d", lines)
	if filter != "" {
		cmd = fmt.Sprintf("dmesg --time-format iso | grep -i '%s' | tail -%d", filter, lines)
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 10})
}

// --- HostContainerRuntimeTool: Docker/containerd status and info ---

type HostContainerRuntimeTool struct{}

func (t *HostContainerRuntimeTool) Name() string { return "host_container_runtime" }
func (t *HostContainerRuntimeTool) Description() string {
	return "Check container runtime (containerd/docker/cri-o) status, running containers, " +
		"image storage, and recent errors. Essential for diagnosing container runtime issues."
}
func (t *HostContainerRuntimeTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"action": {Type: "string", Description: "What to check", Enum: []string{"info", "containers", "images", "logs", "version"}, Default: "info"},
		"lines":  {Type: "integer", Description: "Log lines for 'logs' action", Default: 50},
	}, []string{"action"})
}
func (t *HostContainerRuntimeTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	action, _ := tools.GetString(args, "action")
	lines := tools.GetIntDefault(args, "lines", 50)

	execTool := &HostExecTool{}

	// Detect which runtime is available
	var cmd string
	switch action {
	case "info":
		cmd = `ctr version 2>/dev/null && echo "---containerd---" && crictl info 2>/dev/null || docker info 2>/dev/null || echo "No container runtime CLI found"`
	case "containers":
		cmd = `crictl ps 2>/dev/null || docker ps 2>/dev/null || ctr task ls 2>/dev/null || echo "No container runtime CLI found"`
	case "images":
		cmd = `crictl images 2>/dev/null | head -30 || docker images 2>/dev/null | head -30 || echo "No container runtime CLI found"`
	case "logs":
		cmd = fmt.Sprintf(`journalctl -u containerd --no-pager -n %d 2>/dev/null || journalctl -u docker --no-pager -n %d 2>/dev/null || echo "No runtime service logs found"`, lines, lines)
	case "version":
		cmd = `containerd --version 2>/dev/null; docker --version 2>/dev/null; crictl --version 2>/dev/null`
	default:
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}

	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 15})
}

// --- HostKubeletTool: Kubelet status and logs ---

type HostKubeletTool struct{}

func (t *HostKubeletTool) Name() string { return "host_kubelet" }
func (t *HostKubeletTool) Description() string {
	return "Check kubelet health: process status, configuration, logs, and recent errors. " +
		"Critical for diagnosing node-level scheduling and pod lifecycle issues."
}
func (t *HostKubeletTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"action": {Type: "string", Description: "What to check", Enum: []string{"status", "logs", "config", "errors"}, Default: "status"},
		"lines":  {Type: "integer", Description: "Log lines", Default: 50},
	}, []string{"action"})
}
func (t *HostKubeletTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	action, _ := tools.GetString(args, "action")
	lines := tools.GetIntDefault(args, "lines", 50)

	var cmd string
	switch action {
	case "status":
		cmd = "systemctl status kubelet 2>/dev/null || pgrep -a kubelet"
	case "logs":
		cmd = fmt.Sprintf("journalctl -u kubelet --no-pager -n %d", lines)
	case "config":
		cmd = "cat /var/lib/kubelet/config.yaml 2>/dev/null || cat /etc/kubernetes/kubelet.conf 2>/dev/null || echo 'kubelet config not found'"
	case "errors":
		cmd = fmt.Sprintf("journalctl -u kubelet --no-pager -n 200 -p err 2>/dev/null | tail -%d", lines)
	default:
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 15})
}

// --- HostIPTablesTool: IPVS/iptables rules for network debugging ---

type HostIPTablesTool struct{}

func (t *HostIPTablesTool) Name() string { return "host_iptables" }
func (t *HostIPTablesTool) Description() string {
	return "Check iptables/IPVS rules on the host. Essential for diagnosing: " +
		"service connectivity issues, NodePort/LoadBalancer problems, " +
		"network policy blocks, kube-proxy configuration issues."
}
func (t *HostIPTablesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"table": {Type: "string", Description: "Table name", Enum: []string{"nat", "filter", "mangle", "all"}, Default: "nat"},
		"chain": {Type: "string", Description: "Chain name (e.g. 'KUBE-SERVICES')", Default: ""},
		"ipvs":  {Type: "boolean", Description: "Show IPVS rules instead", Default: false},
	}, []string{})
}
func (t *HostIPTablesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	table := tools.GetStringDefault(args, "table", "nat")
	chain := tools.GetStringDefault(args, "chain", "")
	useIPVS := tools.GetBool(args, "ipvs")

	var cmd string
	if useIPVS {
		cmd = "ipvsadm -Ln 2>/dev/null || echo 'ipvsadm not found'"
	} else {
		if table == "all" {
			cmd = "iptables-save 2>/dev/null | head -200"
		} else {
			if chain != "" {
				cmd = fmt.Sprintf("iptables -t %s -L %s -n -v 2>/dev/null || echo 'iptables not available'", table, chain)
			} else {
				cmd = fmt.Sprintf("iptables -t %s -L -n -v 2>/dev/null | head -100 || echo 'iptables not available'", table)
			}
		}
	}

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 10})
}

// --- HostMountsTool: Filesystem mount information ---

type HostMountsTool struct{}

func (t *HostMountsTool) Name() string { return "host_mounts" }
func (t *HostMountsTool) Description() string {
	return "Get filesystem mount information on the host node. " +
		"Shows all mount points, filesystem types, mount options. " +
		"Useful for diagnosing PV mount failures, disk full issues, and read-only filesystems."
}
func (t *HostMountsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"readOnly": {Type: "boolean", Description: "Only show read-only mounts", Default: false},
	}, []string{})
}
func (t *HostMountsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	readOnlyOnly := tools.GetBool(args, "readOnly")

	cmd := "findmnt --json 2>/dev/null || cat /proc/mounts"
	if readOnlyOnly {
		cmd = "findmnt -ro 2>/dev/null || cat /proc/mounts | grep ' ro,'"
	}

	execTool := &HostExecTool{}
	result, err := execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 10})
	if err != nil {
		return result, err
	}

	// Try to parse JSON and re-pretty print
	if result.Success {
		var parsed map[string]any
		if json.Unmarshal([]byte(result.Output), &parsed) == nil {
			data, _ := json.MarshalIndent(parsed, "", "  ")
			return &tools.ToolResult{Success: true, Output: string(data)}, nil
		}
	}
	return result, nil
}

// --- HostIOTopTool: Disk I/O monitoring ---

type HostDiskIOTool struct{}

func (t *HostDiskIOTool) Name() string { return "host_disk_io" }
func (t *HostDiskIOTool) Description() string {
	return "Check disk I/O statistics on the host. Shows per-device read/write rates and " +
		"top processes by I/O. Useful for diagnosing slow disk performance."
}
func (t *HostDiskIOTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *HostDiskIOTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	cmd := `echo "=== IOSTAT ===" && iostat -x 1 1 2>/dev/null || echo "iostat not available" && echo "=== TOP IO PROCESSES ===" && iotop -b -n 1 -o 2>/dev/null | head -20 || echo "iotop not available"`

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 15})
}

// --- HostMemoryInfoTool: Detailed memory info ---

type HostMemoryInfoTool struct{}

func (t *HostMemoryInfoTool) Name() string { return "host_memory_info" }
func (t *HostMemoryInfoTool) Description() string {
	return "Get detailed host memory information including total, free, cached, swap usage, " +
		"and cgroup memory limits. More detailed than 'free -h'."
}
func (t *HostMemoryInfoTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *HostMemoryInfoTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	cmd := `echo "=== MEMORY ===" && free -h && echo "" && echo "=== MEMINFO ===" && head -20 /proc/meminfo && echo "" && echo "=== SLAB ===" && cat /proc/slabinfo 2>/dev/null | head -10 || echo "slabinfo not accessible"`

	execTool := &HostExecTool{}
	return execTool.Execute(ctx, map[string]any{"command": cmd, "timeout": 10})
}
