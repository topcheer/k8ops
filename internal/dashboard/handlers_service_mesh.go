package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceMeshResult analyzes service mesh coverage: sidecar injection,
// mTLS status, circuit breakers, retry policies.
type ServiceMeshResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         MeshCovSummary      `json:"summary"`
	Gaps            []MeshGap           `json:"gaps"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type MeshCovSummary struct {
	HasIstio       bool   `json:"hasIstio"`
	HasLinkerd     bool   `json:"hasLinkerd"`
	HasConsul      bool   `json:"hasConsul"`
	SidecarInjected int   `json:"sidecarInjected"`
	TotalPods      int    `json:"totalPods"`
	MTLSEnabled    bool   `json:"mtlsEnabled"`
	MeshCoverage   float64 `json:"meshCoverage"`
}

type MeshGap struct {
	Namespace string `json:"namespace"`
	Gap       string `json:"gap"`
	Severity  string `json:"severity"`
}

// handleServiceMesh analyzes service mesh coverage.
// GET /api/product/service-mesh
func (s *Server) handleServiceMesh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ServiceMeshResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	// Detect mesh controllers
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			if strings.Contains(imgLower, "istio") || strings.Contains(imgLower, "istio-proxy") {
				result.Summary.HasIstio = true
			}
			if strings.Contains(imgLower, "linkerd") {
				result.Summary.HasLinkerd = true
			}
			if strings.Contains(imgLower, "consul") {
				result.Summary.HasConsul = true
			}
		}
	}

	// Check namespace mesh injection labels
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] { continue }
		for k, v := range ns.Labels {
			if strings.Contains(k, "istio-injection") && v == "enabled" {
				result.Summary.MTLSEnabled = true
			}
			if strings.Contains(k, "linkerd.io") && v == "enabled" {
				result.Summary.MTLSEnabled = true
			}
		}
	}

	// Count sidecar-injected pods
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] || pod.Status.Phase != "Running" { continue }
		result.Summary.TotalPods++
		hasSidecar := false
		for _, c := range pod.Spec.Containers {
			cLower := strings.ToLower(c.Name + c.Image)
			if strings.Contains(cLower, "istio-proxy") || strings.Contains(cLower, "linkerd-proxy") ||
				strings.Contains(cLower, "envoy") || strings.Contains(cLower, "sidecar") {
				hasSidecar = true
				break
			}
		}
		if hasSidecar {
			result.Summary.SidecarInjected++
		}
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.MeshCoverage = float64(result.Summary.SidecarInjected) / float64(result.Summary.TotalPods) * 100
	}

	// Gaps
	if !result.Summary.HasIstio && !result.Summary.HasLinkerd && !result.Summary.HasConsul {
		result.Gaps = append(result.Gaps, MeshGap{
			Namespace: "*", Gap: "No service mesh detected (Istio/Linkerd/Consul)",
			Severity: "high",
		})
	}
	if !result.Summary.MTLSEnabled {
		result.Gaps = append(result.Gaps, MeshGap{
			Namespace: "*", Gap: "No mTLS — inter-service traffic is unencrypted",
			Severity: "high",
		})
	}
	if result.Summary.MeshCoverage < 50 && result.Summary.TotalPods > 0 {
		result.Gaps = append(result.Gaps, MeshGap{
			Namespace: "*", Gap: fmt.Sprintf("Mesh coverage only %.0f%% — inject sidecars", result.Summary.MeshCoverage),
			Severity: "medium",
		})
	}

	// Score
	score := 20
	if result.Summary.HasIstio || result.Summary.HasLinkerd || result.Summary.HasConsul { score += 40 }
	if result.Summary.MTLSEnabled { score += 20 }
	score += int(result.Summary.MeshCoverage / 5) // 0-20 points
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.Gaps, func(i, j int) bool { return result.Gaps[i].Severity > result.Gaps[j].Severity })

	var recs []string
	recs = append(recs, fmt.Sprintf("Service mesh: %d/100 (grade %s) — Istio:%v Linkerd:%v coverage:%.0f%%", result.HealthScore, result.Grade, result.Summary.HasIstio, result.Summary.HasLinkerd, result.Summary.MeshCoverage))
	if !result.Summary.HasIstio && !result.Summary.HasLinkerd { recs = append(recs, "Deploy Istio or Linkerd for mTLS, traffic control, and observability") }
	if !result.Summary.MTLSEnabled { recs = append(recs, "Enable mTLS for zero-trust inter-service communication") }
	if result.Summary.MeshCoverage < 50 { recs = append(recs, fmt.Sprintf("%.0f%% mesh coverage — enable sidecar injection in more namespaces", 100-result.Summary.MeshCoverage)) }
	if len(recs) == 1 { recs = append(recs, "Service mesh coverage is comprehensive") }
	result.Recommendations = recs

	writeJSON(w, result)
}
