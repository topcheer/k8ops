package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeClient wraps Kubernetes client connections.
type KubeClient struct {
	config  *rest.Config
	dynamic dynamic.Interface
	decoder runtime.Decoder
}

// NewKubeClient creates a new Kubernetes client from in-cluster or kubeconfig.
func NewKubeClient() (*KubeClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
		}
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &KubeClient{
		config:  config,
		dynamic: dynClient,
		decoder: serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer(),
	}, nil
}

// NewKubeClientFromConfig creates a KubeClient from an existing rest.Config.
func NewKubeClientFromConfig(config *rest.Config) (*KubeClient, error) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return &KubeClient{
		config:  config,
		dynamic: dynClient,
		decoder: serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer(),
	}, nil
}

// DynamicClient returns the dynamic interface for CRD browsing.
func (c *KubeClient) DynamicClient() dynamic.Interface {
	return c.dynamic
}

func (k *KubeClient) parseGroupVersionResource(apiVersion string) (schema.GroupVersionResource, error) {
	// Handle "group/version" or just "version" (core API)
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 1 {
		// Core API group (v1, etc.)
		return schema.GroupVersionResource{Group: "", Version: parts[0], Resource: ""}, nil
	}
	if len(parts) == 2 {
		return schema.GroupVersionResource{Group: parts[0], Version: parts[1]}, nil
	}
	return schema.GroupVersionResource{}, fmt.Errorf("invalid apiVersion: %s", apiVersion)
}

// kindToResource converts a kind name to a plural resource name (simple heuristic).
func kindToResource(kind string) string {
	kind = strings.ToLower(kind)
	if strings.HasSuffix(kind, "s") {
		return kind + "es"
	}
	if strings.HasSuffix(kind, "y") {
		return strings.TrimSuffix(kind, "y") + "ies"
	}
	return kind + "s"
}

// --- GetResource Tool ---

type GetResourceTool struct{ Client *KubeClient }

func (t *GetResourceTool) Name() string { return "k8s_get_resource" }
func (t *GetResourceTool) Description() string {
	return "Get a Kubernetes resource by kind, name, and namespace. Works with any resource type including CRDs. " +
		"Returns the full resource as JSON."
}
func (t *GetResourceTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion": {Type: "string", Description: "API version (e.g. 'v1', 'apps/v1', 'argoproj.io/v1alpha1')"},
		"kind":       {Type: "string", Description: "Resource kind (e.g. 'Pod', 'Deployment', 'Application')"},
		"name":       {Type: "string", Description: "Resource name"},
		"namespace":  {Type: "string", Description: "Namespace (omit for cluster-scoped resources)", Default: ""},
	}, []string{"apiVersion", "kind", "name"})
}
func (t *GetResourceTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	apiVersion, _ := tools.GetString(args, "apiVersion")
	kind, _ := tools.GetString(args, "kind")
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")

	gvr, err := t.Client.parseGroupVersionResource(apiVersion)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}
	gvr.Resource = kindToResource(kind)

	var ri dynamic.ResourceInterface
	if namespace != "" {
		ri = t.Client.dynamic.Resource(gvr).Namespace(namespace)
	} else {
		ri = t.Client.dynamic.Resource(gvr)
	}

	obj, err := ri.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get %s/%s: %v", kind, name, err)}, nil
	}

	// Clean up managed fields for readability
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	data, _ := json.MarshalIndent(obj, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- ListResources Tool ---

type ListResourcesTool struct{ Client *KubeClient }

func (t *ListResourcesTool) Name() string { return "k8s_list_resources" }
func (t *ListResourcesTool) Description() string {
	return "List Kubernetes resources by kind. Works with any resource type including CRDs. " +
		"Use labelSelector or fieldSelector to filter. Returns a summary list."
}
func (t *ListResourcesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion":    {Type: "string", Description: "API version"},
		"kind":          {Type: "string", Description: "Resource kind"},
		"namespace":     {Type: "string", Description: "Namespace (empty for all namespaces)", Default: ""},
		"labelSelector": {Type: "string", Description: "Label selector (e.g. 'app=nginx')", Default: ""},
		"fieldSelector": {Type: "string", Description: "Field selector (e.g. 'status.phase=Running')", Default: ""},
		"limit":         {Type: "integer", Description: "Max items to return", Default: 100},
	}, []string{"apiVersion", "kind"})
}
func (t *ListResourcesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	apiVersion, _ := tools.GetString(args, "apiVersion")
	kind, _ := tools.GetString(args, "kind")
	namespace := tools.GetStringDefault(args, "namespace", "")
	labelSelector := tools.GetStringDefault(args, "labelSelector", "")
	fieldSelector := tools.GetStringDefault(args, "fieldSelector", "")
	limit := tools.GetIntDefault(args, "limit", 100)

	gvr, err := t.Client.parseGroupVersionResource(apiVersion)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}
	gvr.Resource = kindToResource(kind)

	listOpts := metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
		Limit:         int64(limit),
	}

	var list *unstructured.UnstructuredList
	if namespace != "" {
		list, err = t.Client.dynamic.Resource(gvr).Namespace(namespace).List(ctx, listOpts)
	} else {
		list, err = t.Client.dynamic.Resource(gvr).List(ctx, listOpts)
	}
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to list %s: %v", kind, err)}, nil
	}

	// Build summary
	type itemSummary struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
		Status    string `json:"status,omitempty"`
		Created   string `json:"created,omitempty"`
	}
	summaries := make([]itemSummary, 0, len(list.Items))
	for _, item := range list.Items {
		s := itemSummary{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Created:   item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		}
		// Try to get status phase if available
		if phase, ok, _ := unstructured.NestedString(item.Object, "status", "phase"); ok {
			s.Status = phase
		}
		summaries = append(summaries, s)
	}

	result := map[string]any{
		"count": len(summaries),
		"items": summaries,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- DescribeResource Tool ---

type DescribeResourceTool struct{ Client *KubeClient }

func (t *DescribeResourceTool) Name() string { return "k8s_describe_resource" }
func (t *DescribeResourceTool) Description() string {
	return "Get detailed information about a resource including events, conditions, and related resources. " +
		"Similar to 'kubectl describe'. Works with any resource type."
}
func (t *DescribeResourceTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"apiVersion": {Type: "string", Description: "API version"},
		"kind":       {Type: "string", Description: "Resource kind"},
		"name":       {Type: "string", Description: "Resource name"},
		"namespace":  {Type: "string", Description: "Namespace", Default: ""},
	}, []string{"apiVersion", "kind", "name"})
}
func (t *DescribeResourceTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	apiVersion, _ := tools.GetString(args, "apiVersion")
	kind, _ := tools.GetString(args, "kind")
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")

	gvr, err := t.Client.parseGroupVersionResource(apiVersion)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}
	gvr.Resource = kindToResource(kind)

	var ri dynamic.ResourceInterface
	if namespace != "" {
		ri = t.Client.dynamic.Resource(gvr).Namespace(namespace)
	} else {
		ri = t.Client.dynamic.Resource(gvr)
	}

	obj, err := ri.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get %s/%s: %v", kind, name, err)}, nil
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	// Get related events
	eventsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	var events []corev1.Event
	if namespace != "" {
		eventList, err := t.Client.dynamic.Resource(eventsGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", name, kind),
			Limit:         20,
		})
		if err == nil && len(eventList.Items) > 0 {
			for _, e := range eventList.Items {
				var event corev1.Event
				data, _ := e.MarshalJSON()
				if err := json.Unmarshal(data, &event); err == nil {
					events = append(events, event)
				}
			}
		}
	}

	result := map[string]any{
		"resource": obj.Object,
		"events":   events,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- GetPodLogs Tool ---

type GetPodLogsTool struct{ Client *KubeClient }

func (t *GetPodLogsTool) Name() string { return "k8s_get_logs" }
func (t *GetPodLogsTool) Description() string {
	return "Get logs from a pod. Can specify container and number of lines."
}
func (t *GetPodLogsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"name":      {Type: "string", Description: "Pod name"},
		"namespace": {Type: "string", Description: "Namespace", Default: "default"},
		"container": {Type: "string", Description: "Container name (if pod has multiple)", Default: ""},
		"tailLines": {Type: "integer", Description: "Number of lines to return from the end", Default: 100},
		"previous":  {Type: "boolean", Description: "Get previous container logs", Default: false},
	}, []string{"name"})
}
func (t *GetPodLogsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	container := tools.GetStringDefault(args, "container", "")
	tailLines := tools.GetIntDefault(args, "tailLines", 100)
	previous := tools.GetBool(args, "previous")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	opts := &corev1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}
	if tailLines > 0 {
		opts.TailLines = ptr(int64(tailLines))
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(name, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to get logs: %v", err)}, nil
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- ListAPIResources Tool (discover available resource types) ---

type ListAPIResourcesTool struct{ Client *KubeClient }

func (t *ListAPIResourcesTool) Name() string { return "k8s_list_api_resources" }
func (t *ListAPIResourcesTool) Description() string {
	return "List all available Kubernetes API resource types in the cluster, including CRDs. " +
		"Useful to discover what resource types exist before querying them."
}
func (t *ListAPIResourcesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"group": {Type: "string", Description: "Filter by API group (empty for all)", Default: ""},
	}, []string{})
}
func (t *ListAPIResourcesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	groups, err := clientset.Discovery().ServerPreferredResources()
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to discover resources: %v", err)}, nil
	}

	type apiResourceInfo struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		APIVersion string `json:"apiVersion"`
		Namespaced bool   `json:"namespaced"`
	}

	resources := make([]apiResourceInfo, 0)
	for _, list := range groups {
		for _, r := range list.APIResources {
			resources = append(resources, apiResourceInfo{
				Name:       r.Name,
				Kind:       r.Kind,
				APIVersion: list.GroupVersion,
				Namespaced: r.Namespaced,
			})
		}
	}

	result := map[string]any{
		"count":     len(resources),
		"resources": resources,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- GetNodes Tool ---

type GetNodesTool struct{ Client *KubeClient }

func (t *GetNodesTool) Name() string { return "k8s_get_nodes" }
func (t *GetNodesTool) Description() string {
	return "Get Kubernetes node information including status, capacity, allocatable resources, and conditions."
}
func (t *GetNodesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"name": {Type: "string", Description: "Specific node name (empty for all nodes)", Default: ""},
	}, []string{})
}
func (t *GetNodesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	name := tools.GetStringDefault(args, "name", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	if name != "" {
		node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		data, _ := json.MarshalIndent(node, "", "  ")
		return &tools.ToolResult{Success: true, Output: string(data)}, nil
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type nodeSummary struct {
		Name       string            `json:"name"`
		Status     string            `json:"status"`
		Role       string            `json:"role"`
		Version    string            `json:"version"`
		OS         string            `json:"os"`
		Arch       string            `json:"arch"`
		CPU        string            `json:"cpu"`
		Memory     string            `json:"memory"`
		Conditions map[string]string `json:"conditions"`
	}

	summaries := make([]nodeSummary, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		ns := nodeSummary{
			Name:    n.Name,
			Status:  "Ready",
			Version: n.Status.NodeInfo.KubeletVersion,
			OS:      n.Status.NodeInfo.OperatingSystem,
			Arch:    n.Status.NodeInfo.Architecture,
		}
		ns.CPU = n.Status.Allocatable.Cpu().String()
		ns.Memory = n.Status.Allocatable.Memory().String()
		ns.Conditions = make(map[string]string)
		for _, c := range n.Status.Conditions {
			ns.Conditions[string(c.Type)] = string(c.Status)
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionFalse {
				ns.Status = "NotReady"
			}
		}
		for k, v := range n.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
				if ns.Role != "" {
					ns.Role += ","
				}
				ns.Role += role
				_ = v
			}
		}
		if ns.Role == "" {
			ns.Role = "worker"
		}
		summaries = append(summaries, ns)
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(summaries), "nodes": summaries}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- Helper ---

func ptr[T any](v T) *T { return &v }
