package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ggai/k8ops/internal/tools"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// --- GetServicesTool: List services with endpoints status ---

type portInfo struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"`
}

type svcInfo struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace"`
	Type         string     `json:"type"`
	ClusterIP    string     `json:"clusterIP"`
	ExternalIPs  []string   `json:"externalIPs,omitempty"`
	Ports        []portInfo `json:"ports"`
	Selector     string     `json:"selector,omitempty"`
	HasEndpoints bool       `json:"hasEndpoints"`
	EndpointIPs  []string   `json:"endpointIPs,omitempty"`
}

type GetServicesTool struct{ Client *KubeClient }

func (t *GetServicesTool) Name() string { return "k8s_get_services" }
func (t *GetServicesTool) Description() string {
	return "List Kubernetes Services with type, cluster IP, external IP, ports, and endpoint readiness. " +
		"Essential for diagnosing connectivity and load balancer issues."
}
func (t *GetServicesTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
		"name":      {Type: "string", Description: "Specific service name", Default: ""},
	}, []string{})
}
func (t *GetServicesTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")
	name := tools.GetStringDefault(args, "name", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var services []svcInfo

	if name != "" && namespace != "" {
		svc, err := clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		services = append(services, buildSvcInfo(svc))
	} else {
		list, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		for _, s := range list.Items {
			services = append(services, buildSvcInfo(&s))
		}
	}

	// Enrich with endpoint info
	for i := range services {
		eps, err := clientset.CoreV1().Endpoints(services[i].Namespace).Get(ctx, services[i].Name, metav1.GetOptions{})
		if err == nil {
			services[i].HasEndpoints = len(eps.Subsets) > 0
			for _, subset := range eps.Subsets {
				for _, addr := range subset.Addresses {
					services[i].EndpointIPs = append(services[i].EndpointIPs, addr.IP)
				}
			}
		}
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(services), "services": services}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

func buildSvcInfo(s *corev1.Service) svcInfo {
	info := svcInfo{
		Name: s.Name, Namespace: s.Namespace,
		Type: string(s.Spec.Type), ClusterIP: s.Spec.ClusterIP,
		ExternalIPs: s.Spec.ExternalIPs,
	}
	for _, p := range s.Spec.Ports {
		info.Ports = append(info.Ports, portInfo{
			Name: p.Name, Port: p.Port,
			TargetPort: p.TargetPort.String(), Protocol: string(p.Protocol),
		})
	}
	if len(s.Spec.Selector) > 0 {
		var pairs []string
		for k, v := range s.Spec.Selector {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(pairs)
		info.Selector = strings.Join(pairs, ",")
	}
	return info
}

// --- GetConfigMapTool: Read ConfigMap data (safe, values truncated) ---

type GetConfigMapTool struct{ Client *KubeClient }

func (t *GetConfigMapTool) Name() string { return "k8s_get_configmap" }
func (t *GetConfigMapTool) Description() string {
	return "Read a ConfigMap's data. Values are truncated to 1000 chars for safety. " +
		"Useful for checking application configuration."
}
func (t *GetConfigMapTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"name":      {Type: "string", Description: "ConfigMap name"},
		"namespace": {Type: "string", Description: "Namespace", Default: "default"},
		"key":       {Type: "string", Description: "Specific key to read (empty for all)", Default: ""},
	}, []string{"name"})
}
func (t *GetConfigMapTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	name, _ := tools.GetString(args, "name")
	namespace := tools.GetStringDefault(args, "namespace", "default")
	keyFilter := tools.GetStringDefault(args, "key", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	result := map[string]any{
		"name":      cm.Name,
		"namespace": cm.Namespace,
	}
	data := make(map[string]string)
	for k, v := range cm.Data {
		if keyFilter != "" && k != keyFilter {
			continue
		}
		if len(v) > 1000 {
			v = v[:1000] + "... (truncated)"
		}
		data[k] = v
	}
	result["data"] = data
	result["binaryKeyCount"] = len(cm.BinaryData)

	out, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(out)}, nil
}

// --- GetIngressTool: List ingress rules and backends ---

type GetIngressTool struct{ Client *KubeClient }

func (t *GetIngressTool) Name() string { return "k8s_get_ingress" }
func (t *GetIngressTool) Description() string {
	return "List Kubernetes Ingress resources with rules, backends, TLS, and load balancer status."
}
func (t *GetIngressTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
	}, []string{})
}
func (t *GetIngressTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	list, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type ingressInfo struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Class      string `json:"class"`
		Hosts      []string `json:"hosts"`
		Backends   []string `json:"backends"`
		TLSEnabled bool    `json:"tlsEnabled"`
		IPAddress  string  `json:"ipAddress,omitempty"`
	}

	results := make([]ingressInfo, 0, len(list.Items))
	for _, ing := range list.Items {
		info := ingressInfo{
			Name: ing.Name, Namespace: ing.Namespace,
		}
		if ing.Spec.IngressClassName != nil {
			info.Class = *ing.Spec.IngressClassName
		}
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				info.Hosts = append(info.Hosts, rule.Host)
			}
			for _, path := range rule.HTTP.Paths {
				backend := fmt.Sprintf("%s:%s", path.Backend.Service.Name, path.Backend.Service.Port.Name)
				if path.Backend.Service.Port.Number > 0 {
					backend = fmt.Sprintf("%s:%d", path.Backend.Service.Name, path.Backend.Service.Port.Number)
				}
				info.Backends = append(info.Backends, backend)
			}
		}
		info.TLSEnabled = len(ing.Spec.TLS) > 0
		for _, lb := range ing.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				info.IPAddress = lb.IP
			} else if lb.Hostname != "" {
				info.IPAddress = lb.Hostname
			}
		}
		results = append(results, info)
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "ingresses": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- GetNetworkPolicyTool: List network policies ---

type GetNetworkPolicyTool struct{ Client *KubeClient }

func (t *GetNetworkPolicyTool) Name() string { return "k8s_get_network_policies" }
func (t *GetNetworkPolicyTool) Description() string {
	return "List Kubernetes NetworkPolicies showing pod selectors, ingress/egress rules. " +
		"Useful for diagnosing connectivity issues caused by network policies."
}
func (t *GetNetworkPolicyTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace (empty for all)", Default: ""},
	}, []string{})
}
func (t *GetNetworkPolicyTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	list, err := clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	type netpolInfo struct {
		Name             string `json:"name"`
		Namespace        string `json:"namespace"`
		PodSelector      string `json:"podSelector"`
		PolicyTypes      []string `json:"policyTypes"`
		IngressRules     int     `json:"ingressRules"`
		EgressRules      int     `json:"egressRules"`
	}

	results := make([]netpolInfo, 0, len(list.Items))
	for _, np := range list.Items {
		info := netpolInfo{
			Name: np.Name, Namespace: np.Namespace,
		}
		if len(np.Spec.PodSelector.MatchLabels) > 0 {
			var pairs []string
			for k, v := range np.Spec.PodSelector.MatchLabels {
				pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(pairs)
			info.PodSelector = strings.Join(pairs, ",")
		} else {
			info.PodSelector = "<all pods>"
		}
		for _, pt := range np.Spec.PolicyTypes {
			info.PolicyTypes = append(info.PolicyTypes, string(pt))
		}
		info.IngressRules = len(np.Spec.Ingress)
		info.EgressRules = len(np.Spec.Egress)
		results = append(results, info)
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "networkPolicies": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// --- GetPodStatusTool: Detailed pod status with container states ---

type GetPodStatusTool struct{ Client *KubeClient }

func (t *GetPodStatusTool) Name() string { return "k8s_pod_status" }
func (t *GetPodStatusTool) Description() string {
	return "Get detailed pod status: phase, container states (running/waiting/terminated), " +
		"restart counts, conditions, QoS class, and node assignment. " +
		"More focused than describe_resource for troubleshooting pod issues."
}
func (t *GetPodStatusTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{
		"namespace": {Type: "string", Description: "Namespace", Default: ""},
		"labelSelector": {Type: "string", Description: "Label selector", Default: ""},
		"failed":    {Type: "boolean", Description: "Only show failed/pending pods", Default: false},
	}, []string{})
}
func (t *GetPodStatusTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	namespace := tools.GetStringDefault(args, "namespace", "")
	labelSelector := tools.GetStringDefault(args, "labelSelector", "")
	failedOnly := tools.GetBool(args, "failed")

	clientset, err := kubernetes.NewForConfig(t.Client.config)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	listOpts := metav1.ListOptions{LabelSelector: labelSelector}
	var pods []corev1.Pod
	if namespace != "" {
		l, err := clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		pods = l.Items
	} else {
		l, err := clientset.CoreV1().Pods("").List(ctx, listOpts)
		if err != nil {
			return &tools.ToolResult{Success: false, Error: err.Error()}, nil
		}
		pods = l.Items
	}

	type containerStatus struct {
		Name     string `json:"name"`
		State    string `json:"state"`
		Reason   string `json:"reason,omitempty"`
		Message  string `json:"message,omitempty"`
		Restarts int32  `json:"restarts"`
		Ready    bool   `json:"ready"`
		Image    string `json:"image"`
	}
	type podInfo struct {
		Name       string            `json:"name"`
		Namespace  string            `json:"namespace"`
		Phase      string            `json:"phase"`
		Node       string            `json:"node"`
		IP         string            `json:"podIP"`
		QoS        string            `json:"qosClass"`
		Age        string            `json:"age"`
		Restarts   int32             `json:"totalRestarts"`
		Containers []containerStatus `json:"containers"`
		Conditions map[string]string `json:"conditions"`
	}

	results := make([]podInfo, 0)
	for _, p := range pods {
		if failedOnly && p.Status.Phase != corev1.PodFailed && p.Status.Phase != corev1.PodPending {
			continue
		}
		info := podInfo{
			Name: p.Name, Namespace: p.Namespace,
			Phase: string(p.Status.Phase),
			Node:  p.Spec.NodeName, IP: p.Status.PodIP,
			QoS:    string(p.Status.QOSClass),
			Conditions: make(map[string]string),
		}
		info.Age = formatAge(p.CreationTimestamp.Time)
		for _, c := range p.Status.ContainerStatuses {
			cs := containerStatus{
				Name: c.Name, Restarts: c.RestartCount,
				Ready: c.Ready, Image: c.Image,
			}
			info.Restarts += c.RestartCount
			if c.State.Running != nil {
				cs.State = "running"
			} else if c.State.Waiting != nil {
				cs.State = "waiting"
				cs.Reason = c.State.Waiting.Reason
				cs.Message = c.State.Waiting.Message
			} else if c.State.Terminated != nil {
				cs.State = "terminated"
				cs.Reason = c.State.Terminated.Reason
				cs.Message = c.State.Terminated.Message
			}
			info.Containers = append(info.Containers, cs)
		}
		for _, cond := range p.Status.Conditions {
			info.Conditions[string(cond.Type)] = string(cond.Status)
		}
		results = append(results, info)
	}

	// Sort: failed/pending first, then by restart count
	sort.Slice(results, func(i, j int) bool {
		if results[i].Phase != results[j].Phase {
			order := map[string]int{"Failed": 0, "Pending": 1, "Running": 2, "Succeeded": 3, "Unknown": 4}
			return order[results[i].Phase] < order[results[j].Phase]
		}
		return results[i].Restarts > results[j].Restarts
	})

	data, _ := json.MarshalIndent(map[string]any{"count": len(results), "pods": results}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

// suppress unused import
var _ = networkingv1.SchemeGroupVersion
