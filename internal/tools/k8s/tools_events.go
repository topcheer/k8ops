// Package k8s — event query and filtering tool.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// --- GetEventsTool: Query and filter Kubernetes events ---

type GetEventsTool struct{ Client *KubeClient }

func (t *GetEventsTool) Name() string { return "k8s_get_events" }
func (t *GetEventsTool) Description() string {
	return "List Kubernetes events, optionally filtered by namespace, resource, or severity (warning/error). " +
		"Essential for understanding what happened in the cluster. Returns last 50 events by default."
}
func (t *GetEventsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
		"kind":      {Type: "string", Description: "Filter by involved object kind (e.g. 'Pod', 'Node')", Default: ""},
		"name":      {Type: "string", Description: "Filter by involved object name", Default: ""},
		"reason":    {Type: "string", Description: "Filter by event reason (e.g. 'OOMKilled')", Default: ""},
		"warning":   {Type: "boolean", Description: "Only show Warning events", Default: false},
		"limit":     {Type: "integer", Description: "Max events to return", Default: 50},
	}, []string{})
}
func (t *GetEventsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")
	kind := tools.GetStringDefault(args, "kind", "")
	name := tools.GetStringDefault(args, "name", "")
	reason := tools.GetStringDefault(args, "reason", "")
	warningOnly := tools.GetBool(args, "warning")
	limit := tools.GetIntDefault(args, "limit", 50)

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	listOpts := metav1.ListOptions{Limit: int64(limit * 2)} // over-fetch then filter
	if reason != "" {
		listOpts.FieldSelector = fmt.Sprintf("reason=%s", reason)
	}

	var events []corev1.Event
	if namespace != "" {
		list, err := clientset.CoreV1().Events(namespace).List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		events = list.Items
	} else {
		list, err := clientset.CoreV1().Events("").List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		events = list.Items
	}

	// Filter
	var filtered []corev1.Event
	for _, e := range events {
		if warningOnly && e.Type != "Warning" {
			continue
		}
		if kind != "" && e.InvolvedObject.Kind != kind {
			continue
		}
		if name != "" && e.InvolvedObject.Name != name {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}

	// Sort by last timestamp descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].LastTimestamp.After(filtered[j].LastTimestamp.Time)
	})

	type eventInfo struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Object    string `json:"object"`
		Namespace string `json:"namespace"`
		Count     int32  `json:"count"`
		LastTime  string `json:"lastTimestamp"`
		FirstTime string `json:"firstTimestamp"`
	}

	results := make([]eventInfo, 0, len(filtered))
	for _, e := range filtered {
		results = append(results, eventInfo{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Object:    fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			Namespace: e.InvolvedObject.Namespace,
			Count:     e.Count,
			LastTime:  e.LastTimestamp.Format("2006-01-02T15:04:05Z"),
			FirstTime: e.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}

	data, _ := json.MarshalIndent(map[string]any{
		"count":  len(results),
		"events": results,
	}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
