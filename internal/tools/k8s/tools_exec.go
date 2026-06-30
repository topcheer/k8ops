// Package k8s — exec into pod tool.
package k8s

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// --- ExecInPodTool: Execute a command inside a running pod ---

type ExecInPodTool struct{ Client *KubeClient }

func (t *ExecInPodTool) Name() string { return "k8s_exec" }
func (t *ExecInPodTool) Description() string {
	return "Execute a command inside a running pod's container. " +
		"Useful for debugging application issues from inside the pod. " +
		"Returns stdout and stderr."
}
func (t *ExecInPodTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"name":      {Type: "string", Description: "Pod name"},
		"namespace": {Type: "string", Description: "Namespace", Default: "default"},
		"container": {Type: "string", Description: "Container name (optional)", Default: ""},
		"command":   {Type: "string", Description: "Command to execute (e.g. 'ls -la /app')"},
	}, []string{"name", "command"})
}
func (t *ExecInPodTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	container := tools.GetStringDefault(args, "container", "")
	command, _ := tools.GetString(args, "command")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   []string{"/bin/sh", "-c", command},
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(t.Client.config, "POST", req.URL())
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("exec setup failed: %v", err)}, nil
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("exec failed: %v", err),
			Output:  stderr.String(),
		}, nil
	}

	output := stdout.String()
	if output == "" && stderr.Len() > 0 {
		output = stderr.String()
	}
	return &tools.ToolResult{Success: true, Output: output}, nil
}

// --- Suppress unused import ---
var _ = metav1.GetOptions{}
