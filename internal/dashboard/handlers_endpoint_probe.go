package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointProbeResult analyzes endpoint readiness by probing service backends.
type EndpointProbeResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         EndpointProbeSummary `json:"summary"`
	Unhealthy       []UnhealthyEndpoint `json:"unhealthy"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type EndpointProbeSummary struct {
	TotalServices   int `json:"totalServices"`
	HealthySvcs     int `json:"healthyServices"`
	PartialSvcs     int `json:"partialServices"`
	NoBackendSvcs   int `json:"noBackendServices"`
	TotalEndpoints  int `json:"totalEndpoints"`
	ReadyEndpoints  int `json:"readyEndpoints"`
}

type UnhealthyEndpoint struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Ready     int    `json:"ready"`
	Total     int    `json:"total"`
	Severity  string `json:"severity"`
}

// handleEndpointProbe analyzes endpoint readiness.
// GET /api/operations/endpoint-probe
func (s *Server) handleEndpointProbe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EndpointProbeResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})

	// Build endpoint health map
	epReady := map[string]int{}
	epTotal := map[string]int{}
	for _, ep := range endpoints.Items {
		key := ep.Namespace + "/" + ep.Name
		for _, sub := range ep.Subsets {
			epReady[key] += len(sub.Addresses)
			epTotal[key] += len(sub.Addresses) + len(sub.NotReadyAddresses)
		}
	}

	for _, svc := range services.Items {
		if systemNS[svc.Namespace] { continue }
		if svc.Spec.Type == corev1.ServiceTypeExternalName { continue }
		result.Summary.TotalServices++

		key := svc.Namespace + "/" + svc.Name
		ready := epReady[key]
		total := epTotal[key]
		result.Summary.TotalEndpoints += total
		result.Summary.ReadyEndpoints += ready

		if total == 0 {
			result.Summary.NoBackendSvcs++
			result.Unhealthy = append(result.Unhealthy, UnhealthyEndpoint{
				Service: svc.Name, Namespace: svc.Namespace,
				Ready: 0, Total: 0, Severity: "high",
			})
		} else if ready < total {
			result.Summary.PartialSvcs++
			result.Unhealthy = append(result.Unhealthy, UnhealthyEndpoint{
				Service: svc.Name, Namespace: svc.Namespace,
				Ready: ready, Total: total, Severity: "medium",
			})
		} else {
			result.Summary.HealthySvcs++
		}
	}

	// Score
	score := 100
	if result.Summary.TotalServices > 0 {
		healthyRatio := float64(result.Summary.HealthySvcs) / float64(result.Summary.TotalServices)
		score = int(healthyRatio * 100)
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.Unhealthy, func(i, j int) bool {
		return result.Unhealthy[i].Severity > result.Unhealthy[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Endpoint health: %d/100 (grade %s) — %d services, %d healthy, %d no-backend", result.HealthScore, result.Grade, result.Summary.TotalServices, result.Summary.HealthySvcs, result.Summary.NoBackendSvcs))
	if result.Summary.NoBackendSvcs > 0 {
		recs = append(recs, fmt.Sprintf("%d services with zero backing endpoints — pods may be down", result.Summary.NoBackendSvcs))
	}
	if result.Summary.PartialSvcs > 0 {
		recs = append(recs, fmt.Sprintf("%d services with partial endpoints — some pods not ready", result.Summary.PartialSvcs))
	}
	if len(recs) == 1 { recs = append(recs, "All service endpoints are healthy") }
	result.Recommendations = recs

	writeJSON(w, result)
}
