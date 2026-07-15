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

// MeshInjectionResult is the service mesh injection coverage & namespace adoption audit.
type MeshInjectionResult struct {
	Timestamp         time.Time             `json:"timestamp"`
	Score             int                   `json:"score"`
	Status            string                `json:"status"`
	Summary           MeshInjSummary        `json:"summary"`
	MeshType          string                `json:"meshType"`
	NamespaceCoverage []MeshInjNSStat       `json:"namespaceCoverage"`
	InjectionGaps     []InjectionGap        `json:"injectionGaps"`
	UnmeshedPods      []UnmeshedPodInfo     `json:"unmeshedPods"`
	MeshNamespaces    []MeshNamespaceDetail `json:"meshNamespaces"`
	Issues            []MeshInjIssue        `json:"issues"`
	Recommendations   []string              `json:"recommendations"`
}

// MeshInjSummary holds aggregate mesh injection metrics.
type MeshInjSummary struct {
	TotalNamespaces       int     `json:"totalNamespaces"`
	MeshEnabledNamespaces int     `json:"meshEnabledNamespaces"`
	TotalPods             int     `json:"totalPods"`
	InjectedPods          int     `json:"injectedPods"`
	UninjectedPods        int     `json:"uninjectedPods"`
	InjectionRate         float64 `json:"injectionRate"`
	OptedOutPods          int     `json:"optedOutPods"`
	MeshDetected          bool    `json:"meshDetected"`
}

// MeshInjNSStat shows mesh adoption per namespace.
type MeshInjNSStat struct {
	Namespace        string  `json:"namespace"`
	TotalPods        int     `json:"totalPods"`
	InjectedPods     int     `json:"injectedPods"`
	InjectionRate    float64 `json:"injectionRate"`
	InjectionEnabled bool    `json:"injectionEnabled"`
	Status           string  `json:"status"`
}

// InjectionGap identifies a pod missing a sidecar despite namespace injection being enabled.
type InjectionGap struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// UnmeshedPodInfo describes a pod without mesh sidecar.
type UnmeshedPodInfo struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Kind      string `json:"kind"`
}

// MeshNamespaceDetail shows mesh config for a namespace.
type MeshNamespaceDetail struct {
	Namespace        string            `json:"namespace"`
	InjectionEnabled bool              `json:"injectionEnabled"`
	Labels           map[string]string `json:"labels"`
	MeshType         string            `json:"meshType"`
	PodCount         int               `json:"podCount"`
	InjectedCount    int               `json:"injectedCount"`
}

// MeshInjIssue identifies a mesh adoption issue.
type MeshInjIssue struct {
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Namespace string `json:"namespace"`
	Message   string `json:"message"`
}

// mesh sidecar container name patterns
var meshSidecarNames = map[string]bool{
	"istio-proxy":    true,
	"linkerd-proxy":  true,
	"envoy":          true,
	"consul-connect": true,
	"sidecar-proxy":  true,
}

func (s *Server) handleMeshInjection(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list namespaces: %v", err))
		return
	}

	result := analyzeMeshInjection(pods.Items, namespaces.Items)
	writeJSON(w, result)
}

func analyzeMeshInjection(pods []corev1.Pod, namespaces []corev1.Namespace) MeshInjectionResult {
	now := time.Now()

	// Determine mesh type from namespace labels
	meshType := "none"
	meshDetected := false

	// Build namespace injection map
	nsMeshEnabled := make(map[string]bool)
	nsLabels := make(map[string]map[string]string)

	for _, ns := range namespaces {
		nsLabels[ns.Name] = ns.Labels
		enabled := false

		// Istio injection
		if v, ok := ns.Labels["istio-injection"]; ok && v == "enabled" {
			enabled = true
			meshType = "istio"
			meshDetected = true
		}
		// Linkerd injection
		if v, ok := ns.Annotations["linkerd.io/inject"]; ok && v == "enabled" {
			enabled = true
			if meshType == "none" {
				meshType = "linkerd"
			}
			meshDetected = true
		}
		// Consul injection
		if v, ok := ns.Annotations["consul.hashicorp.com/connect-inject"]; ok && v == "true" {
			enabled = true
			if meshType == "none" {
				meshType = "consul"
			}
			meshDetected = true
		}

		nsMeshEnabled[ns.Name] = enabled
	}

	// Analyze pods
	summary := MeshInjSummary{
		TotalNamespaces: len(namespaces),
		MeshDetected:    meshDetected,
	}
	nsPodCount := make(map[string]int)
	nsInjectedCount := make(map[string]int)
	var injectionGaps []InjectionGap
	var unmeshedPods []UnmeshedPodInfo
	var issues []MeshInjIssue

	skipNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
		"k8ops-system":    true,
		"istio-system":    true,
		"linkerd":         true,
		"linkerd-system":  true,
		"consul-system":   true,
		"metallb-system":  true,
		"ingress-nginx":   true,
		"calico-system":   true,
		"tigera-system":   true,
		"longhorn-system": true,
		"cert-manager":    true,
	}

	for _, pod := range pods {
		if skipNamespaces[pod.Namespace] {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		summary.TotalPods++
		nsPodCount[pod.Namespace]++

		// Check if pod has sidecar
		hasSidecar := false
		for _, c := range pod.Spec.Containers {
			if meshSidecarNames[c.Name] || meshSidecarNames[strings.ToLower(c.Name)] {
				hasSidecar = true
				break
			}
			// Also check by image name
			imgLower := strings.ToLower(c.Image)
			if strings.Contains(imgLower, "istio/proxy") || strings.Contains(imgLower, "linkerd/proxy") ||
				strings.Contains(imgLower, "envoyproxy") || strings.Contains(imgLower, "consul-connect") {
				hasSidecar = true
				break
			}
		}

		// Check opt-out annotation
		optedOut := false
		if v, ok := pod.Annotations["sidecar.istio.io/inject"]; ok && v == "false" {
			optedOut = true
		}
		if v, ok := pod.Annotations["linkerd.io/inject"]; ok && v == "disabled" {
			optedOut = true
		}

		if hasSidecar {
			summary.InjectedPods++
			nsInjectedCount[pod.Namespace]++
		} else {
			summary.UninjectedPods++
			if optedOut {
				summary.OptedOutPods++
			}

			unmeshedPods = append(unmeshedPods, UnmeshedPodInfo{
				Namespace: pod.Namespace,
				Pod:       pod.Name,
				Kind:      "Pod",
			})

			// Injection gap: namespace has injection enabled but pod doesn't have sidecar
			if nsMeshEnabled[pod.Namespace] && !optedOut {
				severity := "high"
				injectionGaps = append(injectionGaps, InjectionGap{
					Namespace: pod.Namespace,
					Pod:       pod.Name,
					Reason:    "Namespace has mesh injection enabled but pod has no sidecar",
					Severity:  severity,
				})
				issues = append(issues, MeshInjIssue{
					Type:      "InjectionGap",
					Severity:  "high",
					Namespace: pod.Namespace,
					Message:   fmt.Sprintf("Pod %s in namespace %s (injection enabled) is missing mesh sidecar", pod.Name, pod.Namespace),
				})
			}
		}
	}

	if summary.TotalPods > 0 {
		summary.InjectionRate = float64(summary.InjectedPods) / float64(summary.TotalPods) * 100
	}

	// Count mesh-enabled namespaces
	for _, ns := range namespaces {
		if nsMeshEnabled[ns.Name] {
			summary.MeshEnabledNamespaces++
		}
	}

	// Build namespace stats
	var nsStats []MeshInjNSStat
	var meshNSDetails []MeshNamespaceDetail
	for _, ns := range namespaces {
		if skipNamespaces[ns.Name] {
			continue
		}
		total := nsPodCount[ns.Name]
		injected := nsInjectedCount[ns.Name]
		rate := 0.0
		if total > 0 {
			rate = float64(injected) / float64(total) * 100
		}
		status := "no-mesh"
		if nsMeshEnabled[ns.Name] {
			if total == 0 {
				status = "empty"
			} else if rate == 100 {
				status = "fully-meshed"
			} else if rate > 0 {
				status = "partial-mesh"
			} else {
				status = "injection-enabled-but-no-pods-injected"
			}
		} else if total > 0 {
			status = "unmeshed"
		}

		nsStats = append(nsStats, MeshInjNSStat{
			Namespace:        ns.Name,
			TotalPods:        total,
			InjectedPods:     injected,
			InjectionRate:    rate,
			InjectionEnabled: nsMeshEnabled[ns.Name],
			Status:           status,
		})

		if nsMeshEnabled[ns.Name] || total > 0 {
			meshNSDetails = append(meshNSDetails, MeshNamespaceDetail{
				Namespace:        ns.Name,
				InjectionEnabled: nsMeshEnabled[ns.Name],
				Labels:           nsLabels[ns.Name],
				MeshType:         meshType,
				PodCount:         total,
				InjectedCount:    injected,
			})
		}
	}
	sort.Slice(nsStats, func(i, j int) bool {
		return nsStats[i].InjectionRate < nsStats[j].InjectionRate
	})

	// Score
	score := 100
	if summary.TotalPods > 0 {
		score = int(summary.InjectionRate)
	}
	score -= len(injectionGaps) * 3
	if !meshDetected {
		score = 100 // no mesh detected, can't penalize
	}
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if !meshDetected {
		recs = append(recs, "No service mesh detected; consider adopting Istio/Linkerd for mTLS, traffic management, and observability")
	} else {
		if summary.MeshEnabledNamespaces == 0 {
			recs = append(recs, fmt.Sprintf("%s detected but no namespaces have injection enabled; enable injection in application namespaces", meshType))
		}
		if len(injectionGaps) > 0 {
			recs = append(recs, fmt.Sprintf("%d pod(s) in injection-enabled namespaces are missing sidecars; check pod annotations and namespace labels", len(injectionGaps)))
		}
		if summary.InjectionRate < 50 && summary.TotalPods > 5 {
			recs = append(recs, fmt.Sprintf("Mesh injection rate is %.0f%%; expand mesh coverage to more namespaces for consistent mTLS and traffic control", summary.InjectionRate))
		}
		if summary.OptedOutPods > 0 {
			recs = append(recs, fmt.Sprintf("%d pod(s) explicitly opted out of mesh injection; verify this is intentional", summary.OptedOutPods))
		}
	}
	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("Mesh injection coverage looks healthy at %.0f%% across %d namespaces", summary.InjectionRate, summary.MeshEnabledNamespaces))
	}

	return MeshInjectionResult{
		Timestamp:         now,
		Score:             score,
		Status:            status,
		Summary:           summary,
		MeshType:          meshType,
		NamespaceCoverage: nsStats,
		InjectionGaps:     injectionGaps,
		UnmeshedPods:      unmeshedPods,
		MeshNamespaces:    meshNSDetails,
		Issues:            issues,
		Recommendations:   recs,
	}
}
