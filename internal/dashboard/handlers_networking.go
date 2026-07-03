package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// SvcHealthStatus describes the health of a Service from a networking perspective.
type SvcHealthStatus string

const (
	SvcHealthHealthy       SvcHealthStatus = "healthy"       // has ready endpoints
	SvcHealthDegraded      SvcHealthStatus = "degraded"      // some endpoints not ready
	SvcHealthNoEndpoints   SvcHealthStatus = "no-endpoints"  // zero endpoints
	SvcHealthMisconfigured SvcHealthStatus = "misconfigured" // selector mismatch
	SvcHealthExternal      SvcHealthStatus = "external"      // ExternalName or LoadBalancer (informational)
)

// ServiceHealth summarizes the networking health of a single Service.
type ServiceHealth struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Type              string            `json:"type"` // ClusterIP, NodePort, LoadBalancer, ExternalName
	Status            SvcHealthStatus   `json:"status"`
	Selector          map[string]string `json:"selector,omitempty"`
	Ports             []SvcPortInfo     `json:"ports,omitempty"`
	EndpointCount     int               `json:"endpointCount"`
	ReadyEndpoints    int               `json:"readyEndpoints"`
	NotReadyEndpoints int               `json:"notReadyEndpoints"`
	MatchingPods      int               `json:"matchingPods"` // pods matching selector
	HealthyPods       int               `json:"healthyPods"`  // matching pods that are Running+Ready
	ClusterIP         string            `json:"clusterIP,omitempty"`
	ExternalIPs       []string          `json:"externalIPs,omitempty"`
	LoadBalancerIP    string            `json:"loadBalancerIP,omitempty"`
	Issues            []string          `json:"issues,omitempty"`
}

// SvcPortInfo is a compact view of a service port.
type SvcPortInfo struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

// IngressHealth summarizes the health of a single Ingress.
type IngressHealth struct {
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	Healthy    bool             `json:"healthy"`
	Hosts      []string         `json:"hosts"`
	BackendSvc []IngressBackend `json:"backends"`
	Issues     []string         `json:"issues,omitempty"`
}

// IngressBackend tracks a single ingress backend.
type IngressBackend struct {
	ServiceName  string `json:"serviceName"`
	ServicePort  string `json:"servicePort"`
	Exists       bool   `json:"exists"`
	HasEndpoints bool   `json:"hasEndpoints"`
}

// NetHealthResult is the full scan output.
type NetHealthResult struct {
	ScannedAt time.Time        `json:"scannedAt"`
	Summary   NetHealthSummary `json:"summary"`
	Services  []ServiceHealth  `json:"services"`
	Ingresses []IngressHealth  `json:"ingresses,omitempty"`
}

// NetHealthSummary aggregates networking health statistics.
type NetHealthSummary struct {
	TotalServices    int            `json:"totalServices"`
	ByStatus         map[string]int `json:"byStatus"`
	TotalIngresses   int            `json:"totalIngresses"`
	UnhealthyIngress int            `json:"unhealthyIngress"`
	NoEndpointSvcs   int            `json:"noEndpointServices"`
}

// handleNetworkingHealth scans all Services and Ingresses for networking health.
// GET /api/networking/health
func (s *Server) handleNetworkingHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	// List all services
	svcList, err := rc.clientset.CoreV1().Services(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List all endpoint slices
	epSliceList, err := rc.clientset.DiscoveryV1().EndpointSlices(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List all pods (for selector matching analysis)
	podList, err := rc.clientset.CoreV1().Pods(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List ingresses
	ingList, err := rc.clientset.NetworkingV1().Ingresses(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		// Ingresses might not be installed in all clusters; continue without them
		ingList = &networkingv1.IngressList{}
	}

	// Build endpoint slice index by service
	endpointsBySvc := make(map[string]int) // total addresses
	readyBySvc := make(map[string]int)     // ready addresses
	notReadyBySvc := make(map[string]int)  // not-ready addresses

	for i := range epSliceList.Items {
		eps := &epSliceList.Items[i]
		svcName := eps.Labels[discoveryv1.LabelServiceName]
		if svcName == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s", eps.Namespace, svcName)
		for _, ep := range eps.Endpoints {
			endpointsBySvc[key]++
			isReady := true
			if ep.Conditions.Ready != nil {
				isReady = *ep.Conditions.Ready
			}
			if isReady {
				readyBySvc[key]++
			} else {
				notReadyBySvc[key]++
			}
		}
	}

	// Build pod index for selector matching
	podsByNs := make(map[string][]*corev1.Pod)
	for i := range podList.Items {
		pod := &podList.Items[i]
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod)
	}

	// Build service name set for ingress validation
	svcKeySet := make(map[string]bool)
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		svcKeySet[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
	}

	var services []ServiceHealth
	summary := NetHealthSummary{ByStatus: make(map[string]int)}

	for i := range svcList.Items {
		svc := &svcList.Items[i]
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

		health := analyzeServiceHealth(svc, endpointsBySvc[key], readyBySvc[key], notReadyBySvc[key], podsByNs[svc.Namespace])

		// Skip system services with no selector and ExternalName
		if len(services) < 500 || health.Status != SvcHealthHealthy {
			services = append(services, health)
		}

		summary.TotalServices++
		summary.ByStatus[string(health.Status)]++
		if health.Status == SvcHealthNoEndpoints {
			summary.NoEndpointSvcs++
		}
	}

	// Sort: problematic first
	sort.Slice(services, func(i, j int) bool {
		rankI := svcStatusRank(services[i].Status)
		rankJ := svcStatusRank(services[j].Status)
		if rankI != rankJ {
			return rankI < rankJ
		}
		return services[i].Namespace+"/"+services[i].Name < services[j].Namespace+"/"+services[j].Name
	})

	// Analyze ingresses
	var ingresses []IngressHealth
	for i := range ingList.Items {
		ing := &ingList.Items[i]
		ih := analyzeIngressHealth(ing, svcKeySet, endpointsBySvc)
		ingresses = append(ingresses, ih)
		summary.TotalIngresses++
		if !ih.Healthy {
			summary.UnhealthyIngress++
		}
	}

	// Sort ingresses: unhealthy first
	sort.Slice(ingresses, func(i, j int) bool {
		if ingresses[i].Healthy == ingresses[j].Healthy {
			return ingresses[i].Namespace+"/"+ingresses[i].Name < ingresses[j].Namespace+"/"+ingresses[j].Name
		}
		return !ingresses[i].Healthy
	})

	writeJSON(w, NetHealthResult{
		ScannedAt: time.Now(),
		Summary:   summary,
		Services:  services,
		Ingresses: ingresses,
	})
}

// analyzeServiceHealth evaluates the networking health of a single Service.
func analyzeServiceHealth(
	svc *corev1.Service,
	totalEndpoints, readyEndpoints, notReadyEndpoints int,
	pods []*corev1.Pod,
) ServiceHealth {
	h := ServiceHealth{
		Name:              svc.Name,
		Namespace:         svc.Namespace,
		Type:              string(svc.Spec.Type),
		Selector:          svc.Spec.Selector,
		EndpointCount:     totalEndpoints,
		ReadyEndpoints:    readyEndpoints,
		NotReadyEndpoints: notReadyEndpoints,
		ClusterIP:         svc.Spec.ClusterIP,
	}

	// Extract port info
	for _, p := range svc.Spec.Ports {
		pi := SvcPortInfo{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			Protocol:   string(p.Protocol),
			NodePort:   p.NodePort,
		}
		h.Ports = append(h.Ports, pi)
	}

	// ExternalName services are informational only
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		h.Status = SvcHealthExternal
		h.Issues = append(h.Issues, "ExternalName service — traffic is forwarded via DNS CNAME")
		if svc.Spec.ExternalName != "" {
			h.ExternalIPs = []string{svc.Spec.ExternalName}
		}
		return h
	}

	// Services without selectors (e.g., manually managed endpoints, headless to external)
	if len(svc.Spec.Selector) == 0 {
		if totalEndpoints > 0 {
			if notReadyEndpoints > 0 && readyEndpoints == 0 {
				h.Status = SvcHealthNoEndpoints
				h.Issues = append(h.Issues, fmt.Sprintf("Service has no selector and all %d endpoints are not ready", totalEndpoints))
			} else if notReadyEndpoints > 0 {
				h.Status = SvcHealthDegraded
				h.Issues = append(h.Issues, fmt.Sprintf("Service has no selector, %d/%d endpoints not ready", notReadyEndpoints, totalEndpoints))
			} else {
				h.Status = SvcHealthHealthy
			}
		} else {
			// No selector and no endpoints — could be a LoadBalancer waiting for cloud provider
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				h.Status = SvcHealthExternal
				if len(svc.Status.LoadBalancer.Ingress) > 0 {
					for _, lb := range svc.Status.LoadBalancer.Ingress {
						if lb.IP != "" {
							h.LoadBalancerIP = lb.IP
						}
						if lb.Hostname != "" {
							h.ExternalIPs = append(h.ExternalIPs, lb.Hostname)
						}
					}
					h.Issues = append(h.Issues, "LoadBalancer service with external ingress configured")
				} else {
					h.Issues = append(h.Issues, "LoadBalancer service — waiting for external IP assignment")
				}
			} else {
				h.Status = SvcHealthHealthy
				h.Issues = append(h.Issues, "Service has no selector and no endpoints — manually managed or headless")
			}
		}
		return h
	}

	// Services WITH selectors: check pod matching
	selector := labels.Set(svc.Spec.Selector).AsSelectorPreValidated()
	matchingPods := 0
	healthyPods := 0
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			matchingPods++
			if pod.Status.Phase == corev1.PodRunning && isPodReady(pod) {
				healthyPods++
			}
		}
	}
	h.MatchingPods = matchingPods
	h.HealthyPods = healthyPods

	// Check LoadBalancer status
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			for _, lb := range svc.Status.LoadBalancer.Ingress {
				if lb.IP != "" {
					h.LoadBalancerIP = lb.IP
				}
				if lb.Hostname != "" {
					h.ExternalIPs = append(h.ExternalIPs, lb.Hostname)
				}
			}
		} else {
			h.Issues = append(h.Issues, "LoadBalancer service — no external IP assigned yet")
		}
	}

	// Determine health based on endpoints and pods
	if readyEndpoints > 0 && notReadyEndpoints == 0 {
		h.Status = SvcHealthHealthy
	} else if readyEndpoints > 0 && notReadyEndpoints > 0 {
		h.Status = SvcHealthDegraded
		h.Issues = append(h.Issues, fmt.Sprintf("%d of %d endpoints not ready", notReadyEndpoints, totalEndpoints))
	} else if totalEndpoints > 0 && readyEndpoints == 0 {
		// Has endpoints but none ready
		h.Status = SvcHealthNoEndpoints
		h.Issues = append(h.Issues, fmt.Sprintf("All %d endpoints are not ready", totalEndpoints))
	} else {
		// No endpoints at all
		if matchingPods == 0 {
			h.Status = SvcHealthMisconfigured
			h.Issues = append(h.Issues, "No pods match the service selector — check label mismatch")
		} else if healthyPods == 0 {
			h.Status = SvcHealthNoEndpoints
			h.Issues = append(h.Issues, fmt.Sprintf("%d pod(s) match selector but none are Running+Ready", matchingPods))
		} else {
			h.Status = SvcHealthMisconfigured
			h.Issues = append(h.Issues, fmt.Sprintf("%d healthy pods match selector but no endpoints created — check targetPort mismatch or endpoint controller", healthyPods))
		}
	}

	return h
}

// analyzeIngressHealth checks if ingress backends point to existing services.
func analyzeIngressHealth(
	ing *networkingv1.Ingress,
	svcKeySet map[string]bool,
	endpointsBySvc map[string]int,
) IngressHealth {
	h := IngressHealth{
		Name:      ing.Name,
		Namespace: ing.Namespace,
		Healthy:   true,
	}

	// Extract hosts
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			h.Hosts = append(h.Hosts, rule.Host)
		}
	}
	if len(h.Hosts) == 0 && ing.Spec.DefaultBackend != nil {
		h.Hosts = append(h.Hosts, "*")
	}

	// Check default backend
	if ing.Spec.DefaultBackend != nil {
		if be := ing.Spec.DefaultBackend.Service; be != nil {
			backend := IngressBackend{
				ServiceName: be.Name,
			}
			if be.Port.Number > 0 {
				backend.ServicePort = fmt.Sprintf("%d", be.Port.Number)
			} else {
				backend.ServicePort = be.Port.Name
			}
			svcKey := fmt.Sprintf("%s/%s", ing.Namespace, be.Name)
			backend.Exists = svcKeySet[svcKey]
			backend.HasEndpoints = endpointsBySvc[svcKey] > 0
			if !backend.Exists {
				h.Healthy = false
				h.Issues = append(h.Issues, fmt.Sprintf("Default backend service '%s' does not exist", be.Name))
			} else if !backend.HasEndpoints {
				h.Healthy = false
				h.Issues = append(h.Issues, fmt.Sprintf("Default backend service '%s' has no endpoints", be.Name))
			}
			h.BackendSvc = append(h.BackendSvc, backend)
		}
	}

	// Check rule backends
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			be := path.Backend.Service
			backend := IngressBackend{
				ServiceName: be.Name,
			}
			if be.Port.Number > 0 {
				backend.ServicePort = fmt.Sprintf("%d", be.Port.Number)
			} else {
				backend.ServicePort = be.Port.Name
			}
			svcKey := fmt.Sprintf("%s/%s", ing.Namespace, be.Name)
			backend.Exists = svcKeySet[svcKey]
			backend.HasEndpoints = endpointsBySvc[svcKey] > 0
			if !backend.Exists {
				h.Healthy = false
				h.Issues = append(h.Issues, fmt.Sprintf("Backend service '%s' (host: %s, path: %s) does not exist", be.Name, rule.Host, path.Path))
			} else if !backend.HasEndpoints {
				h.Healthy = false
				h.Issues = append(h.Issues, fmt.Sprintf("Backend service '%s' (host: %s, path: %s) has no endpoints", be.Name, rule.Host, path.Path))
			}
			h.BackendSvc = append(h.BackendSvc, backend)
		}
	}

	if len(h.BackendSvc) == 0 && len(h.Hosts) > 0 {
		h.Healthy = false
		h.Issues = append(h.Issues, "Ingress has rules but no service backends defined")
	}

	return h
}

// isPodReady checks if a pod's readiness gate conditions are all true.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// svcStatusRank returns sort priority (lower = more problematic).
func svcStatusRank(status SvcHealthStatus) int {
	switch status {
	case SvcHealthMisconfigured:
		return 0
	case SvcHealthNoEndpoints:
		return 1
	case SvcHealthDegraded:
		return 2
	case SvcHealthExternal:
		return 3
	case SvcHealthHealthy:
		return 4
	default:
		return 5
	}
}

// formatSvcKey builds a human-readable service identifier.
func formatSvcKey(ns, name string) string {
	return fmt.Sprintf("%s/%s", ns, name)
}

// hasMatchingSelector checks if a pod matches the service selector.
func hasMatchingSelector(selector map[string]string, podLabels map[string]string) bool {
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

// joinIssues concatenates issue strings with semicolons.
func joinIssues(issues []string) string {
	return strings.Join(issues, "; ")
}
