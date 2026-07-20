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

// SidecarInjectionResult audits sidecar container injection compliance and health.
type SidecarInjectionResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         SidecarInjectSummary   `json:"summary"`
	ByWorkload      []SidecarWorkloadEntry `json:"byWorkload"`
	MissingSidecars []SidecarWorkloadEntry `json:"missingSidecars"`
	SidecarHealth   []SidecarHealthEntry   `json:"sidecarHealth"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type SidecarInjectSummary struct {
	TotalPods         int `json:"totalPods"`
	WithSidecars      int `json:"podsWithSidecars"`
	MissingMesh       int `json:"missingMeshInjection"`
	MissingLogging    int `json:"missingLoggingSidecar"`
	MissingMonitoring int `json:"missingMonitoringSidecar"`
	UnhealthySidecars int `json:"unhealthySidecars"`
}

type SidecarWorkloadEntry struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	MainContainer string   `json:"mainContainer"`
	Sidecars      []string `json:"sidecars"`
	Missing       []string `json:"missing"`
	RiskLevel     string   `json:"riskLevel"`
}

type SidecarHealthEntry struct {
	PodName     string `json:"podName"`
	Namespace   string `json:"namespace"`
	SidecarName string `json:"sidecarName"`
	Restarts    int    `json:"restarts"`
	Ready       bool   `json:"ready"`
	Issue       string `json:"issue"`
}

// Known sidecar injection annotation prefixes
var knownSidecarAnnotations = []string{
	"sidecar.istio.io",
	"linkerd.io",
	"consul.hashicorp.com",
	"fluentd",
	"prometheus.io",
	"datadog",
}

// handleSidecarInjectionAudit handles GET /api/deployment/sidecar-injection-audit
func (s *Server) handleSidecarInjectionAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SidecarInjectionResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		var mainContainer string
		var sidecars []string
		for i, c := range pod.Spec.Containers {
			if i == 0 {
				mainContainer = c.Name
			} else {
				sidecars = append(sidecars, c.Name)
			}
		}

		// Detect sidecar type by name patterns
		hasMesh := false
		hasLogging := false
		hasMonitoring := false
		for _, sc := range sidecars {
			scLower := strings.ToLower(sc)
			if strings.Contains(scLower, "istio-proxy") || strings.Contains(scLower, "envoy") ||
				strings.Contains(scLower, "linkerd") || strings.Contains(scLower, "consul") {
				hasMesh = true
			}
			if strings.Contains(scLower, "fluent") || strings.Contains(scLower, "filebeat") ||
				strings.Contains(scLower, "log") || strings.Contains(scLower, "vector") {
				hasLogging = true
			}
			if strings.Contains(scLower, "prometheus") || strings.Contains(scLower, "datadog") ||
				strings.Contains(scLower, "monitor") || strings.Contains(scLower, "otel") {
				hasMonitoring = true
			}
		}

		// Check annotations for mesh injection
		for _, ann := range knownSidecarAnnotations {
			for k := range pod.Annotations {
				if strings.HasPrefix(k, ann) {
					hasMesh = true
				}
			}
		}

		entry := SidecarWorkloadEntry{
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			MainContainer: mainContainer,
			Sidecars:      sidecars,
		}

		var missing []string
		if !hasMesh {
			missing = append(missing, "mesh-proxy")
			result.Summary.MissingMesh++
		}
		if !hasLogging {
			missing = append(missing, "log-collector")
			result.Summary.MissingLogging++
		}
		if !hasMonitoring {
			missing = append(missing, "monitoring-agent")
			result.Summary.MissingMonitoring++
		}

		entry.Missing = missing
		switch {
		case len(missing) >= 3:
			entry.RiskLevel = "high"
		case len(missing) >= 2:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if len(sidecars) > 0 {
			result.Summary.WithSidecars++
		}
		if len(missing) > 0 {
			result.MissingSidecars = append(result.MissingSidecars, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)

		// Check sidecar container health
		for _, cs := range pod.Status.ContainerStatuses {
			isSidecar := false
			for _, sc := range sidecars {
				if cs.Name == sc {
					isSidecar = true
					break
				}
			}
			if !isSidecar {
				continue
			}
			if cs.RestartCount > 0 || !cs.Ready {
				issue := ""
				if cs.RestartCount > 5 {
					issue = fmt.Sprintf("high restart count: %d", cs.RestartCount)
				} else if !cs.Ready {
					issue = "not ready"
				} else if cs.RestartCount > 0 {
					issue = fmt.Sprintf("%d restarts", cs.RestartCount)
				}
				result.SidecarHealth = append(result.SidecarHealth, SidecarHealthEntry{
					PodName:     pod.Name,
					Namespace:   pod.Namespace,
					SidecarName: cs.Name,
					Restarts:    int(cs.RestartCount),
					Ready:       cs.Ready,
					Issue:       issue,
				})
				result.Summary.UnhealthySidecars++
			}
		}
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalPods > 0 {
		withRatio := float64(result.Summary.WithSidecars) / float64(result.Summary.TotalPods)
		result.HealthScore = int(withRatio * 60)
		if result.Summary.UnhealthySidecars == 0 {
			result.HealthScore += 20
		}
		// Bonus if most pods have sidecars
		if withRatio > 0.5 {
			result.HealthScore += 20
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Sidecar 注入审计: %d Pod, %d 有 sidecar, %d 缺少 mesh, %d 缺少日志, %d 不健康",
			result.Summary.TotalPods, result.Summary.WithSidecars,
			result.Summary.MissingMesh, result.Summary.MissingLogging, result.Summary.UnhealthySidecars),
	}
	if result.Summary.MissingMesh > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 缺少 service mesh 注入", result.Summary.MissingMesh))
	}
	if result.Summary.UnhealthySidecars > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 sidecar 不健康 (重启或未就绪)", result.Summary.UnhealthySidecars))
	}
	if result.HealthScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 为关键工作负载注入 mesh proxy 和日志收集 sidecar")
	}
	writeJSON(w, result)
}
