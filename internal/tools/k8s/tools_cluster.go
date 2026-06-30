// Package k8s — cluster info, namespace, and node drain tools.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// --- GetClusterVersionTool: Get cluster version info ---

type GetClusterVersionTool struct{ Client *KubeClient }

func (t *GetClusterVersionTool) Name() string { return "k8s_cluster_info" }
func (t *GetClusterVersionTool) Description() string {
	return "Get Kubernetes cluster version, platform info, and installed operator versions. " +
		"Useful to understand what the cluster is running."
}
func (t *GetClusterVersionTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *GetClusterVersionTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	info, err := clientset.ServerVersion()
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	result := map[string]any{
		"gitVersion": info.GitVersion,
		"gitCommit":  info.GitCommit,
		"buildDate":  info.BuildDate,
		"platform":   info.Platform,
		"major":      info.Major,
		"minor":      info.Minor,
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		result["nodeCount"] = len(nodes.Items)
	}

	nss, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		result["namespaceCount"] = len(nss.Items)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- GetNamespacesTool: List namespaces with status ---

type GetNamespacesTool struct{ Client *KubeClient }

func (t *GetNamespacesTool) Name() string { return "k8s_get_namespaces" }
func (t *GetNamespacesTool) Description() string {
	return "List all Kubernetes namespaces with their status (Active/Terminating) and labels."
}
func (t *GetNamespacesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *GetNamespacesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	list, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type nsInfo struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Age    string `json:"age"`
	}
	results := make([]nsInfo, 0, len(list.Items))
	for _, ns := range list.Items {
		results = append(results, nsInfo{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    formatAge(ns.CreationTimestamp.Time),
		})
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "namespaces": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- DrainNodeTool: Safely drain a node ---

type DrainNodeTool struct{ Client *KubeClient }

func (t *DrainNodeTool) Name() string { return "k8s_drain_node" }
func (t *DrainNodeTool) Description() string {
	return "Safely drain a node: cordon it, then evict all pods respecting PDBs. " +
		"Useful before node maintenance. This is a potentially disruptive operation."
}
func (t *DrainNodeTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"node":               {Type: "string", Description: "Node name to drain"},
		"ignoreDaemonSets":   {Type: "boolean", Description: "Ignore DaemonSet pods", Default: true},
		"deleteEmptyDirData": {Type: "boolean", Description: "Delete pods using emptyDir", Default: true},
		"timeout":            {Type: "integer", Description: "Timeout in seconds", Default: 120},
	}, []string{"node"})
}
func (t *DrainNodeTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	nodeName, _ := tools.GetString(args, "node")
	ignoreDaemonSets := tools.GetBool(args, "ignoreDaemonSets")
	if !ignoreDaemonSets {
		ignoreDaemonSets = true
	}

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	// 1. Cordon the node
	_, err = clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType,
		[]byte(`{"spec":{"unschedulable":true}}`), metav1.PatchOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to cordon: %v", err)}, nil
	}

	// 2. Get all pods on this node
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to list pods: %v", err)}, nil
	}

	evicted := 0
	skipped := 0
	var errors []string

	for _, pod := range pods.Items {
		if ignoreDaemonSets && isDaemonSetPod(pod) {
			skipped++
			continue
		}
		if _, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
			skipped++
			continue
		}
		gracePeriod := int64(30)
		err := clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			Preconditions: &metav1.Preconditions{
				ResourceVersion: &pod.ResourceVersion,
			},
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s: %v", pod.Namespace, pod.Name, err))
		} else {
			evicted++
		}
	}

	result := map[string]any{
		"node":    nodeName,
		"evicted": evicted,
		"skipped": skipped,
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

func isDaemonSetPod(pod corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
