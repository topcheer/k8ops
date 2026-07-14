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

// EastWestTrafficResult is the east-west traffic & service-to-service connectivity audit.
type EastWestTrafficResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         EastWestSummary       `json:"summary"`
	ByNamespace     []EastWestNSStat      `json:"byNamespace"`
	ExposedServices []ExposedServiceEntry `json:"exposedServices"`
	InternalRoutes  []InternalRouteEntry  `json:"internalRoutes"`
	Risks           []EastWestRisk        `json:"risks"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// EastWestSummary aggregates east-west traffic analysis.
type EastWestSummary struct {
	TotalServices        int `json:"totalServices"`
	ClusterIPServices    int `json:"clusterIPServices"`    // internal only
	NodePortServices     int `json:"nodePortServices"`     // exposed via NodePort
	LoadBalancerSvcs     int `json:"loadBalancerSvcs"`     // exposed via LB
	ExternalNameSvcs     int `json:"externalNameSvcs"`     // external DNS alias
	HeadlessServices     int `json:"headlessServices"`     // no ClusterIP
	WithNetworkPolicy    int `json:"withNetworkPolicy"`    // services with NP protection
	WithoutNetworkPolicy int `json:"withoutNetworkPolicy"` // services without NP
	CrossNSAccess        int `json:"crossNSAccess"`        // services reachable cross-namespace
	PubliclyExposed      int `json:"publiclyExposed"`      // services exposed to internet
	InternalOnly         int `json:"internalOnly"`         // services internal only
	HasMeshSidecar       int `json:"hasMeshSidecar"`       // services with mesh sidecar
	WithoutMesh          int `json:"withoutMesh"`          // services without mesh sidecar
}

// EastWestNSStat per-namespace east-west traffic stats.
type EastWestNSStat struct {
	Namespace     string `json:"namespace"`
	TotalServices int    `json:"totalServices"`
	Exposed       int    `json:"exposed"`   // NodePort + LB
	Internal      int    `json:"internal"`  // ClusterIP only
	WithNP        int    `json:"withNP"`    // has network policy
	WithoutNP     int    `json:"withoutNP"` // no network policy
	WithMesh      int    `json:"withMesh"`  // pods have sidecar
	RiskLevel     string `json:"riskLevel"` // low, medium, high, critical
}

// ExposedServiceEntry describes a service that is exposed.
type ExposedServiceEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`      // NodePort, LoadBalancer, ExternalName
	Ports     string `json:"ports"`     // port summary
	HasNP     bool   `json:"hasNP"`     // has network policy
	HasMesh   bool   `json:"hasMesh"`   // pods have sidecar
	RiskLevel string `json:"riskLevel"` // low, medium, high, critical
}

// InternalRouteEntry describes an internal service-to-service route.
type InternalRouteEntry struct {
	ServiceName string `json:"serviceName"`
	Namespace   string `json:"namespace"`
	TargetPods  int    `json:"targetPods"`
	HasSidecar  int    `json:"hasSidecar"` // pods with sidecar
	NoSidecar   int    `json:"noSidecar"`  // pods without sidecar
	HasNP       bool   `json:"hasNP"`      // network policy protection
}

// EastWestRisk describes a risk in east-west traffic.
type EastWestRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Service   string `json:"service,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleEastWestTraffic audits east-west traffic & service-to-service connectivity.
// GET /api/product/east-west-traffic
func (s *Server) handleEastWestTraffic(w http.ResponseWriter, r *http.Request) {
	result := EastWestTrafficResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get services, pods, and network policies
	services, err := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list services: %v", err))
		return
	}

	pods, podErr := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	_ = podErr

	networkPolicies, npErr := rc.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	_ = npErr

	// Build namespace → network policy count map
	nsNPCount := map[string]int{}
	if npErr == nil {
		for _, np := range networkPolicies.Items {
			nsNPCount[np.Namespace]++
		}
	}

	// Build namespace → pod sidecar stats
	nsSidecarStats := map[string]struct{ with, without int }{}
	if podErr == nil {
		for _, pod := range pods.Items {
			ns := pod.Namespace
			stats := nsSidecarStats[ns]
			hasSidecar := false
			for _, c := range pod.Spec.Containers {
				cName := strings.ToLower(c.Name)
				if strings.Contains(cName, "istio-proxy") || strings.Contains(cName, "linkerd-proxy") ||
					strings.Contains(cName, "envoy") || strings.Contains(cName, "sidecar") {
					hasSidecar = true
					break
				}
			}
			if !hasSidecar {
				for _, c := range pod.Spec.InitContainers {
					cName := strings.ToLower(c.Name)
					if strings.Contains(cName, "istio") || strings.Contains(cName, "linkerd") {
						hasSidecar = true
						break
					}
				}
			}
			if hasSidecar {
				stats.with++
			} else {
				stats.without++
			}
			nsSidecarStats[ns] = stats
		}
	}

	// 2. Analyze each service
	nsStats := map[string]*EastWestNSStat{}
	for _, svc := range services.Items {
		result.Summary.TotalServices++
		ns := svc.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &EastWestNSStat{Namespace: ns, RiskLevel: "low"}
		}
		nsStats[ns].TotalServices++

		// Check sidecar coverage
		svcHasMesh := false
		if podErr == nil {
			svcLabels := svc.Spec.Selector
			if len(svcLabels) > 0 {
				for _, pod := range pods.Items {
					if pod.Namespace != ns {
						continue
					}
					if matchLabels(pod.Labels, svcLabels) {
						for _, c := range pod.Spec.Containers {
							cName := strings.ToLower(c.Name)
							if strings.Contains(cName, "istio-proxy") || strings.Contains(cName, "linkerd-proxy") ||
								strings.Contains(cName, "envoy") || strings.Contains(cName, "sidecar") {
								svcHasMesh = true
								break
							}
						}
					}
					if svcHasMesh {
						break
					}
				}
			}
		}

		hasNP := nsNPCount[ns] > 0
		svcType := string(svc.Spec.Type)
		isHeadless := svc.Spec.ClusterIP == "None"

		// Classify service
		switch svcType {
		case "NodePort":
			result.Summary.NodePortServices++
			nsStats[ns].Exposed++
			result.Summary.PubliclyExposed++
			result.ExposedServices = append(result.ExposedServices, ExposedServiceEntry{
				Name: svc.Name, Namespace: ns, Type: "NodePort",
				Ports: formatServicePorts(svc.Spec.Ports), HasNP: hasNP, HasMesh: svcHasMesh,
				RiskLevel: classifyServiceRisk("NodePort", hasNP, svcHasMesh),
			})
			if !hasNP {
				result.Risks = append(result.Risks, EastWestRisk{
					Namespace: ns, Service: svc.Name,
					Issue:    "NodePort service without network policy — accessible from any pod in cluster",
					Severity: "high",
				})
			}
		case "LoadBalancer":
			result.Summary.LoadBalancerSvcs++
			nsStats[ns].Exposed++
			result.Summary.PubliclyExposed++
			result.ExposedServices = append(result.ExposedServices, ExposedServiceEntry{
				Name: svc.Name, Namespace: ns, Type: "LoadBalancer",
				Ports: formatServicePorts(svc.Spec.Ports), HasNP: hasNP, HasMesh: svcHasMesh,
				RiskLevel: classifyServiceRisk("LoadBalancer", hasNP, svcHasMesh),
			})
			if !hasNP {
				result.Risks = append(result.Risks, EastWestRisk{
					Namespace: ns, Service: svc.Name,
					Issue:    "LoadBalancer service without network policy — exposed externally with no internal isolation",
					Severity: "critical",
				})
			}
		case "ExternalName":
			result.Summary.ExternalNameSvcs++
			result.Risks = append(result.Risks, EastWestRisk{
				Namespace: ns, Service: svc.Name,
				Issue:    "ExternalName service — traffic bypasses cluster networking, no mesh/mTLS coverage",
				Severity: "warning",
			})
		default:
			if isHeadless {
				result.Summary.HeadlessServices++
			} else {
				result.Summary.ClusterIPServices++
			}
			result.Summary.InternalOnly++
			nsStats[ns].Internal++
			result.InternalRoutes = append(result.InternalRoutes, InternalRouteEntry{
				ServiceName: svc.Name, Namespace: ns,
				HasNP: hasNP, HasSidecar: boolToInt(svcHasMesh),
			})
		}

		// Network policy coverage
		if hasNP {
			result.Summary.WithNetworkPolicy++
			nsStats[ns].WithNP++
		} else {
			result.Summary.WithoutNetworkPolicy++
			nsStats[ns].WithoutNP++
			if svcType == "ClusterIP" && !isHeadless {
				result.Risks = append(result.Risks, EastWestRisk{
					Namespace: ns, Service: svc.Name,
					Issue:    "Internal service without network policy — accessible from any pod in any namespace",
					Severity: "medium",
				})
				result.Summary.CrossNSAccess++
			}
		}

		// Mesh sidecar coverage
		if svcHasMesh {
			result.Summary.HasMeshSidecar++
			nsStats[ns].WithMesh++
		} else {
			result.Summary.WithoutMesh++
		}
	}

	// 3. Assess namespace risk levels
	for _, stat := range nsStats {
		if stat.Exposed > 0 && stat.WithoutNP > 0 {
			stat.RiskLevel = "high"
		} else if stat.Exposed > 0 {
			stat.RiskLevel = "medium"
		} else if stat.WithoutNP > 3 {
			stat.RiskLevel = "medium"
		} else {
			stat.RiskLevel = "low"
		}
	}

	// 4. Build namespace stats slice
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalServices > result.ByNamespace[j].TotalServices
	})

	// 5. Calculate health score
	score := 100
	if result.Summary.PubliclyExposed > 0 && result.Summary.WithoutNetworkPolicy > 0 {
		score -= 20
	}
	if result.Summary.WithoutNetworkPolicy > 0 {
		score -= min(25, result.Summary.WithoutNetworkPolicy*2)
	}
	if result.Summary.WithoutMesh > 0 && result.Summary.TotalServices > 0 {
		meshRate := result.Summary.HasMeshSidecar * 100 / result.Summary.TotalServices
		if meshRate < 50 {
			score -= 10
		}
		if meshRate == 0 {
			result.Risks = append(result.Risks, EastWestRisk{
				Issue:    "No service mesh sidecars detected — east-west traffic is unencrypted and unobservable",
				Severity: "high",
			})
		}
	}
	if result.Summary.ExternalNameSvcs > 0 {
		score -= min(10, result.Summary.ExternalNameSvcs*5)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 6. Recommendations
	if result.Summary.PubliclyExposed > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service(s) are publicly exposed — ensure network policies restrict access", result.Summary.PubliclyExposed))
	}
	if result.Summary.WithoutNetworkPolicy > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service(s) have no network policy — add NetworkPolicy to restrict east-west traffic", result.Summary.WithoutNetworkPolicy))
	}
	if result.Summary.WithoutMesh > 0 && result.Summary.TotalServices > 0 {
		meshRate := result.Summary.HasMeshSidecar * 100 / result.Summary.TotalServices
		if meshRate < 100 {
			result.Recommendations = append(result.Recommendations,
				fmt.Sprintf("Mesh sidecar coverage is %d%% — deploy sidecars to all services for mTLS and traffic observability", meshRate))
		}
	}
	if result.Summary.ExternalNameSvcs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ExternalName service(s) bypass cluster networking — consider migrating to internal services", result.Summary.ExternalNameSvcs))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"East-west traffic is well-protected — all services have network policies and mesh sidecars")
	}

	writeJSON(w, result)
}

// formatServicePorts formats service ports for summary display.
func formatServicePorts(ports []corev1.ServicePort) string {
	if len(ports) == 0 {
		return "none"
	}
	parts := []string{}
	for _, p := range ports {
		if p.NodePort > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s(NodePort)", p.Port, p.NodePort, p.Protocol))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
	}
	return strings.Join(parts, ", ")
}

// classifyServiceRisk classifies risk level for an exposed service.
func classifyServiceRisk(svcType string, hasNP, hasMesh bool) string {
	if !hasNP && (svcType == "LoadBalancer" || svcType == "NodePort") {
		return "critical"
	}
	if !hasNP {
		return "high"
	}
	if !hasMesh {
		return "medium"
	}
	return "low"
}

// boolToInt converts a bool to int (1 or 0).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
