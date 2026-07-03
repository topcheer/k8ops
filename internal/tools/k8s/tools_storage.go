// Package k8s — storage status tool.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// --- GetStorageTool: PVC/PV/StorageClass status ---

type GetStorageTool struct{ Client *KubeClient }

func (t *GetStorageTool) Name() string { return "k8s_get_storage" }
func (t *GetStorageTool) Description() string {
	return "Get storage status: PersistentVolumeClaims, PersistentVolumes, and StorageClasses. " +
		"Useful for diagnosing PVC Pending issues."
}
func (t *GetStorageTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"type":      {Type: "string", Description: "Resource type", Enum: []string{"pvc", "pv", "storageclass", "all"}, Default: "all"},
		"namespace": {Type: "string", Description: "Namespace for PVC (empty for all)", Default: ""},
		"warning":   {Type: "boolean", Description: "Only show problematic ones (Pending/Lost)", Default: false},
	}, []string{})
}
func (t *GetStorageTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	resourceType := tools.GetStringDefault(args, "type", "all")
	namespace := tools.GetStringDefault(args, "namespace", "")
	warningOnly := tools.GetBool(args, "warning")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	result := map[string]any{}

	if resourceType == "pvc" || resourceType == "all" {
		list, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			type pvcInfo struct {
				Name         string `json:"name"`
				Namespace    string `json:"namespace"`
				Status       string `json:"status"`
				Capacity     string `json:"capacity"`
				StorageClass string `json:"storageClass"`
				Volume       string `json:"volumeName"`
			}
			pvcs := make([]pvcInfo, 0)
			for _, p := range list.Items {
				phase := string(p.Status.Phase)
				if warningOnly && phase != "Pending" && phase != "Lost" {
					continue
				}
				cap := ""
				if p.Status.Capacity != nil {
					if q, ok := p.Status.Capacity[corev1.ResourceStorage]; ok {
						cap = q.String()
					}
				}
				pvcs = append(pvcs, pvcInfo{
					Name: p.Name, Namespace: p.Namespace, Status: phase,
					Capacity: cap, StorageClass: derefStr(p.Spec.StorageClassName),
					Volume: p.Spec.VolumeName,
				})
			}
			result["pvcs"] = pvcs
		}
	}

	if resourceType == "pv" || resourceType == "all" {
		list, err := clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
		if err == nil {
			type pvInfo struct {
				Name     string `json:"name"`
				Status   string `json:"status"`
				Capacity string `json:"capacity"`
				Claim    string `json:"claim"`
				Reason   string `json:"reason,omitempty"`
			}
			pvs := make([]pvInfo, 0)
			for _, p := range list.Items {
				phase := string(p.Status.Phase)
				if warningOnly && phase != "Pending" && phase != "Failed" && phase != "Released" {
					continue
				}
				claim := ""
				if p.Spec.ClaimRef != nil {
					claim = fmt.Sprintf("%s/%s", p.Spec.ClaimRef.Namespace, p.Spec.ClaimRef.Name)
				}
				capQ, ok := p.Spec.Capacity[corev1.ResourceStorage]
				capStr := ""
				if ok {
					capStr = capQ.String()
				}
				pvs = append(pvs, pvInfo{
					Name: p.Name, Status: phase,
					Capacity: capStr,
					Claim:    claim, Reason: p.Status.Reason,
				})
			}
			result["pvs"] = pvs
		}
	}

	if resourceType == "storageclass" || resourceType == "all" {
		list, err := clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err == nil {
			type scInfo struct {
				Name        string `json:"name"`
				Provisioner string `json:"provisioner"`
				Default     bool   `json:"default"`
			}
			scs := make([]scInfo, 0)
			for _, s := range list.Items {
				isDefault := false
				for k, v := range s.Annotations {
					if k == "storageclass.kubernetes.io/is-default-class" && v == "true" {
						isDefault = true
					}
				}
				scs = append(scs, scInfo{Name: s.Name, Provisioner: s.Provisioner, Default: isDefault})
			}
			result["storageclasses"] = scs
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
