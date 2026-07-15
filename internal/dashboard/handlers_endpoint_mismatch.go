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

// EndpointMismatchResult is the service endpoint vs pod readiness mismatch audit.
type EndpointMismatchResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         EndpointMismatchSummary `json:"summary"`
	ByNamespace     []EndpointNSStat        `json:"byNamespace"`
	MismatchedSvcs  []MismatchedService     `json:"mismatchedSvcs"`
	DeadServices    []DeadService           `json:"deadServices"`
	Risks           []EndpointRisk          `json:"risks"`
	Recommendations []string                `json:"recommendations"`
	HealthScore     int                     `json:"healthScore"`
}

// EndpointMismatchSummary aggregates mismatch metrics.
type EndpointMismatchSummary struct {
	TotalServices      int `json:"totalServices"`
	HealthyServices    int `json:"healthyServices"`    // endpoints match running pods
	MismatchedServices int `json:"mismatchedServices"` // endpoints don't match pod state
	DeadServices       int `json:"deadServices"`       // no ready endpoints
	WithSelector       int `json:"withSelector"`       // services with selector
	NoSelector         int `json:"noSelector"`         // services without selector (headless/external)
	ZeroEndpoints      int `json:"zeroEndpoints"`      // endpoints with 0 addresses
	PodsNotInEndpoints int `json:"podsNotInEndpoints"` // running pods not in any endpoint
}

// EndpointNSStat per-namespace mismatch stats.
type EndpointNSStat struct {
	Namespace     string `json:"namespace"`
	TotalServices int    `json:"totalServices"`
	Mismatched    int    `json:"mismatched"`
	Dead          int    `json:"dead"`
	Healthy       int    `json:"healthy"`
}

// MismatchedService describes a service with endpoint/pod mismatch.
type MismatchedService struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	ReadyEndpoints    int    `json:"readyEndpoints"`
	NotReadyEndpoints int    `json:"notReadyEndpoints"`
	RunningPods       int    `json:"runningPods"`
	ReadyPods         int    `json:"readyPods"`
	Issue             string `json:"issue"`
}

// DeadService describes a service with no ready endpoints.
type DeadService struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Selector  string `json:"selector,omitempty"`
	Reason    string `json:"reason"`
}

// EndpointRisk describes an endpoint-related risk.
type EndpointRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Service   string `json:"service,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleEndpointMismatch audits service endpoint vs pod readiness mismatch.
// GET /api/product/endpoint-mismatch
func (s *Server) handleEndpointMismatch(w http.ResponseWriter, r *http.Request) {
	result := EndpointMismatchResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	services, err := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list services: %v", err))
		return
	}

	endpoints, err := rc.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list endpoints: %v", err))
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// Build endpoint map: namespace/name → Endpoints
	epMap := map[string]*corev1.Endpoints{}
	for i := range endpoints.Items {
		ep := &endpoints.Items[i]
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		epMap[key] = ep
	}

	// Build pod map per namespace for selector matching
	nsPodsMap := map[string][]*corev1.Pod{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		nsPodsMap[pod.Namespace] = append(nsPodsMap[pod.Namespace], pod)
	}

	nsStats := map[string]*EndpointNSStat{}

	for i := range services.Items {
		svc := &services.Items[i]
		result.Summary.TotalServices++

		ns := svc.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &EndpointNSStat{Namespace: ns}
		}
		nsStats[ns].TotalServices++

		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		ep := epMap[key]

		// Count ready and not-ready endpoints
		readyAddrs := 0
		notReadyAddrs := 0
		if ep != nil {
			for _, subset := range ep.Subsets {
				readyAddrs += len(subset.Addresses)
				notReadyAddrs += len(subset.NotReadyAddresses)
			}
		}

		if len(svc.Spec.Selector) == 0 {
			result.Summary.NoSelector++
			continue // skip services without selector (headless/external)
		}
		result.Summary.WithSelector++

		// Count matching pods
		matchingPods := 0
		readyMatchingPods := 0
		for _, pod := range nsPodsMap[svc.Namespace] {
			if pod.Spec.NodeName == "" {
				continue
			}
			if matchLabels(pod.Labels, svc.Spec.Selector) {
				matchingPods++
				isReady := pod.Status.Phase == corev1.PodRunning
				if isReady {
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							isReady = false
							break
						}
					}
				}
				if isReady {
					readyMatchingPods++
				}
			}
		}

		// Detect mismatch
		if readyAddrs == 0 && notReadyAddrs == 0 {
			// No endpoints at all
			result.Summary.DeadServices++
			result.Summary.ZeroEndpoints++
			nsStats[ns].Dead++
			dead := DeadService{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Selector:  labelMapToString(svc.Spec.Selector),
				Reason:    "No endpoints — selector may not match any pods",
			}
			if matchingPods > 0 {
				dead.Reason = fmt.Sprintf("Selector matches %d pods but no endpoints created — check pod readiness", matchingPods)
			}
			result.DeadServices = append(result.DeadServices, dead)
			result.Risks = append(result.Risks, EndpointRisk{
				Namespace: svc.Namespace,
				Service:   svc.Name,
				Issue:     fmt.Sprintf("Service %s/%s has no ready endpoints", svc.Namespace, svc.Name),
				Severity:  "critical",
			})
		} else if readyAddrs != readyMatchingPods {
			// Mismatch between endpoints and pod readiness
			result.Summary.MismatchedServices++
			nsStats[ns].Mismatched++
			result.MismatchedSvcs = append(result.MismatchedSvcs, MismatchedService{
				Name:              svc.Name,
				Namespace:         svc.Namespace,
				ReadyEndpoints:    readyAddrs,
				NotReadyEndpoints: notReadyAddrs,
				RunningPods:       matchingPods,
				ReadyPods:         readyMatchingPods,
				Issue:             fmt.Sprintf("Endpoint ready addresses (%d) != ready pods (%d) — possible stale endpoints", readyAddrs, readyMatchingPods),
			})
			result.Risks = append(result.Risks, EndpointRisk{
				Namespace: svc.Namespace,
				Service:   svc.Name,
				Issue:     fmt.Sprintf("Service %s/%s has endpoint/pod readiness mismatch", svc.Namespace, svc.Name),
				Severity:  "warning",
			})
		} else {
			result.Summary.HealthyServices++
			nsStats[ns].Healthy++
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Mismatched+result.ByNamespace[i].Dead >
			result.ByNamespace[j].Mismatched+result.ByNamespace[j].Dead
	})

	// Health score
	score := 100
	if result.Summary.DeadServices > 0 {
		score -= min(30, result.Summary.DeadServices*10)
	}
	if result.Summary.MismatchedServices > 0 {
		score -= min(20, result.Summary.MismatchedServices*5)
	}
	if result.Summary.ZeroEndpoints > 0 {
		score -= min(15, result.Summary.ZeroEndpoints*3)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.DeadServices > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service(s) have no ready endpoints — check selectors and pod readiness", result.Summary.DeadServices))
	}
	if result.Summary.MismatchedServices > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service(s) have endpoint/pod mismatch — endpoints may be stale, check endpoint controller", result.Summary.MismatchedServices))
	}
	if result.Summary.NoSelector > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service(s) have no selector — manually managed endpoints, verify they are up to date", result.Summary.NoSelector))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"All service endpoints match pod readiness — no mismatches detected")
	}

	writeJSON(w, result)
}

// labelMapToString converts a label map to a readable string.
func labelMapToString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := []string{}
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
