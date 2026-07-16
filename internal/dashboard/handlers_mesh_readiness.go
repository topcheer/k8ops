package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeshReadinessResult analyzes service mesh readiness, sidecar injection status,
// mTLS coverage, and traffic management policy gaps.
type MeshReadinessResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          MeshSummary         `json:"summary"`
	MeshDetected     bool                `json:"meshDetected"`
	MeshType         string              `json:"meshType"`
	InjectionGaps    []MeshInjectionGap  `json:"injectionGaps"`
	MTLSCoverage     MeshMTLSCoverage    `json:"mtlsCoverage"`
	TrafficPolicy    []TrafficPolicyGap  `json:"trafficPolicyGaps"`
	ReadinessScore   int                 `json:"readinessScore"`
	Grade            string              `json:"grade"`
	Recommendations  []string            `json:"recommendations"`
}

type MeshSummary struct {
	TotalServices    int `json:"totalServices"`
	MeshedServices   int `json:"meshedServices"`
	UnmeshedServices int `json:"unmeshedServices"`
	NamespacesOptIn  int `json:"namespacesOptIn"`
	NamespacesMeshed int `json:"namespacesMeshed"`
	HasCircuitBreaker int `json:"hasCircuitBreaker"`
	HasRetryPolicy    int `json:"hasRetryPolicy"`
	HasTimeoutPolicy  int `json:"hasTimeoutPolicy"`
}

type MeshInjectionGap struct {
	Namespace    string `json:"namespace"`
	ServiceName  string `json:"serviceName"`
	PortCount    int    `json:"portCount"`
	Reason       string `json:"reason"`
	Impact       string `json:"impact"`
	Priority     string `json:"priority"`
}

type MeshMTLSCoverage struct {
	Mode          string  `json:"mode"`
	Score         int     `json:"score"`
	MeshedPct     float64 `json:"meshedPct"`
	UnmeshedPct   float64 `json:"unmeshedPct"`
	Status        string  `json:"status"`
}

type TrafficPolicyGap struct {
	ServiceName  string `json:"serviceName"`
	Namespace    string `json:"namespace"`
	MissingPolicy string `json:"missingPolicy"`
	Risk         string `json:"risk"`
}

// handleMeshReadiness provides service mesh readiness and mTLS gap analysis.
// GET /api/product/mesh-readiness
func (s *Server) handleMeshReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MeshReadinessResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	// Detect service mesh via pods with mesh sidecars
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Detect mesh type from running pods
	meshType := "none"
	meshPods := map[string]bool{
		"istio-proxy": true, "istiod": true,
		"linkerd-proxy": true, "linkerd-destination": true,
		"envoy": true,
	}
	meshNSDetected := map[string]bool{}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			if meshPods[c.Name] || strings.Contains(c.Image, "istio") || strings.Contains(c.Image, "linkerd") {
				meshNSDetected[pod.Namespace] = true
				if strings.Contains(c.Image, "istio") || c.Name == "istio-proxy" {
					if meshType == "none" {
						meshType = "istio"
					}
				}
				if strings.Contains(c.Image, "linkerd") || c.Name == "linkerd-proxy" {
					if meshType == "none" {
						meshType = "linkerd"
					}
				}
			}
		}
	}
	result.MeshType = meshType
	result.MeshDetected = meshType != "none"

	// Check namespace mesh injection labels
	nsMeshInjection := map[string]bool{}
	for _, ns := range namespaces.Items {
		if systemNS[ns.Name] {
			continue
		}
		if ns.Labels["istio-injection"] == "enabled" || ns.Labels["linkerd.io/inject"] == "enabled" {
			nsMeshInjection[ns.Name] = true
			result.Summary.NamespacesOptIn++
		}
		if nsMeshInjection[ns.Name] || meshNSDetected[ns.Name] {
			result.Summary.NamespacesMeshed++
		}
	}

	// Analyze services for mesh gaps
	type svcKey struct{ ns, name string }
	svcPorts := map[svcKey]int{}
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		result.Summary.TotalServices++
		svcPorts[svcKey{svc.Namespace, svc.Name}] = len(svc.Spec.Ports)
	}

	// Check which pods in service namespaces have mesh sidecars
	nsHasMeshedPods := map[string]bool{}
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if strings.Contains(c.Name, "istio-proxy") || strings.Contains(c.Name, "linkerd-proxy") || strings.Contains(c.Name, "envoy") {
				nsHasMeshedPods[pod.Namespace] = true
			}
		}
	}

	// Build injection gaps
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		hasMesh := nsMeshInjection[svc.Namespace] || nsHasMeshedPods[svc.Namespace]
		if hasMesh {
			result.Summary.MeshedServices++
		} else {
			result.Summary.UnmeshedServices++
			priority := "medium"
			portCount := len(svc.Spec.Ports)
			if portCount > 1 {
				priority = "high"
			}
			result.InjectionGaps = append(result.InjectionGaps, MeshInjectionGap{
				Namespace:   svc.Namespace,
				ServiceName: svc.Name,
				PortCount:   portCount,
				Reason:      "no sidecar injection label and no mesh sidecar detected",
				Impact:      fmt.Sprintf("Service '%s' has no mTLS, traffic splitting, or circuit breaker protection", svc.Name),
				Priority:    priority,
			})
		}
	}

	// mTLS coverage analysis
	meshedPct := 0.0
	if result.Summary.TotalServices > 0 {
		meshedPct = float64(result.Summary.MeshedServices) / float64(result.Summary.TotalServices) * 100
	}
	unmeshedPct := 100.0 - meshedPct

	mtlsScore := int(meshedPct)
	mtlsMode := "disabled"
	mtlsStatus := "no-mesh"
	if result.MeshDetected {
		mtlsMode = "permissive"
		mtlsStatus = "partial"
		if meshedPct > 80 {
			mtlsMode = "strict"
			mtlsStatus = "enforced"
		}
	}
	result.MTLSCoverage = MeshMTLSCoverage{
		Mode: mtlsMode, Score: mtlsScore,
		MeshedPct: meshedPct, UnmeshedPct: unmeshedPct,
		Status: mtlsStatus,
	}

	// Traffic policy gaps — all unmeshed services lack CB/retry/timeout
	for _, gap := range result.InjectionGaps {
		if gap.Priority == "high" {
			result.TrafficPolicy = append(result.TrafficPolicy, TrafficPolicyGap{
				ServiceName: gap.ServiceName, Namespace: gap.Namespace,
				MissingPolicy: "circuit-breaker,retry,timeout",
				Risk:          "No resilience policies — cascading failures can propagate unchecked",
			})
		}
	}

	// Score
	score := 0
	if result.MeshDetected {
		score += 30
	}
	score += int(meshedPct * 0.4)
	if result.Summary.NamespacesMeshed > 0 {
		totalNS := len(namespaces.Items) - 3
		if totalNS < 1 {
			totalNS = 1
		}
		nsRatio := result.Summary.NamespacesMeshed * 30 / totalNS
		score += nsRatio
	}
	result.ReadinessScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.ReadinessScore)

	// Sort gaps
	sort.Slice(result.InjectionGaps, func(i, j int) bool {
		return result.InjectionGaps[i].Priority > result.InjectionGaps[j].Priority
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Service mesh readiness: %d/100 (grade %s)", result.ReadinessScore, result.Grade))
	if !result.MeshDetected {
		recommendation := "No service mesh detected — consider installing Istio or Linkerd for mTLS, traffic splitting, and circuit breaking"
		recs = append(recs, recommendation)
	} else {
		recs = append(recs, fmt.Sprintf("Mesh '%s' detected — %d/%d services meshed (%.0f%%)", result.MeshType, result.Summary.MeshedServices, result.Summary.TotalServices, meshedPct))
	}
	if result.Summary.UnmeshedServices > 0 {
		recs = append(recs, fmt.Sprintf("%d services lack mesh sidecars — missing mTLS, circuit breakers, and retry policies", result.Summary.UnmeshedServices))
	}
	if meshedPct < 50 {
		recs = append(recs, "mTLS coverage below 50% — east-west traffic is largely unencrypted")
	}
	if len(result.TrafficPolicy) > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-port services have zero traffic resilience policies", len(result.TrafficPolicy)))
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
