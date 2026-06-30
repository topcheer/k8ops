// Package k8s — Horizontal Pod Autoscaler status tool.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ggai/k8ops/internal/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// --- GetHPATool: Horizontal Pod Autoscaler status ---

type GetHPATool struct{ Client *KubeClient }

func (t *GetHPATool) Name() string { return "k8s_get_hpa" }
func (t *GetHPATool) Description() string {
	return "Get Horizontal Pod Autoscaler status including current/target replicas and metrics thresholds."
}
func (t *GetHPATool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
		"name":      {Type: "string", Description: "HPA name (empty for all)", Default: ""},
	}, []string{})
}
func (t *GetHPATool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")
	name := tools.GetStringDefault(args, "name", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type hpaInfo struct {
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		Target          string `json:"target"`
		MinReplicas     int32  `json:"minReplicas"`
		MaxReplicas     int32  `json:"maxReplicas"`
		CurrentReplicas int32  `json:"currentReplicas"`
		DesiredReplicas int32  `json:"desiredReplicas"`
	}

	if name != "" {
		hpa, err := clientset.AutoscalingV1().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		data, _ := json.MarshalIndent(hpa, "", "  ")
		return &tools.ToolResult{Success: true, Output: string(data)}, nil
	}

	list, err := clientset.AutoscalingV1().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		// Try v2
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("HPA list failed: %v", err)}, nil
	}

	results := make([]hpaInfo, 0, len(list.Items))
	for _, h := range list.Items {
		info := hpaInfo{
			Name: h.Name, Namespace: h.Namespace,
			MinReplicas:     derefInt32(h.Spec.MinReplicas, 1),
			MaxReplicas:     h.Spec.MaxReplicas,
			CurrentReplicas: h.Status.CurrentReplicas,
			DesiredReplicas: h.Status.DesiredReplicas,
		}
		if h.Spec.ScaleTargetRef.Kind != "" {
			info.Target = fmt.Sprintf("%s/%s", h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name)
		}
		results = append(results, info)
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "hpas": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
