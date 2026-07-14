package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointSliceResult is the endpoint slice health & topology-aware routing audit.
type EndpointSliceResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         EndpointSliceSummary   `json:"summary"`
	Services        []EndpointSliceService `json:"services"`
	Gaps            []EndpointSliceGap     `json:"gaps"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// EndpointSliceSummary aggregates endpoint slice statistics.
type EndpointSliceSummary struct {
	TotalServices         int `json:"totalServices"`
	ServicesWithEndpoints int `json:"servicesWithEndpoints"`
	ServicesNoEndpoints   int `json:"servicesNoEndpoints"`
	TotalSlices           int `json:"totalSlices"`
	TotalEndpoints        int `json:"totalEndpoints"`
	ReadyEndpoints        int `json:"readyEndpoints"`
	NotReadyEndpoints     int `json:"notReadyEndpoints"`
	WithTopologyHints     int `json:"withTopologyHints"`
	NoTopologyHints       int `json:"noTopologyHints"`
	CrossZoneEndpoints    int `json:"crossZoneEndpoints"`
}

// EndpointSliceService describes a service's endpoint slice status.
type EndpointSliceService struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	SliceCount       int      `json:"sliceCount"`
	EndpointCount    int      `json:"endpointCount"`
	ReadyCount       int      `json:"readyCount"`
	NotReadyCount    int      `json:"notReadyCount"`
	HasTopologyHints bool     `json:"hasTopologyHints"`
	Zones            []string `json:"zones"`
	Status           string   `json:"status"`
}

// EndpointSliceGap describes a service with endpoint issues.
type EndpointSliceGap struct {
	Namespace string `json:"namespace"`
	Service   string `json:"service"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleEndpointSlice audits endpoint slice health & topology-aware routing.
// GET /api/product/endpoint-slice
func (s *Server) handleEndpointSlice(w http.ResponseWriter, r *http.Request) {
	result := EndpointSliceResult{
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

	// 1. List all EndpointSlices
	endpointSlices, err := rc.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		// EndpointSlices may not be available — fall back to Endpoints
		writeError(w, http.StatusServiceUnavailable, "EndpointSlice API not available")
		return
	}

	// Group slices by service
	serviceMap := make(map[string]*EndpointSliceService)

	for _, slice := range endpointSlices.Items {
		if systemNamespaces[slice.Namespace] {
			continue
		}

		svcName := ""
		if slice.Labels != nil {
			svcName = slice.Labels["kubernetes.io/service-name"]
		}
		if svcName == "" {
			continue
		}

		key := fmt.Sprintf("%s/%s", slice.Namespace, svcName)
		svc, ok := serviceMap[key]
		if !ok {
			svc = &EndpointSliceService{
				Name:      svcName,
				Namespace: slice.Namespace,
			}
			serviceMap[key] = svc
		}
		svc.SliceCount++
		result.Summary.TotalSlices++

		zones := make(map[string]bool)
		hasTopologyHints := false

		for _, ep := range slice.Endpoints {
			svc.EndpointCount++
			result.Summary.TotalEndpoints++

			// Check ready
			isReady := false
			if ep.Conditions.Ready != nil {
				if *ep.Conditions.Ready {
					isReady = true
					svc.ReadyCount++
					result.Summary.ReadyEndpoints++
				} else {
					svc.NotReadyCount++
					result.Summary.NotReadyEndpoints++
				}
			} else {
				// No conditions = assume ready
				isReady = true
				svc.ReadyCount++
				result.Summary.ReadyEndpoints++
			}

			// Check topology hints
			if ep.DeprecatedTopology != nil {
				hasTopologyHints = true
				if zone, ok := ep.DeprecatedTopology["topology.kubernetes.io/zone"]; ok {
					zones[zone] = true
				}
			}
			if ep.Zone != nil {
				hasTopologyHints = true
				zones[*ep.Zone] = true
			}

			_ = isReady
		}

		if hasTopologyHints {
			svc.HasTopologyHints = true
			result.Summary.WithTopologyHints++
		} else {
			result.Summary.NoTopologyHints++
		}

		// Collect unique zones
		for zone := range zones {
			svc.Zones = append(svc.Zones, zone)
			result.Summary.CrossZoneEndpoints++
		}
	}

	// 2. List services and cross-reference with endpoint slices
	services, err := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range services.Items {
			if systemNamespaces[svc.Namespace] {
				continue
			}
			result.Summary.TotalServices++

			key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
			es, ok := serviceMap[key]
			if !ok {
				es = &EndpointSliceService{
					Name:      svc.Name,
					Namespace: svc.Namespace,
					Status:    "no-endpoints",
				}
				serviceMap[key] = es
				result.Summary.ServicesNoEndpoints++
				result.Gaps = append(result.Gaps, EndpointSliceGap{
					Namespace: svc.Namespace,
					Service:   svc.Name,
					Issue:     "Service has no endpoint slices — no backing pods",
					Severity:  "high",
				})
			} else {
				result.Summary.ServicesWithEndpoints++
				if es.NotReadyCount > 0 {
					es.Status = "degraded"
					result.Gaps = append(result.Gaps, EndpointSliceGap{
						Namespace: svc.Namespace,
						Service:   svc.Name,
						Issue:     fmt.Sprintf("%d endpoints not ready", es.NotReadyCount),
						Severity:  "medium",
					})
				} else if es.ReadyCount == 0 {
					es.Status = "critical"
					result.Gaps = append(result.Gaps, EndpointSliceGap{
						Namespace: svc.Namespace,
						Service:   svc.Name,
						Issue:     "All endpoints not ready",
						Severity:  "critical",
					})
				} else {
					es.Status = "healthy"
				}

				if !es.HasTopologyHints && es.ReadyCount > 1 {
					result.Gaps = append(result.Gaps, EndpointSliceGap{
						Namespace: svc.Namespace,
						Service:   svc.Name,
						Issue:     "No topology hints — traffic may cross zones unnecessarily",
						Severity:  "low",
					})
				}
			}
		}
	}

	// Build service list
	for _, svc := range serviceMap {
		result.Services = append(result.Services, *svc)
	}
	sort.Slice(result.Services, func(i, j int) bool {
		return result.Services[i].Status > result.Services[j].Status
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if result.Summary.ServicesNoEndpoints > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d services have no endpoints — check backing pods and selectors", result.Summary.ServicesNoEndpoints))
	}
	if result.Summary.NotReadyEndpoints > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d endpoints not ready — check pod health and readiness probes", result.Summary.NotReadyEndpoints))
	}
	if result.Summary.NoTopologyHints > 0 && result.Summary.WithTopologyHints == 0 {
		result.Recommendations = append(result.Recommendations,
			"No topology hints detected — enable topology-aware routing for better traffic distribution")
	}

	// Health score
	score := 100
	if result.Summary.TotalServices > 0 {
		score = (result.Summary.ServicesWithEndpoints * 100) / result.Summary.TotalServices
		score -= result.Summary.NotReadyEndpoints * 2
		if result.Summary.NoTopologyHints > 0 {
			score -= 5
		}
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score

	writeJSON(w, result)
}

var _ = discoveryv1.EndpointSlice{}
var _ = corev1.ServiceSpec{}
var _ = strings.Contains
