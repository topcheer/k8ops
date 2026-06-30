// Package k8s — Pod Disruption Budget status tool.
package k8s

import (
	"context"
	"encoding/json"

	"github.com/ggai/k8ops/internal/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// --- GetPDBTool: Pod Disruption Budget status ---

type GetPDBTool struct{ Client *KubeClient }

func (t *GetPDBTool) Name() string { return "k8s_get_pdb" }
func (t *GetPDBTool) Description() string {
	return "Get Pod Disruption Budget status. Shows allowed disruptions and current/desired healthy pods."
}
func (t *GetPDBTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
	}, []string{})
}
func (t *GetPDBTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	list, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type pdbInfo struct {
		Name               string `json:"name"`
		Namespace          string `json:"namespace"`
		MinAvailable       string `json:"minAvailable,omitempty"`
		MaxUnavailable     string `json:"maxUnavailable,omitempty"`
		CurrentHealthy     int32  `json:"currentHealthy"`
		DesiredHealthy     int32  `json:"desiredHealthy"`
		AllowedDisruptions int32  `json:"allowedDisruptions"`
	}

	results := make([]pdbInfo, 0, len(list.Items))
	for _, p := range list.Items {
		info := pdbInfo{
			Name:               p.Name,
			Namespace:          p.Namespace,
			CurrentHealthy:     p.Status.CurrentHealthy,
			DesiredHealthy:     p.Status.DesiredHealthy,
			AllowedDisruptions: p.Status.DisruptionsAllowed,
		}
		if p.Spec.MinAvailable != nil {
			info.MinAvailable = p.Spec.MinAvailable.String()
		}
		if p.Spec.MaxUnavailable != nil {
			info.MaxUnavailable = p.Spec.MaxUnavailable.String()
		}
		results = append(results, info)
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "pdbs": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
