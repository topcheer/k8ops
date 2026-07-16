package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficFlowResult analyzes east-west service communication patterns:
// service-to-service connectivity, external dependency mapping,
// traffic flow topology, and unexposed service detection.
type TrafficFlowResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         TrafficFlowSummary  `json:"summary"`
	ServiceFlows    []ServiceFlow       `json:"serviceFlows"`
	IsolatedServices []IsolatedService  `json:"isolatedServices"`
	FlowScore       int                 `json:"flowScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type TrafficFlowSummary struct {
	TotalServices    int `json:"totalServices"`
	ClusterIPLoadBalancer int `json:"clusterIPLoadBalancer"`
	HeadlessServices int `json:"headlessServices"`
	ExternalNameSvc  int `json:"externalNameServices"`
	NodePortServices int `json:"nodePortServices"`
	NoSelectorSvc    int `json:"noSelectorServices"`
}

type ServiceFlow struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Type        string   `json:"type"`
	ClusterIP   string   `json:"clusterIP"`
	Ports       []string `json:"ports"`
	HasEndpoint bool     `json:"hasEndpoint"`
	BackendCount int     `json:"backendCount"`
	ExposureLevel string `json:"exposureLevel"`
}

type IsolatedService struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// handleTrafficFlow analyzes east-west service communication and traffic flow.
// GET /api/product/traffic-flow
func (s *Server) handleTrafficFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TrafficFlowResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build endpoint map: ns/name -> address count
	endpointMap := map[string]int{}
	for _, ep := range endpoints.Items {
		key := ep.Namespace + "/" + ep.Name
		count := 0
		for _, sub := range ep.Subsets {
			count += len(sub.Addresses)
			count += len(sub.NotReadyAddresses)
		}
		endpointMap[key] = count
	}

	// Build pod count per namespace
	nsPodCount := map[string]int{}
	for _, pod := range pods.Items {
		if !systemNS[pod.Namespace] && pod.Status.Phase == corev1.PodRunning {
			nsPodCount[pod.Namespace]++
		}
	}

	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		result.Summary.TotalServices++

		svcType := string(svc.Spec.Type)
		key := svc.Namespace + "/" + svc.Name
		backendCount := endpointMap[key]

		var ports []string
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, string(p.Protocol)))
		}

		// Classify exposure level
		exposure := "cluster-internal"
		if svcType == "LoadBalancer" {
			exposure = "internet-facing"
			result.Summary.ClusterIPLoadBalancer++
		} else if svcType == "NodePort" {
			exposure = "node-port"
			result.Summary.NodePortServices++
		}

		if svc.Spec.ClusterIP == "None" {
			result.Summary.HeadlessServices++
			exposure = "headless"
		}
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalNameSvc++
			exposure = "external-name"
		}
		if svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
			result.Summary.NoSelectorSvc++
		}

		hasEndpoint := backendCount > 0
		result.ServiceFlows = append(result.ServiceFlows, ServiceFlow{
			Name:          svc.Name,
			Namespace:     svc.Namespace,
			Type:          svcType,
			ClusterIP:     svc.Spec.ClusterIP,
			Ports:         ports,
			HasEndpoint:   hasEndpoint,
			BackendCount:  backendCount,
			ExposureLevel: exposure,
		})

		// Check for isolated/orphaned services
		if !hasEndpoint && svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
			result.IsolatedServices = append(result.IsolatedServices, IsolatedService{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Reason:    "Service has selector but no backing endpoints — pods may be down",
				Severity:  "high",
			})
		}
	}

	// Score
	score := 100
	score -= len(result.IsolatedServices) * 10
	if result.Summary.ClusterIPLoadBalancer > 5 {
		score -= (result.Summary.ClusterIPLoadBalancer - 5) * 3
	}
	if score < 0 {
		score = 0
	}
	result.FlowScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.FlowScore)

	// Sort
	sort.Slice(result.ServiceFlows, func(i, j int) bool {
		return result.ServiceFlows[i].BackendCount < result.ServiceFlows[j].BackendCount
	})
	sort.Slice(result.IsolatedServices, func(i, j int) bool {
		return result.IsolatedServices[i].Severity > result.IsolatedServices[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Traffic flow health: %d/100 (grade %s) — %d services mapped", result.FlowScore, result.Grade, result.Summary.TotalServices))
	if len(result.IsolatedServices) > 0 {
		recs = append(recs, fmt.Sprintf("%d services with selectors but no backing endpoints — pods may be misconfigured or down", len(result.IsolatedServices)))
	}
	if result.Summary.ClusterIPLoadBalancer > 0 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer services exposed to internet — review if all need external access", result.Summary.ClusterIPLoadBalancer))
	}
	if result.Summary.HeadlessServices > 0 {
		recs = append(recs, fmt.Sprintf("%d headless services — used for StatefulSet DNS, verify pod readiness", result.Summary.HeadlessServices))
	}
	if result.Summary.NoSelectorSvc > 0 {
		recs = append(recs, fmt.Sprintf("%d services without selectors — manually managed endpoints, verify correctness", result.Summary.NoSelectorSvc))
	}
	if len(recs) == 1 {
		recs = append(recs, "Traffic flow topology is healthy — all services have backing endpoints")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
