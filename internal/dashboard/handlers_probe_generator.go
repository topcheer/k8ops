package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProbeGeneratorResult generates health probe patch commands for containers
// that are missing liveness or readiness probes. Provides strategic
// probe patch JSON for immediate application.
type ProbeGeneratorResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         ProbeGenSummary `json:"summary"`
	Generated       []ProbePatch    `json:"generated"`
	BatchApply      []string        `json:"batchApply"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type ProbeGenSummary struct {
	TotalContainers  int `json:"totalContainers"`
	WithLiveness     int `json:"withLiveness"`
	WithReadiness    int `json:"withReadiness"`
	MissingBoth      int `json:"missingBoth"`
	MissingLiveness  int `json:"missingLiveness"`
	MissingReadiness int `json:"missingReadiness"`
}

type ProbePatch struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Kind      string `json:"kind"`
	Missing   string `json:"missing"` // liveness, readiness, both
	PatchJSON string `json:"patchJSON"`
	Command   string `json:"command"`
}

// handleProbeGenerator handles GET /api/deployment/probe-generator
func (s *Server) handleProbeGenerator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ProbeGeneratorResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var patches []ProbePatch
	var batchCmds []string

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			hasLive := c.LivenessProbe != nil
			hasReady := c.ReadinessProbe != nil

			if hasLive {
				result.Summary.WithLiveness++
			}
			if hasReady {
				result.Summary.WithReadiness++
			}

			if hasLive && hasReady {
				continue
			}

			missing := "both"
			if !hasLive && hasReady {
				missing = "liveness"
				result.Summary.MissingLiveness++
			} else if hasLive && !hasReady {
				missing = "readiness"
				result.Summary.MissingReadiness++
			} else {
				result.Summary.MissingBoth++
			}

			// Determine port from container
			port := 8080
			if len(c.Ports) > 0 {
				port = int(c.Ports[0].ContainerPort)
			}

			patchJSON := generateProbePatchJSON(c.Name, missing, port)
			cmd := fmt.Sprintf("kubectl patch deployment %s -n %s --type=strategic -p '%s'", d.Name, d.Namespace, patchJSON)

			patches = append(patches, ProbePatch{
				Workload: d.Name, Namespace: d.Namespace,
				Container: c.Name, Kind: "Deployment",
				Missing: missing, PatchJSON: patchJSON, Command: cmd,
			})
			batchCmds = append(batchCmds, cmd)
		}
	}

	if result.Summary.TotalContainers > 0 {
		withBoth := result.Summary.TotalContainers - result.Summary.MissingBoth - result.Summary.MissingLiveness - result.Summary.MissingReadiness
		result.HealthScore = withBoth * 100 / result.Summary.TotalContainers
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	sort.Slice(patches, func(i, j int) bool {
		return patches[i].Missing == "both" && patches[j].Missing != "both"
	})

	result.Generated = patches
	result.BatchApply = batchCmds
	result.Recommendations = buildProbeGenRecs(&result)
	writeJSON(w, result)
}

func generateProbePatchJSON(container, missing string, port int) string {
	parts := ""
	if missing == "liveness" || missing == "both" {
		parts += fmt.Sprintf(`"livenessProbe":{"httpGet":{"path":"/healthz","port":%d},"initialDelaySeconds":15,"periodSeconds":10}`, port)
	}
	if missing == "readiness" || missing == "both" {
		if parts != "" {
			parts += ","
		}
		parts += fmt.Sprintf(`"readinessProbe":{"httpGet":{"path":"/ready","port":%d},"initialDelaySeconds":5,"periodSeconds":5}`, port)
	}
	return fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s",%s}]}}}}`, container, parts)
}

func buildProbeGenRecs(r *ProbeGeneratorResult) []string {
	recs := []string{}
	if r.Summary.MissingBoth > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器完全缺少探针", r.Summary.MissingBoth))
	}
	if r.Summary.MissingLiveness > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器缺少 livenessProbe", r.Summary.MissingLiveness))
	}
	if r.Summary.MissingReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器缺少 readinessProbe", r.Summary.MissingReadiness))
	}
	if len(recs) == 0 {
		recs = append(recs, "所有容器都配置了探针")
		return recs
	}
	recs = append(recs, "注意: 生成的探针路径为默认值 (/healthz, /ready)，需根据应用实际端点调整")
	return recs
}

var _ appsv1.Deployment
var _ corev1.Container
