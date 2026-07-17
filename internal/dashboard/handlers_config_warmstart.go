package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigWarmstartResult identifies which workloads would benefit from
// pre-warming (init containers, startup optimization, config preloading).
// It analyzes startup patterns and recommends warm-start strategies.
type ConfigWarmstartResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         WarmstartSummary `json:"summary"`
	Candidates      []WarmstartEntry `json:"candidates"`
	Optimizations   []WarmstartOpt   `json:"optimizations"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type WarmstartSummary struct {
	TotalWorkloads      int `json:"totalWorkloads"`
	WithInitContainers  int `json:"withInitContainers"`
	SlowStarters        int `json:"slowStarters"`
	WithProbes          int `json:"withProbes"`
	WarmstartCandidates int `json:"warmstartCandidates"`
}

type WarmstartEntry struct {
	Workload       string `json:"workload"`
	Namespace      string `json:"namespace"`
	Replicas       int    `json:"replicas"`
	HasInit        bool   `json:"hasInitContainers"`
	HasStartup     bool   `json:"hasStartupProbe"`
	Score          int    `json:"score"`
	Reason         string `json:"reason"`
	Recommendation string `json:"recommendation"`
}

type WarmstartOpt struct {
	Category string `json:"category"`
	Issue    string `json:"issue"`
	Action   string `json:"action"`
	Impact   string `json:"impact"`
}

// handleConfigWarmstart handles GET /api/product/config-warmstart
func (s *Server) handleConfigWarmstart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ConfigWarmstartResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod startup time map (heuristic: use pod age vs container ready time)
	podStartupMap := make(map[string]float64) // ns/wl -> seconds
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != "Running" {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		// Estimate startup time from created to first ready
		created := pod.CreationTimestamp.Time
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				startupSec := time.Since(created).Seconds()
				if startupSec > podStartupMap[key] || podStartupMap[key] == 0 {
					podStartupMap[key] = startupSec
				}
			}
		}
	}

	var candidates []WarmstartEntry
	var optimizations []WarmstartOpt

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int(ptrInt32(d.Spec.Replicas))

		hasInit := len(d.Spec.Template.Spec.InitContainers) > 0
		hasStartup := false
		hasLiveness := false
		hasReadiness := false

		for _, c := range d.Spec.Template.Spec.Containers {
			if c.StartupProbe != nil {
				hasStartup = true
			}
			if c.LivenessProbe != nil {
				hasLiveness = true
			}
			if c.ReadinessProbe != nil {
				hasReadiness = true
			}
		}

		if hasInit {
			result.Summary.WithInitContainers++
		}
		if hasLiveness && hasReadiness {
			result.Summary.WithProbes++
		}

		key := d.Namespace + "/" + d.Name
		startupTime := podStartupMap[key]
		if startupTime > 30 {
			result.Summary.SlowStarters++
		}

		// Score for warm-start candidacy
		score := 0
		reasons := []string{}

		if !hasStartup && startupTime > 30 {
			score += 30
			reasons = append(reasons, "slow startup without startupProbe")
		}
		if !hasReadiness && replicas > 1 {
			score += 25
			reasons = append(reasons, "no readinessProbe for multi-replica")
		}
		if replicas >= 3 && !hasInit {
			score += 20
			reasons = append(reasons, "multi-replica could benefit from init container")
		}
		if startupTime > 60 {
			score += 25
			reasons = append(reasons, fmt.Sprintf("startup %.0fs", startupTime))
		}

		if score >= 25 {
			result.Summary.WarmstartCandidates++
			recommendation := "Add startupProbe for slow initializers"
			if !hasReadiness {
				recommendation += ", add readinessProbe"
			}

			candidates = append(candidates, WarmstartEntry{
				Workload: d.Name, Namespace: d.Namespace,
				Replicas: replicas, HasInit: hasInit, HasStartup: hasStartup,
				Score: score, Reason: joinStrs(reasons, "; "),
				Recommendation: recommendation,
			})
		}
	}

	// Optimization suggestions
	if result.Summary.SlowStarters > 0 {
		optimizations = append(optimizations, WarmstartOpt{
			Category: "Startup",
			Issue:    fmt.Sprintf("%d workloads have slow startup", result.Summary.SlowStarters),
			Action:   "Add startupProbe to distinguish initialization from liveness",
			Impact:   "Reduces false-positive restarts during slow initialization",
		})
	}
	if result.Summary.TotalWorkloads-result.Summary.WithProbes > 5 {
		optimizations = append(optimizations, WarmstartOpt{
			Category: "Probes",
			Issue:    fmt.Sprintf("%d workloads missing probes", result.Summary.TotalWorkloads-result.Summary.WithProbes),
			Action:   "Use /api/deployment/probe-generator to add liveness+readiness probes",
			Impact:   "Enables zero-downtime rolling updates and traffic routing",
		})
	}
	if result.Summary.WithInitContainers == 0 {
		optimizations = append(optimizations, WarmstartOpt{
			Category: "Init",
			Issue:    "No workloads use init containers",
			Action:   "Consider init containers for config preloading, migration, dependency waiting",
			Impact:   "Ensures dependencies are ready before app starts",
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	result.Candidates = candidates
	result.Optimizations = optimizations

	// Score
	if result.Summary.TotalWorkloads > 0 {
		optimized := result.Summary.TotalWorkloads - result.Summary.WarmstartCandidates
		result.HealthScore = optimized * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
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

	result.Recommendations = buildWarmstartRecs(&result)
	writeJSON(w, result)
}

func buildWarmstartRecs(r *ConfigWarmstartResult) []string {
	recs := []string{}
	if r.Summary.SlowStarters > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载启动缓慢 (>30s)", r.Summary.SlowStarters))
	}
	if r.Summary.WarmstartCandidates > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载适合预热优化", r.Summary.WarmstartCandidates))
	}
	if len(r.Optimizations) > 0 {
		recs = append(recs, fmt.Sprintf("已生成 %d 个优化建议", len(r.Optimizations)))
	}
	if len(recs) == 0 {
		recs = append(recs, "工作负载启动配置良好")
	}
	return recs
}

var _ appsv1.DeploymentList
