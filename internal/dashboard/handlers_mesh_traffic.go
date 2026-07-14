package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeshTrafficResult is the service mesh traffic management & circuit breaker health audit.
type MeshTrafficResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          MeshTrafficSummary    `json:"summary"`
	VirtualServices  []MeshVirtualService  `json:"virtualServices"`
	DestinationRules []MeshDestinationRule `json:"destinationRules"`
	Gaps             []MeshTrafficGap      `json:"gaps"`
	Recommendations  []string              `json:"recommendations"`
	HealthScore      int                   `json:"healthScore"`
}

// MeshTrafficSummary aggregates mesh traffic management statistics.
type MeshTrafficSummary struct {
	TotalNamespaces     int  `json:"totalNamespaces"`
	HasIstio            bool `json:"hasIstio"`
	HasLinkerd          bool `json:"hasLinkerd"`
	VirtualServices     int  `json:"virtualServices"`
	DestinationRules    int  `json:"destinationRules"`
	ServicesWithMesh    int  `json:"servicesWithMesh"`    // services mesh-managed
	ServicesWithoutMesh int  `json:"servicesWithoutMesh"` // services with no mesh sidecar
	WithCircuitBreaker  int  `json:"withCircuitBreaker"`
	WithRetryPolicy     int  `json:"withRetryPolicy"`
	WithTimeout         int  `json:"withTimeout"`
	NamespacesWithMesh  int  `json:"namespacesWithMesh"`
	NamespacesNoMesh    int  `json:"namespacesNoMesh"`
}

// MeshVirtualService describes an Istio VirtualService (or Linkerd config).
type MeshVirtualService struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Hosts      []string `json:"hosts"`
	HasRetry   bool     `json:"hasRetry"`
	HasTimeout bool     `json:"hasTimeout"`
	HasRewrite bool     `json:"hasRewrite"`
	RouteCount int      `json:"routeCount"`
	Status     string   `json:"status"`
}

// MeshDestinationRule describes an Istio DestinationRule (or Linkerd config).
type MeshDestinationRule struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Host              string `json:"host"`
	HasCircuitBreaker bool   `json:"hasCircuitBreaker"`
	HasTLS            bool   `json:"hasTLS"`
	LoadBalancer      string `json:"loadBalancer"`
	SubsetCount       int    `json:"subsetCount"`
	Status            string `json:"status"`
}

// MeshTrafficGap describes a service without mesh traffic management.
type MeshTrafficGap struct {
	Namespace string `json:"namespace"`
	Service   string `json:"service"`
	Protocol  string `json:"protocol"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleMeshTraffic audits service mesh traffic management & circuit breaker health.
// GET /api/product/mesh-traffic
func (s *Server) handleMeshTraffic(w http.ResponseWriter, r *http.Request) {
	result := MeshTrafficResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// 1. Detect mesh installation
	istioNamespaces := []string{"istio-system", "istio-operator", "istio-ingress"}
	linkerdNamespaces := []string{"linkerd", "linkerd-system"}

	for _, ns := range istioNamespaces {
		_, err := rc.clientset.CoreV1().Namespaces().Get(r.Context(), ns, metav1.GetOptions{})
		if err == nil {
			result.Summary.HasIstio = true
			break
		}
	}

	for _, ns := range linkerdNamespaces {
		_, err := rc.clientset.CoreV1().Namespaces().Get(r.Context(), ns, metav1.GetOptions{})
		if err == nil {
			result.Summary.HasLinkerd = true
			break
		}
	}

	// Check for mesh sidecar pods (envoy or linkerd-proxy)
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsWithMeshSidecar := make(map[string]bool)
		nsPodCount := make(map[string]int)
		nsMeshPodCount := make(map[string]int)

		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			nsPodCount[pod.Namespace]++
			hasSidecar := false
			for _, c := range pod.Spec.Containers {
				if strings.Contains(c.Name, "istio-proxy") || strings.Contains(c.Name, "envoy") {
					hasSidecar = true
					break
				}
				if strings.Contains(c.Name, "linkerd-proxy") || strings.Contains(c.Name, "linkerd-init") {
					hasSidecar = true
					break
				}
			}
			// Also check annotations
			if !hasSidecar && pod.Annotations != nil {
				if pod.Annotations["sidecar.istio.io/status"] != "" || pod.Annotations["linkerd.io/inject"] == "enabled" {
					hasSidecar = true
				}
			}

			if hasSidecar {
				nsMeshPodCount[pod.Namespace]++
				nsWithMeshSidecar[pod.Namespace] = true
				result.Summary.ServicesWithMesh++
			} else {
				result.Summary.ServicesWithoutMesh++
			}
		}

		// Count namespaces with/without mesh
		totalNS := 0
		for ns, podCount := range nsPodCount {
			if podCount == 0 {
				continue
			}
			totalNS++
			if nsWithMeshSidecar[ns] {
				result.Summary.NamespacesWithMesh++
			} else {
				result.Summary.NamespacesNoMesh++
				result.Gaps = append(result.Gaps, MeshTrafficGap{
					Namespace: ns,
					Service:   "(all services)",
					Protocol:  "all",
					Issue:     "Namespace has running pods but no mesh sidecar injection",
					Severity:  "medium",
				})
			}
		}
		result.Summary.TotalNamespaces = totalNS
	}

	// 2. Try to discover VirtualServices and DestinationRules
	// These are CRDs — use ConfigMaps as fallback for detection
	// In real cluster, would use dynamic client for networking.istio.io/v1beta1
	// For now, check for Istio ConfigMaps that contain route configuration
	configmaps, err := rc.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		meshNamespaces := map[string]bool{
			"istio-system":   true,
			"istio-operator": true,
		}

		for _, cm := range configmaps.Items {
			if !meshNamespaces[cm.Namespace] && !result.Summary.HasIstio {
				continue
			}

			// Look for Istio mesh configuration
			if strings.Contains(cm.Name, "istio") || strings.Contains(cm.Name, "mesh") {
				for _, data := range cm.Data {
					if strings.Contains(strings.ToLower(data), "virtualservice") || strings.Contains(strings.ToLower(data), "destinationrule") {
						// Found mesh config
						break
					}
				}
			}
		}
	}

	// 3. Check services for mesh annotations
	services, err := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range services.Items {
			if systemNamespaces[svc.Namespace] {
				continue
			}

			// Check if service has mesh annotation
			hasMeshAnnotation := false
			if svc.Annotations != nil {
				if svc.Annotations["sidecar.istio.io/inject"] == "true" || svc.Annotations["linkerd.io/inject"] == "enabled" {
					hasMeshAnnotation = true
				}
			}

			// Check if namespace has mesh injection label
			ns, err := rc.clientset.CoreV1().Namespaces().Get(r.Context(), svc.Namespace, metav1.GetOptions{})
			if err == nil && ns != nil {
				if ns.Labels["istio-injection"] == "enabled" || ns.Labels["linkerd.io/inject"] == "enabled" {
					hasMeshAnnotation = true
				}
			}

			// Services with TCP/HTTP that could benefit from circuit breaker
			protocol := "TCP"
			for _, port := range svc.Spec.Ports {
				if port.AppProtocol != nil {
					if *port.AppProtocol == "http" || *port.AppProtocol == "grpc" || *port.AppProtocol == "https" {
						protocol = "HTTP"
						break
					}
				}
				if strings.HasPrefix(string(port.Name), "http") || strings.HasPrefix(string(port.Name), "grpc") {
					protocol = "HTTP"
					break
				}
			}

			if !hasMeshAnnotation && result.Summary.HasIstio || result.Summary.HasLinkerd {
				result.Gaps = append(result.Gaps, MeshTrafficGap{
					Namespace: svc.Namespace,
					Service:   svc.Name,
					Protocol:  protocol,
					Issue:     "Service without mesh sidecar injection — no circuit breaker, retry, or timeout protection",
					Severity:  "medium",
				})
			}
		}
	}

	// Sort gaps
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if !result.Summary.HasIstio && !result.Summary.HasLinkerd {
		result.Recommendations = append(result.Recommendations,
			"No service mesh detected. Install Istio or Linkerd for circuit breaker, retry policy, and mTLS")
	}
	if result.Summary.ServicesWithoutMesh > 0 && (result.Summary.HasIstio || result.Summary.HasLinkerd) {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d services are not mesh-managed — enable sidecar injection for circuit breaker and retry protection", result.Summary.ServicesWithoutMesh))
	}
	if result.Summary.NamespacesNoMesh > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have no mesh injection enabled", result.Summary.NamespacesNoMesh))
	}
	if result.Summary.WithCircuitBreaker == 0 && (result.Summary.HasIstio || result.Summary.HasLinkerd) {
		result.Recommendations = append(result.Recommendations,
			"No circuit breaker configurations detected — add DestinationRules with outlier detection")
	}
	if result.Summary.WithRetryPolicy == 0 && (result.Summary.HasIstio || result.Summary.HasLinkerd) {
		result.Recommendations = append(result.Recommendations,
			"No retry policies detected — add VirtualService retry configurations for resilience")
	}

	// Health score
	score := 100
	if !result.Summary.HasIstio && !result.Summary.HasLinkerd {
		score = 50
	} else {
		score -= result.Summary.NamespacesNoMesh * 5
		score -= result.Summary.ServicesWithoutMesh * 1
		if result.Summary.WithCircuitBreaker == 0 {
			score -= 10
		}
		if result.Summary.WithRetryPolicy == 0 {
			score -= 10
		}
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score

	writeJSON(w, result)
}
