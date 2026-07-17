package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayHealthResult analyzes API gateway and ingress controller health.
type GatewayHealthResult struct {
	ScannedAt        time.Time                `json:"scannedAt"`
	Summary          GatewayHealthSummary     `json:"summary"`
	ControllerHealth []GatewayControllerEntry `json:"controllerHealth"`
	IngressGaps      []GatewayIngressGap      `json:"ingressGaps"`
	HealthScore      int                      `json:"healthScore"`
	Grade            string                   `json:"grade"`
	Recommendations  []string                 `json:"recommendations"`
}

type GatewayHealthSummary struct {
	ControllerType          string `json:"controllerType"`
	ControllerRunning       bool   `json:"controllerRunning"`
	TotalIngresses          int    `json:"totalIngresses"`
	HealthyIngresses        int    `json:"healthyIngresses"`
	IngressesWithoutTLS     int    `json:"ingressesWithoutTLS"`
	IngressesWithoutBackend int    `json:"ingressesWithoutBackend"`
}

type GatewayControllerEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Type      string `json:"type"`
}

type GatewayIngressGap struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Gap       string `json:"gap"`
	Severity  string `json:"severity"`
}

// handleGatewayHealth analyzes API gateway and ingress controller health.
// GET /api/product/api-gateway-health
func (s *Server) handleGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GatewayHealthResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Detect ingress controllers
	controllerKeywords := map[string]string{
		"traefik": "Traefik", "nginx-ingress": "Nginx", "ingress-nginx": "Nginx",
		"haproxy": "HAProxy", "contour": "Contour", "envoy": "Envoy",
		"ambassador": "Ambassador", "kong": "Kong",
	}
	for _, dep := range deployments.Items {
		depLower := strings.ToLower(dep.Name + " " + dep.Namespace)
		for kw, ctrlType := range controllerKeywords {
			if strings.Contains(depLower, kw) {
				ready := dep.Status.ReadyReplicas > 0
				result.Summary.ControllerType = ctrlType
				result.Summary.ControllerRunning = ready
				result.ControllerHealth = append(result.ControllerHealth, GatewayControllerEntry{
					Name: dep.Name, Namespace: dep.Namespace, Ready: ready, Type: ctrlType,
				})
			}
		}
	}
	// Also check pods
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, ctrlType := range controllerKeywords {
				if strings.Contains(imgLower, kw) && result.Summary.ControllerType == "" {
					result.Summary.ControllerType = ctrlType
					result.Summary.ControllerRunning = pod.Status.Phase == "Running"
				}
			}
		}
	}

	// Build service map for backend checking
	svcMap := map[string]bool{}
	for _, svc := range services.Items {
		svcMap[svc.Namespace+"/"+svc.Name] = true
	}

	// Analyze ingresses
	for _, ing := range ingresses.Items {
		if systemNS[ing.Namespace] {
			continue
		}
		result.Summary.TotalIngresses++

		// Check TLS
		hasTLS := len(ing.Spec.TLS) > 0
		if !hasTLS {
			result.Summary.IngressesWithoutTLS++
			result.IngressGaps = append(result.IngressGaps, GatewayIngressGap{
				Name: ing.Name, Namespace: ing.Namespace,
				Gap: "No TLS configured — traffic unencrypted", Severity: "high",
			})
		}

		// Check backend services exist
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					backendName := path.Backend.Service.Name
					if !svcMap[ing.Namespace+"/"+backendName] {
						result.Summary.IngressesWithoutBackend++
						result.IngressGaps = append(result.IngressGaps, GatewayIngressGap{
							Name: ing.Name, Namespace: ing.Namespace,
							Gap: fmt.Sprintf("Backend service '%s' not found", backendName), Severity: "critical",
						})
					}
				}
			}
		}

		if len(result.IngressGaps) == 0 || (hasTLS && result.Summary.IngressesWithoutBackend == 0) {
			result.Summary.HealthyIngresses++
		}
	}

	// Score
	score := 50
	if result.Summary.ControllerRunning {
		score += 25
	}
	if result.Summary.TotalIngresses > 0 {
		score += result.Summary.HealthyIngresses * 25 / result.Summary.TotalIngresses
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.IngressGaps, func(i, j int) bool {
		return result.IngressGaps[i].Severity > result.IngressGaps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Gateway health: %d/100 (grade %s) — controller: %s, %d ingresses", result.HealthScore, result.Grade, result.Summary.ControllerType, result.Summary.TotalIngresses))
	if !result.Summary.ControllerRunning {
		recs = append(recs, "Ingress controller not running or not detected — deploy Traefik/Nginx/Envoy")
	}
	if result.Summary.IngressesWithoutTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d ingresses without TLS — configure cert-manager for automatic TLS", result.Summary.IngressesWithoutTLS))
	}
	if result.Summary.IngressesWithoutBackend > 0 {
		recs = append(recs, fmt.Sprintf("%d ingresses with missing backend services — fix routing rules", result.Summary.IngressesWithoutBackend))
	}
	if len(recs) == 1 {
		recs = append(recs, "Gateway and ingress configuration is healthy")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
