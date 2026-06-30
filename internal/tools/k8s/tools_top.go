// Package k8s — resource usage metrics tool.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
)

// --- GetTopTool: Pod/Node resource usage metrics ---

type GetTopTool struct{ Client *KubeClient }

func (t *GetTopTool) Name() string { return "k8s_top" }
func (t *GetTopTool) Description() string {
	return "Get current resource usage (CPU/memory) for pods or nodes. " +
		"Requires metrics-server to be installed. Similar to 'kubectl top pods/nodes'."
}
func (t *GetTopTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"resource":  {Type: "string", Description: "What to query", Enum: []string{"pods", "nodes"}, Default: "pods"},
		"namespace": {Type: "string", Description: "Namespace for pods (empty for all)", Default: ""},
		"sortBy":    {Type: "string", Description: "Sort by", Enum: []string{"cpu", "memory"}, Default: "cpu"},
		"limit":     {Type: "integer", Description: "Max items", Default: 20},
	}, []string{"resource"})
}
func (t *GetTopTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	resourceType := tools.GetStringDefault(args, "resource", "pods")
	namespace := tools.GetStringDefault(args, "namespace", "")
	sortBy := tools.GetStringDefault(args, "sortBy", "cpu")
	limit := tools.GetIntDefault(args, "limit", 20)

	metricsClient, err := metricsv1beta1.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("metrics client error: %v", err)}, nil
	}

	type usage struct {
		Name      string  `json:"name"`
		Namespace string  `json:"namespace,omitempty"`
		CPU       string  `json:"cpu"`
		CPUMili   int64   `json:"cpuMillis"`
		Memory    string  `json:"memory"`
		MemoryMB  float64 `json:"memoryMB"`
	}

	var results []usage

	if resourceType == "nodes" {
		list, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get node metrics (is metrics-server installed?): %v", err)}, nil
		}
		for _, m := range list.Items {
			cpu := m.Usage[corev1.ResourceCPU]
			mem := m.Usage[corev1.ResourceMemory]
			results = append(results, usage{
				Name: m.Name, CPU: cpu.String(), CPUMili: cpu.MilliValue(),
				Memory: mem.String(), MemoryMB: float64(mem.Value()) / 1024 / 1024,
			})
		}
	} else {
		ns := namespace
		if ns == "" {
			ns = ""
		}
		list, err := metricsClient.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get pod metrics (is metrics-server installed?): %v", err)}, nil
		}
		for _, m := range list.Items {
			var totalCPU int64
			var totalMem int64
			for _, c := range m.Containers {
				if cpuQ, ok := c.Usage[corev1.ResourceCPU]; ok {
					totalCPU += (&cpuQ).MilliValue()
				}
				if memQ, ok := c.Usage[corev1.ResourceMemory]; ok {
					totalMem += (&memQ).Value()
				}
			}
			results = append(results, usage{
				Name: m.Name, Namespace: m.Namespace,
				CPU: fmt.Sprintf("%dm", totalCPU), CPUMili: totalCPU,
				Memory: fmt.Sprintf("%dMi", totalMem/1024/1024), MemoryMB: float64(totalMem) / 1024 / 1024,
			})
		}
	}

	// Sort
	sort.Slice(results, func(i, j int) bool {
		if sortBy == "memory" {
			return results[i].MemoryMB > results[j].MemoryMB
		}
		return results[i].CPUMili > results[j].CPUMili
	})

	if len(results) > limit {
		results = results[:limit]
	}

	data, _ := json.MarshalIndent(map[string]any{"resource": resourceType, "count": len(results), "items": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
