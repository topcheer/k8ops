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

// MeshHealthResult is the service mesh sidecar health & mTLS coverage audit.
type MeshHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         MeshHealthSummary  `json:"summary"`
	ByNamespace     []MeshNSStat       `json:"byNamespace"`
	SidecarPods     []MeshSidecarEntry `json:"sidecarPods"`
	Issues          []MeshIssue        `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

// MeshHealthSummary aggregates service mesh health statistics.
type MeshHealthSummary struct {
	HasIstio           bool `json:"hasIstio"`
	HasLinkerd         bool `json:"hasLinkerd"`
	HasConsul          bool `json:"hasConsul"`
	TotalPods          int  `json:"totalPods"`
	PodsWithSidecar    int  `json:"podsWithSidecar"`
	PodsWithoutSidecar int  `json:"podsWithoutSidecar"`
	MTLSEnabled        int  `json:"mtlsEnabled"`     // pods with mTLS annotations
	MTLSDisabled       int  `json:"mtlsDisabled"`    // pods with mTLS explicitly disabled
	MTLSUnknown        int  `json:"mtlsUnknown"`     // pods with unknown mTLS status
	SidecarRestarts    int  `json:"sidecarRestarts"` // sidecar containers with high restart count
	HealthScore        int  `json:"healthScore"`
}

// MeshSidecarEntry describes a pod with a mesh sidecar.
type MeshSidecarEntry struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	SidecarName  string `json:"sidecarName"`
	SidecarImage string `json:"sidecarImage"`
	RestartCount int32  `json:"restartCount"`
	MTLSStatus   string `json:"mtlsStatus"` // enabled, disabled, unknown
}

// MeshIssue is a detected mesh issue.
type MeshIssue struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// MeshNSStat shows mesh coverage per namespace.
type MeshNSStat struct {
	Namespace       string `json:"namespace"`
	TotalPods       int    `json:"totalPods"`
	PodsWithSidecar int    `json:"podsWithSidecar"`
	PodsWithout     int    `json:"podsWithout"`
	MTLSEnabled     int    `json:"mtlsEnabled"`
}

// Known sidecar container names and images
var knownSidecarNames = map[string]bool{
	"istio-proxy":    true,
	"linkerd-proxy":  true,
	"envoy":          true,
	"consul-connect": true,
}

var knownSidecarImages = []string{
	"istio/proxyv2",
	"linkerd/proxy",
	"envoyproxy/envoy",
	"consul",
}

// meshHealthAuditCore performs the service mesh audit on pods (testable).
func meshHealthAuditCore(pods []corev1.Pod) MeshHealthResult {
	result := MeshHealthResult{
		ScannedAt: time.Now(),
	}

	// First pass: detect mesh control plane
	for i := range pods {
		pod := &pods[i]
		podName := strings.ToLower(pod.Name)
		ns := strings.ToLower(pod.Namespace)

		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			// Detect Istio control plane
			if strings.Contains(podName, "istiod") || strings.Contains(ns, "istio-system") ||
				strings.Contains(img, "istio/pilot") || strings.Contains(img, "istio/proxyv2") {
				if strings.Contains(podName, "istiod") || strings.Contains(img, "istio/pilot") {
					result.Summary.HasIstio = true
				}
			}
			// Detect Linkerd control plane
			if strings.Contains(ns, "linkerd") || strings.Contains(podName, "linkerd") {
				if strings.Contains(podName, "linkerd-identity") || strings.Contains(podName, "linkerd-proxy-injector") ||
					strings.Contains(img, "linkerd/controller") {
					result.Summary.HasLinkerd = true
				}
			}
			// Detect Consul Connect
			if strings.Contains(img, "consul") && strings.Contains(podName, "connect") {
				result.Summary.HasConsul = true
			}
		}
	}

	// Second pass: analyze each pod for sidecar and mTLS
	nsStats := make(map[string]*MeshNSStat)

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &MeshNSStat{Namespace: ns}
		}
		nsStats[ns].TotalPods++
		result.Summary.TotalPods++

		// Skip mesh control plane namespaces
		if isMeshControlPlaneNS(ns) {
			continue
		}

		// Find sidecar container
		var sidecar *corev1.Container
		var sidecarIdx int
		for j := range pod.Spec.Containers {
			c := &pod.Spec.Containers[j]
			if knownSidecarNames[strings.ToLower(c.Name)] {
				sidecar = c
				sidecarIdx = j
				break
			}
			imgLower := strings.ToLower(c.Image)
			for _, known := range knownSidecarImages {
				if strings.Contains(imgLower, known) {
					sidecar = c
					sidecarIdx = j
					break
				}
			}
			if sidecar != nil {
				break
			}
		}

		if sidecar != nil {
			result.Summary.PodsWithSidecar++
			nsStats[ns].PodsWithSidecar++

			// Check restart count
			var restartCount int32
			if sidecarIdx < len(pod.Status.ContainerStatuses) {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Name == sidecar.Name {
						restartCount = cs.RestartCount
						break
					}
				}
			}

			// Check mTLS status from annotations
			mtlsStatus := "unknown"
			if pod.Annotations != nil {
				if mtls, ok := pod.Annotations["security.istio.io/tlsMode"]; ok {
					if mtls == "istio" || mtls == "strict" {
						mtlsStatus = "enabled"
						result.Summary.MTLSEnabled++
						nsStats[ns].MTLSEnabled++
					} else if mtls == "disable" || mtls == "disabled" {
						mtlsStatus = "disabled"
						result.Summary.MTLSDisabled++
					} else {
						result.Summary.MTLSUnknown++
					}
				} else if mtls, ok := pod.Annotations["linkerd.io/tls"]; ok {
					if mtls == "optional" || mtls == "true" {
						mtlsStatus = "enabled"
						result.Summary.MTLSEnabled++
						nsStats[ns].MTLSEnabled++
					} else {
						result.Summary.MTLSUnknown++
					}
				} else {
					result.Summary.MTLSUnknown++
				}
			} else {
				result.Summary.MTLSUnknown++
			}

			entry := MeshSidecarEntry{
				PodName:      pod.Name,
				Namespace:    ns,
				SidecarName:  sidecar.Name,
				SidecarImage: sidecar.Image,
				RestartCount: restartCount,
				MTLSStatus:   mtlsStatus,
			}
			result.SidecarPods = append(result.SidecarPods, entry)

			// Check for high restart count
			if restartCount > 5 {
				result.Summary.SidecarRestarts++
				result.Issues = append(result.Issues, MeshIssue{
					PodName: pod.Name, Namespace: ns,
					Issue:    fmt.Sprintf("sidecar %s has %d restarts — check proxy logs and mesh control plane health", sidecar.Name, restartCount),
					Severity: "medium",
				})
			}

			// Check for mTLS disabled
			if mtlsStatus == "disabled" {
				result.Issues = append(result.Issues, MeshIssue{
					PodName: pod.Name, Namespace: ns,
					Issue:    "mTLS is explicitly disabled — traffic is unencrypted",
					Severity: "high",
				})
			}
		} else {
			// Pod without sidecar (only flag if mesh is installed)
			if result.Summary.HasIstio || result.Summary.HasLinkerd || result.Summary.HasConsul {
				result.Summary.PodsWithoutSidecar++
				nsStats[ns].PodsWithout++

				// Only flag if pod has multiple containers (likely a real workload, not infra)
				if len(pod.Spec.Containers) > 0 && !isSystemNS(ns) {
					result.Issues = append(result.Issues, MeshIssue{
						PodName: pod.Name, Namespace: ns,
						Issue:    "pod has no mesh sidecar — not participating in service mesh",
						Severity: "low",
					})
				}
			}
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodsWithout > result.ByNamespace[j].PodsWithout
	})

	sort.Slice(result.SidecarPods, func(i, j int) bool {
		return result.SidecarPods[i].RestartCount > result.SidecarPods[j].RestartCount
	})

	result.Summary.HealthScore = meshHealthScore(result.Summary)
	result.Recommendations = meshHealthRecommendations(result.Summary)

	return result
}

// isMeshControlPlaneNS returns true for known mesh control plane namespaces.
func isMeshControlPlaneNS(ns string) bool {
	switch strings.ToLower(ns) {
	case "istio-system", "linkerd", "linkerd-system", "consul", "consul-system":
		return true
	}
	return false
}

// isSystemNS returns true for system namespaces.
func isSystemNS(ns string) bool {
	switch strings.ToLower(ns) {
	case "kube-system", "kube-public", "kube-node-lease", "gatekeeper-system":
		return true
	}
	return false
}

// meshHealthScore calculates health score.
func meshHealthScore(s MeshHealthSummary) int {
	if !s.HasIstio && !s.HasLinkerd && !s.HasConsul {
		return 50 // neutral — no mesh installed
	}
	base := 100
	// Penalty for pods without sidecar
	if s.TotalPods > 0 {
		noSidecarPct := float64(s.PodsWithoutSidecar) / float64(s.TotalPods) * 100
		base -= int(noSidecarPct / 5)
	}
	// Penalty for mTLS disabled
	base -= s.MTLSDisabled * 10
	// Penalty for sidecar restarts
	base -= s.SidecarRestarts * 5
	// Penalty for unknown mTLS
	base -= s.MTLSUnknown / 10
	if base < 0 {
		base = 0
	}
	if base > 100 {
		base = 100
	}
	return base
}

// meshHealthRecommendations generates recommendations.
func meshHealthRecommendations(s MeshHealthSummary) []string {
	var recs []string
	if !s.HasIstio && !s.HasLinkerd && !s.HasConsul {
		recs = append(recs, "no service mesh detected — consider installing Istio or Linkerd for mTLS, traffic management, and observability")
		return recs
	}
	meshName := "Istio"
	if s.HasLinkerd {
		meshName = "Linkerd"
	} else if s.HasConsul {
		meshName = "Consul Connect"
	}
	if s.PodsWithoutSidecar > 0 {
		recs = append(recs, fmt.Sprintf("%d pods are not in the service mesh — enable sidecar injection for %s namespace annotation", s.PodsWithoutSidecar, meshName))
	}
	if s.MTLSDisabled > 0 {
		recs = append(recs, fmt.Sprintf("%d pods have mTLS disabled — enable strict mTLS mode for full traffic encryption", s.MTLSDisabled))
	}
	if s.MTLSUnknown > 0 {
		recs = append(recs, fmt.Sprintf("%d pods have unknown mTLS status — verify mesh configuration and annotations", s.MTLSUnknown))
	}
	if s.SidecarRestarts > 0 {
		recs = append(recs, fmt.Sprintf("%d sidecar containers have high restart counts — investigate proxy health and resource limits", s.SidecarRestarts))
	}
	if s.MTLSDisabled == 0 && s.SidecarRestarts == 0 && s.PodsWithoutSidecar == 0 {
		recs = append(recs, fmt.Sprintf("%s service mesh is healthy — all pods have sidecars and mTLS is properly configured", meshName))
	}
	return recs
}

// handleMeshHealth audits service mesh sidecar health and mTLS coverage.
// GET /api/product/mesh-health
func (s *Server) handleMeshHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := meshHealthAuditCore(pods.Items)
	writeJSON(w, result)
}
