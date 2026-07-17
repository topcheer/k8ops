package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointExposureResult is the API endpoint exposure & attack surface mapper.
// It catalogs every externally-reachable endpoint (Ingress, LoadBalancer, NodePort),
// classifies exposure level, identifies TLS gaps, and maps the complete external
// attack surface for security auditing.
type EndpointExposureResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	Summary          EPExposureSummary  `json:"summary"`
	ExposedEndpoints []ExposedEndpoint  `json:"exposedEndpoints"`
	ByNamespace      []EPExposureNSStat `json:"byNamespace"`
	TLSGaps          []EPTLSGap         `json:"tlsGaps"`
	AttackSurface    AttackSurfaceMap   `json:"attackSurface"`
	HealthScore      int                `json:"healthScore"`
	Grade            string             `json:"grade"`
	Recommendations  []string           `json:"recommendations"`
}

// ExposureSummary aggregates exposure statistics.
type EPExposureSummary struct {
	TotalServices    int `json:"totalServices"`
	IngressServices  int `json:"ingressServices"`
	LoadBalancerSvc  int `json:"loadBalancerServices"`
	NodePortServices int `json:"nodePortServices"`
	ClusterIPOnly    int `json:"clusterIPOnly"`
	ExternalName     int `json:"externalName"`
	HeadlessServices int `json:"headlessServices"`
	WithTLS          int `json:"withTLS"`
	WithoutTLS       int `json:"withoutTLS"`
	ExposedPorts     int `json:"exposedPorts"`
}

// ExposedEndpoint describes one externally-reachable endpoint.
type ExposedEndpoint struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	ServiceType     string   `json:"serviceType"`
	Ports           []int    `json:"ports"`
	Protocol        string   `json:"protocol"`
	ExposureLevel   string   `json:"exposureLevel"` // public, internal, cluster-only
	HasIngress      bool     `json:"hasIngress"`
	HasTLS          bool     `json:"hasTLS"`
	Hosts           []string `json:"hosts,omitempty"`
	BackendWorkload string   `json:"backendWorkload"`
	Risk            string   `json:"risk"`
}

// ExposureNSStat per-namespace exposure stats.
type EPExposureNSStat struct {
	Namespace     string  `json:"namespace"`
	TotalServices int     `json:"totalServices"`
	ExposedCount  int     `json:"exposedCount"`
	TLSCount      int     `json:"tlsCount"`
	PortCount     int     `json:"portCount"`
	ExposurePct   float64 `json:"exposurePct"`
}

// TLSGap describes a missing TLS configuration.
type EPTLSGap struct {
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"` // missing-tls, expired-cert, http-only
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// AttackSurfaceMap summarizes the external attack surface.
type AttackSurfaceMap struct {
	TotalExposedPorts int `json:"totalExposedPorts"`
	PublicEndpoints   int `json:"publicEndpoints"`
	InternalEndpoints int `json:"internalEndpoints"`
	ClusterOnlyCount  int `json:"clusterOnlyCount"`
	UniqueHosts       int `json:"uniqueHosts"`
	HighRiskEndpoints int `json:"highRiskEndpoints"`
}

// handleEndpointExposureMap handles GET /api/security/endpoint-exposure-map
func (s *Server) handleAttackSurface(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EndpointExposureResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build ingress→service map
	ingressSvcMap := map[string]*networkingv1.Ingress{}
	for _, ing := range ingresses.Items {
		for _, rule := range ing.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				key := ing.Namespace + "/" + path.Backend.Service.Name
				ingressSvcMap[key] = &ing
			}
		}
	}

	// Build pod→workload map
	podOwnerMap := map[string]string{}
	for _, pod := range pods.Items {
		owner := pod.Name
		if len(pod.OwnerReferences) > 0 {
			owner = pod.OwnerReferences[0].Name
		}
		podOwnerMap[pod.Namespace+"/"+pod.Name] = owner
	}

	nsStats := map[string]*EPExposureNSStat{}
	hostSet := map[string]bool{}

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}

		result.Summary.TotalServices++

		// Determine exposure level
		exposureLevel := "cluster-only"
		risk := "low"
		switch svc.Spec.Type {
		case corev1.ServiceTypeLoadBalancer:
			exposureLevel = "public"
			risk = "high"
			result.Summary.LoadBalancerSvc++
		case corev1.ServiceTypeNodePort:
			exposureLevel = "public"
			risk = "medium"
			result.Summary.NodePortServices++
		case corev1.ServiceTypeExternalName:
			exposureLevel = "external"
			risk = "low"
			result.Summary.ExternalName++
		default:
			result.Summary.ClusterIPOnly++
			if svc.Spec.ClusterIP == "None" {
				result.Summary.HeadlessServices++
			}
		}

		// Check ingress
		key := svc.Namespace + "/" + svc.Name
		hasIngress := false
		var hosts []string
		hasTLS := false
		if ing, ok := ingressSvcMap[key]; ok {
			hasIngress = true
			result.Summary.IngressServices++
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" {
					hosts = append(hosts, rule.Host)
					hostSet[rule.Host] = true
				}
			}
			if len(ing.Spec.TLS) > 0 {
				hasTLS = true
				result.Summary.WithTLS++
			} else {
				result.Summary.WithoutTLS++
				result.TLSGaps = append(result.TLSGaps, EPTLSGap{
					Resource: "Ingress/" + ing.Name, Namespace: ing.Namespace,
					Type: "missing-tls", Severity: "high",
					Detail: fmt.Sprintf("Ingress %s has no TLS configured — traffic is unencrypted", ing.Name),
				})
			}
			if exposureLevel == "cluster-only" {
				exposureLevel = "public"
				risk = "high"
			}
		}

		// Collect ports
		var ports []int
		for _, p := range svc.Spec.Ports {
			ports = append(ports, int(p.Port))
			result.Summary.ExposedPorts++
		}

		// Find backend workload
		backendWk := ""
		for _, pod := range pods.Items {
			if pod.Namespace != svc.Namespace || len(svc.Spec.Selector) == 0 {
				continue
			}
			allMatch := true
			for k, v := range svc.Spec.Selector {
				if pod.Labels[k] != v {
					allMatch = false
					break
				}
			}
			if allMatch && len(pod.OwnerReferences) > 0 {
				backendWk = pod.OwnerReferences[0].Name
				break
			}
		}

		result.ExposedEndpoints = append(result.ExposedEndpoints, ExposedEndpoint{
			Name: svc.Name, Namespace: svc.Namespace, ServiceType: string(svc.Spec.Type),
			Ports: ports, ExposureLevel: exposureLevel,
			HasIngress: hasIngress, HasTLS: hasTLS, Hosts: hosts,
			BackendWorkload: backendWk, Risk: risk,
		})

		// Namespace stats
		ns := svc.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &EPExposureNSStat{Namespace: ns}
		}
		nsStats[ns].TotalServices++
		if exposureLevel != "cluster-only" {
			nsStats[ns].ExposedCount++
		}
		if hasTLS {
			nsStats[ns].TLSCount++
		}
		nsStats[ns].PortCount += len(ports)
	}

	// Build namespace stats
	for _, ns := range nsStats {
		if ns.TotalServices > 0 {
			ns.ExposurePct = float64(ns.ExposedCount) / float64(ns.TotalServices) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ExposedCount > result.ByNamespace[j].ExposedCount
	})

	// Attack surface map
	publicCount := 0
	internalCount := 0
	clusterOnly := 0
	highRisk := 0
	for _, ep := range result.ExposedEndpoints {
		switch ep.ExposureLevel {
		case "public":
			publicCount++
		case "internal":
			internalCount++
		default:
			clusterOnly++
		}
		if ep.Risk == "high" {
			highRisk++
		}
	}
	result.AttackSurface = AttackSurfaceMap{
		TotalExposedPorts: result.Summary.ExposedPorts,
		PublicEndpoints:   publicCount,
		InternalEndpoints: internalCount,
		ClusterOnlyCount:  clusterOnly,
		UniqueHosts:       len(hostSet),
		HighRiskEndpoints: highRisk,
	}

	// Score
	result.HealthScore = computeExposureScore(result.Summary, len(result.TLSGaps))
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateExposureRecs(result)

	writeJSON(w, result)
}

// computeExposureScore computes security posture from exposure data.
func computeExposureScore(s EPExposureSummary, tlsGaps int) int {
	score := 100
	if s.TotalServices == 0 {
		return score
	}
	// Penalize exposed services without TLS
	score -= minInt(tlsGaps*5, 25)
	// Penalize LoadBalancer services (direct internet exposure)
	if s.LoadBalancerSvc > 3 {
		score -= 10
	}
	// Penalize high exposure ratio
	exposedRatio := float64(s.LoadBalancerSvc+s.NodePortServices+s.IngressServices) / float64(s.TotalServices)
	if exposedRatio > 0.5 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateExposureRecs produces security recommendations.
func generateExposureRecs(r EndpointExposureResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Attack surface: %d services (%d public, %d with ingress), %d ports exposed — score %d/100",
		r.Summary.TotalServices, r.AttackSurface.PublicEndpoints, r.Summary.IngressServices, r.Summary.ExposedPorts, r.HealthScore))

	if len(r.TLSGaps) > 0 {
		recs = append(recs, fmt.Sprintf("%d TLS gap(s) detected — add TLS to ingress resources", len(r.TLSGaps)))
	}

	if r.Summary.LoadBalancerSvc > 3 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer service(s) — consolidate behind Ingress/Gateway API", r.Summary.LoadBalancerSvc))
	}

	if r.AttackSurface.HighRiskEndpoints > 0 {
		recs = append(recs, fmt.Sprintf("%d high-risk endpoint(s) — review exposure and add WAF/Rate limiting", r.AttackSurface.HighRiskEndpoints))
	}

	if r.AttackSurface.UniqueHosts > 0 {
		recs = append(recs, fmt.Sprintf("%d unique external hostname(s) — ensure DNS and cert management coverage", r.AttackSurface.UniqueHosts))
	}

	return recs
}
