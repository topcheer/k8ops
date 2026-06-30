// Package remediation provides tools for executing fix actions on the cluster.
package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Remediator struct {
	config  *rest.Config
	dynamic dynamic.Interface
}

func NewRemediator(config *rest.Config) (*Remediator, error) {
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Remediator{config: config, dynamic: dyn}, nil
}

func (r *Remediator) parseGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 1 {
		gvr := schema.GroupVersionResource{Group: "", Version: parts[0]}
		gvr.Resource = kindToResource(kind)
		return gvr, nil
	}
	if len(parts) == 2 {
		gvr := schema.GroupVersionResource{Group: parts[0], Version: parts[1]}
		gvr.Resource = kindToResource(kind)
		return gvr, nil
	}
	return schema.GroupVersionResource{}, fmt.Errorf("invalid apiVersion: %s", apiVersion)
}

func kindToResource(kind string) string {
	k := strings.ToLower(kind)
	if strings.HasSuffix(k, "s") {
		return k + "es"
	}
	if strings.HasSuffix(k, "y") {
		return strings.TrimSuffix(k, "y") + "ies"
	}
	return k + "s"
}

// --- PatchResourceTool ---

type PatchResourceTool struct{ R *Remediator }

func (t *PatchResourceTool) Name() string { return "k8s_patch_resource" }
func (t *PatchResourceTool) Description() string {
	return "Patch a Kubernetes resource using a JSON merge patch. Works with any resource type including CRDs. " +
		"Use this to update deployment replicas, resource limits, annotations, etc."
}
func (t *PatchResourceTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion": {Type: "string", Description: "API version"},
		"kind":       {Type: "string", Description: "Resource kind"},
		"name":       {Type: "string", Description: "Resource name"},
		"namespace":  {Type: "string", Description: "Namespace", Default: "default"},
		"patch":      {Type: "string", Description: "JSON merge patch (e.g. '{\"spec\":{\"replicas\":3}}')"},
	}, []string{"apiVersion", "kind", "name", "patch"})
}
func (t *PatchResourceTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	apiVersion, _ := tools.GetString(args, "apiVersion")
	kind, _ := tools.GetString(args, "kind")
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	patchStr, _ := tools.GetString(args, "patch")

	gvr, err := t.R.parseGVR(apiVersion, kind)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var ri dynamic.ResourceInterface = t.R.dynamic.Resource(gvr)
	if namespace != "" {
		ri = t.R.dynamic.Resource(gvr).Namespace(namespace)
	}

	// Store pre-patch state for rollback
	before, _ := ri.Get(ctx, name, metav1.GetOptions{})

	obj, err := ri.Patch(ctx, name, types.MergePatchType, []byte(patchStr), metav1.PatchOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("patch failed: %v", err)}, nil
	}

	result := map[string]any{
		"message": fmt.Sprintf("Successfully patched %s/%s", kind, name),
		"patched": true,
	}
	if before != nil {
		beforeJSON, _ := json.Marshal(before.Object)
		result["previous_state"] = string(beforeJSON)
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	objJSON, _ := json.MarshalIndent(obj.Object, "", "  ")
	result["new_state"] = string(objJSON)

	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- ScaleResourceTool ---

type ScaleResourceTool struct{ R *Remediator }

func (t *ScaleResourceTool) Name() string { return "k8s_scale_resource" }
func (t *ScaleResourceTool) Description() string {
	return "Scale a Deployment, StatefulSet, or ReplicaSet to a specific number of replicas."
}
func (t *ScaleResourceTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion": {Type: "string", Description: "API version (e.g. 'apps/v1')"},
		"kind":       {Type: "string", Description: "Resource kind (Deployment, StatefulSet, ReplicaSet)"},
		"name":       {Type: "string", Description: "Resource name"},
		"namespace":  {Type: "string", Description: "Namespace", Default: "default"},
		"replicas":   {Type: "integer", Description: "Target number of replicas"},
	}, []string{"apiVersion", "kind", "name", "replicas"})
}
func (t *ScaleResourceTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	apiVersion, _ := tools.GetString(args, "apiVersion")
	kind, _ := tools.GetString(args, "kind")
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	replicas := tools.GetIntDefault(args, "replicas", 1)

	patchStr := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	patchTool := &PatchResourceTool{R: t.R}
	patchArgs := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"namespace":  namespace,
		"patch":      patchStr,
	}
	result, err := patchTool.Execute(ctx, patchArgs)
	if err != nil {
		return result, err
	}
	// Enhance message
	if result.Success {
		result.Output = fmt.Sprintf("Scaled %s/%s in %s to %d replicas.\n%s", kind, name, namespace, replicas, result.Output)
	}
	return result, nil
}

// --- RestartPodTool ---

type RestartPodTool struct{ R *Remediator }

func (t *RestartPodTool) Name() string { return "k8s_restart_pod" }
func (t *RestartPodTool) Description() string {
	return "Restart a pod by deleting it (the controller will recreate it) or restart a Deployment by rolling restart."
}
func (t *RestartPodTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion": {Type: "string", Description: "API version", Default: "v1"},
		"kind":       {Type: "string", Description: "Kind: 'Pod' to delete a single pod, or 'Deployment' for rolling restart"},
		"name":       {Type: "string", Description: "Resource name"},
		"namespace":  {Type: "string", Description: "Namespace", Default: "default"},
	}, []string{"kind", "name"})
}
func (t *RestartPodTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	kind, _ := tools.GetString(args, "kind")
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	apiVersion := tools.GetStringDefault(args, "apiVersion", "v1")

	if strings.EqualFold(kind, "Deployment") {
		// Rolling restart via annotation patch
		patchStr := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().Format("2006-01-02T15:04:05Z07:00"))
		if apiVersion == "v1" {
			apiVersion = "apps/v1"
		}
		patchTool := &PatchResourceTool{R: t.R}
		return patchTool.Execute(ctx, map[string]any{
			"apiVersion": apiVersion,
			"kind":       "Deployment",
			"name":       name,
			"namespace":  namespace,
			"patch":      patchStr,
		})
	}

	// Delete single pod
	gvr, err := t.R.parseGVR(apiVersion, kind)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	err = t.R.dynamic.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to delete pod: %v", err)}, nil
	}

	return &tools.ToolResult{Success: true, Output: fmt.Sprintf("Pod %s/%s deleted (controller will recreate it)", namespace, name)}, nil
}

// --- CordonNodeTool ---

type CordonNodeTool struct{ R *Remediator }

func (t *CordonNodeTool) Name() string { return "k8s_cordon_node" }
func (t *CordonNodeTool) Description() string {
	return "Cordon a node to mark it as unschedulable. New pods will not be scheduled on it."
}
func (t *CordonNodeTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"node":    {Type: "string", Description: "Node name"},
		"uncordon": {Type: "boolean", Description: "If true, uncordon instead", Default: false},
	}, []string{"node"})
}
func (t *CordonNodeTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	nodeName, _ := tools.GetString(args, "node")
	uncordon := tools.GetBool(args, "uncordon")

	clientset, err := kubernetes.NewForConfig(t.R.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Verify node exists
	_, err = clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get node: %v", err)}, nil
	}

	patchStr := `{"spec":{"unschedulable":%t}}`
	if uncordon {
		patchStr = fmt.Sprintf(patchStr, false)
	} else {
		patchStr = fmt.Sprintf(patchStr, true)
	}

	_, err = clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, []byte(patchStr), metav1.PatchOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to patch node: %v", err)}, nil
	}

	action := "cordoned"
	if uncordon {
		action = "uncordoned"
	}
	return &tools.ToolResult{Success: true, Output: fmt.Sprintf("Node %s successfully %s", nodeName, action)}, nil
}

// --- DeletePodEvictedTool ---

type DeleteEvictedPodsTool struct{ R *Remediator }

func (t *DeleteEvictedPodsTool) Name() string { return "k8s_delete_evicted_pods" }
func (t *DeleteEvictedPodsTool) Description() string {
	return "Delete all pods in Evicted/Failed state in a namespace (or all namespaces)."
}
func (t *DeleteEvictedPodsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
	}, []string{})
}
func (t *DeleteEvictedPodsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")

	clientset, err := kubernetes.NewForConfig(t.R.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	listOpts := metav1.ListOptions{
		FieldSelector: "status.phase=Failed",
	}

	var pods []corev1.Pod
	if namespace != "" {
		podList, err := clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		pods = podList.Items
	} else {
		podList, err := clientset.CoreV1().Pods("").List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		pods = podList.Items
	}

	deleted := 0
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodFailed {
			_ = clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
			deleted++
		}
	}

	return &tools.ToolResult{Success: true, Output: fmt.Sprintf("Deleted %d failed/evicted pods", deleted)}, nil
}

// --- ApplyManifestTool ---

type ApplyManifestTool struct{ R *Remediator }

func (t *ApplyManifestTool) Name() string { return "k8s_apply_manifest" }
func (t *ApplyManifestTool) Description() string {
	return "Apply a Kubernetes manifest (YAML/JSON) to the cluster. Can create or update any resource including CRDs."
}
func (t *ApplyManifestTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"manifest": {Type: "string", Description: "YAML or JSON manifest"},
		"namespace": {Type: "string", Description: "Namespace (overrides manifest)", Default: ""},
	}, []string{"manifest"})
}
func (t *ApplyManifestTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	manifest, _ := tools.GetString(args, "manifest")

	// Parse the manifest
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON([]byte(manifest)); err != nil {
		// Try YAML
		jsonBytes, err2 := yamlToJSON([]byte(manifest))
		if err2 != nil {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to parse manifest: %v", err)}, nil
		}
		if err := obj.UnmarshalJSON(jsonBytes); err != nil {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to parse manifest: %v", err)}, nil
		}
	}

	gvr, err := t.R.parseGVR(obj.GetAPIVersion(), obj.GetKind())
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = tools.GetStringDefault(args, "namespace", "")
	}

	var ri dynamic.ResourceInterface = t.R.dynamic.Resource(gvr)
	if namespace != "" {
		ri = t.R.dynamic.Resource(gvr).Namespace(namespace)
	}

	result, err := ri.Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		// Try update if already exists
		if strings.Contains(err.Error(), "already exists") {
			existing, err2 := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
			if err2 != nil {
				return &tools.ToolResult{Success: false, Error: err.Error()}, nil
			}
			obj.SetResourceVersion(existing.GetResourceVersion())
			result, err = ri.Update(ctx, obj, metav1.UpdateOptions{})
			if err != nil {
				return &tools.ToolResult{Success: false, Error: fmt.Sprintf("update failed: %v", err)}, nil
			}
		} else {
			return &tools.ToolResult{Success: false, Error: fmt.Sprintf("create failed: %v", err)}, nil
		}
	}

	return &tools.ToolResult{Success: true, Output: fmt.Sprintf("Applied %s/%s", result.GetKind(), result.GetName())}, nil
}

// yamlToJSON converts YAML to JSON bytes using sigs.k8s.io/yaml.
func yamlToJSON(yamlData []byte) ([]byte, error) {
	return yaml.YAMLToJSON(yamlData)
}
