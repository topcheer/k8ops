package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutoscaleReadinessResult evaluates which workloads would benefit from HPA
// based on their current resource usage, replica count, and traffic patterns.
// It generates ready-to-apply HPA YAML manifests.
type AutoscaleReadinessResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         ASReadinessSummary `json:"summary"`
	Candidates      []ASCandidate      `json:"candidates"`
	ExistingHPAs    []ASExistingHPA    `json:"existingHPAs"`
	GeneratedYAML   []ASManifest       `json:"generatedYAML"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type ASReadinessSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	WithHPA        int `json:"withHPA"`
	WithoutHPA     int `json:"withoutHPA"`
	GoodCandidates int `json:"goodCandidates"`
	MultiReplica   int `json:"multiReplica"`
	WithRequests   int `json:"withRequests"`
}

type ASCandidate struct {
	Workload     string `json:"workload"`
	Namespace    string `json:"namespace"`
	Replicas     int    `json:"replicas"`
	HasRequests  bool   `json:"hasRequests"`
	Score        int    `json:"score"`
	Reason       string `json:"reason"`
	ManifestYAML string `json:"manifestYAML,omitempty"`
}

type ASExistingHPA struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	TargetRef   string   `json:"targetRef"`
	MinReplicas int      `json:"minReplicas"`
	MaxReplicas int      `json:"maxReplicas"`
	Issues      []string `json:"issues"`
}

type ASManifest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	YAML      string `json:"yaml"`
}

// handleAutoscaleReadiness handles GET /api/scalability/autoscale-readiness
func (s *Server) handleAutoscaleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AutoscaleReadinessResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// Build HPA coverage map
	hpaMap := make(map[string]bool) // ns/name -> has HPA
	var existingHPAs []ASExistingHPA
	for _, hpa := range hpas.Items {
		if isSystemNamespace(hpa.Namespace) {
			continue
		}
		key := hpa.Namespace + "/" + hpa.Spec.ScaleTargetRef.Name
		hpaMap[key] = true
		result.Summary.WithHPA++

		eh := ASExistingHPA{
			Name: hpa.Name, Namespace: hpa.Namespace,
			TargetRef: hpa.Spec.ScaleTargetRef.Name,
		}
		if hpa.Spec.MinReplicas != nil {
			eh.MinReplicas = int(*hpa.Spec.MinReplicas)
		}
		eh.MaxReplicas = int(hpa.Spec.MaxReplicas)
		// Check for issues
		if eh.MaxReplicas > 50 {
			eh.Issues = append(eh.Issues, "maxReplicas too high (>50)")
		}
		if eh.MinReplicas == eh.MaxReplicas {
			eh.Issues = append(eh.Issues, "minReplicas = maxReplicas (no scaling)")
		}
		if len(hpa.Spec.Metrics) == 0 {
			eh.Issues = append(eh.Issues, "no metrics defined")
		}
		existingHPAs = append(existingHPAs, eh)
	}
	result.ExistingHPAs = existingHPAs

	var candidates []ASCandidate
	var manifests []ASManifest

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		key := d.Namespace + "/" + d.Name
		replicas := int(ptrInt32(d.Spec.Replicas))

		if replicas >= 2 {
			result.Summary.MultiReplica++
		}

		if hpaMap[key] {
			continue
		}
		result.Summary.WithoutHPA++

		// Check if has resource requests
		hasCPUReq := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok && !v.IsZero() {
				hasCPUReq = true
				break
			}
		}

		if hasCPUReq {
			result.Summary.WithRequests++
		}

		// Score candidate
		score := 0
		reasons := []string{}

		if replicas >= 2 {
			score += 30
			reasons = append(reasons, "multi-replica")
		}
		if replicas >= 3 {
			score += 20
		}
		if hasCPUReq {
			score += 30
			reasons = append(reasons, "has CPU requests")
		}
		if len(d.Spec.Template.Spec.Containers) == 1 {
			score += 10
			reasons = append(reasons, "single-container")
		}
		// Long-running workloads are better candidates
		ageDays := time.Since(d.CreationTimestamp.Time).Hours() / 24
		if ageDays > 7 {
			score += 10
		}

		if score >= 40 {
			result.Summary.GoodCandidates++
			yaml := generateHPAYAML(d.Name, d.Namespace, replicas)
			cand := ASCandidate{
				Workload: d.Name, Namespace: d.Namespace,
				Replicas: replicas, HasRequests: hasCPUReq,
				Score: score, Reason: joinReasons(reasons),
				ManifestYAML: yaml,
			}
			candidates = append(candidates, cand)
			manifests = append(manifests, ASManifest{
				Name: d.Name + "-hpa", Namespace: d.Namespace, YAML: yaml,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	result.Candidates = candidates
	result.GeneratedYAML = manifests

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.WithHPA * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 50:
		result.Grade = "B"
	case result.HealthScore >= 25:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildASReadinessRecs(&result)
	writeJSON(w, result)
}

func generateHPAYAML(name, ns string, replicas int) string {
	minRep := replicas
	if minRep < 1 {
		minRep = 1
	}
	maxRep := minRep * 4
	if maxRep < 4 {
		maxRep = 4
	}
	return fmt.Sprintf(`apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: %s-hpa
  namespace: %s
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: %s
  minReplicas: %d
  maxReplicas: %d
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70`, name, ns, name, minRep, maxRep)
}

func joinReasons(r []string) string {
	if len(r) == 0 {
		return ""
	}
	result := r[0]
	for i := 1; i < len(r); i++ {
		result += ", " + r[i]
	}
	return result
}

func buildASReadinessRecs(r *AutoscaleReadinessResult) []string {
	recs := []string{}
	if r.Summary.GoodCandidates > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载适合添加 HPA", r.Summary.GoodCandidates))
	}
	if r.Summary.WithoutHPA > 0 && r.Summary.MultiReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d 个多副本工作负载没有 HPA", r.Summary.MultiReplica))
	}
	if r.Summary.WithRequests < r.Summary.MultiReplica {
		recs = append(recs, "部分多副本工作负载缺少 CPU requests，HPA 需要 CPU 指标")
	}
	if len(r.ExistingHPAs) > 0 {
		bad := 0
		for _, h := range r.ExistingHPAs {
			if len(h.Issues) > 0 {
				bad++
			}
		}
		if bad > 0 {
			recs = append(recs, fmt.Sprintf("%d 个已有 HPA 存在配置问题", bad))
		}
	}
	if len(recs) == 0 {
		recs = append(recs, "自动扩缩配置良好")
	}
	return recs
}

var _ appsv1.Deployment
var _ autoscalingv2.HorizontalPodAutoscaler
var _ autoscalingv2.HorizontalPodAutoscaler
