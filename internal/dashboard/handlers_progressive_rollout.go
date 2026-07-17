package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProgressiveRolloutResult evaluates rolling update strategies for
// progressive delivery readiness: canary, blue-green, A/B testing capability.
type ProgressiveRolloutResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         ProgressiveSummary `json:"summary"`
	ByWorkload      []ProgressiveEntry `json:"byWorkload"`
	ReadyWorkloads  []ProgressiveEntry `json:"readyWorkloads"`
	ReadinessScore  int                `json:"readinessScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type ProgressiveSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	CanaryReady     int `json:"canaryReady"`
	BlueGreenReady  int `json:"blueGreenReady"`
	HasProbes       int `json:"hasProbes"`
	HasMultiReplica int `json:"hasMultiReplica"`
	HasAffinity     int `json:"hasAffinity"`
	HasServiceMesh  int `json:"hasServiceMesh"`
	MissingBlocks   int `json:"missingBlocks"`
}

type ProgressiveEntry struct {
	Workload       string   `json:"workload"`
	Namespace      string   `json:"namespace"`
	Kind           string   `json:"kind"`
	Replicas       int      `json:"replicas"`
	HasProbe       bool     `json:"hasProbe"`
	HasPDB         bool     `json:"hasPDB"`
	Strategy       string   `json:"strategy"`
	MaxSurge       int      `json:"maxSurge"`
	MaxUnavailable int      `json:"maxUnavailable"`
	CanaryReady    bool     `json:"canaryReady"`
	BlueGreenReady bool     `json:"blueGreenReady"`
	Blockers       []string `json:"blockers"`
	ReadinessPct   int      `json:"readinessPct"`
}

// handleProgressiveRollout handles GET /api/deployment/progressive-rollout
func (s *Server) handleProgressiveRollout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ProgressiveRolloutResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbMap := make(map[string]bool)
	for _, p := range pdbs.Items {
		pdbMap[p.Namespace+"/"+p.Name] = true
	}

	var entries []ProgressiveEntry
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := ProgressiveEntry{Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment"}
		replicas := 1
		if d.Spec.Replicas != nil {
			replicas = int(*d.Spec.Replicas)
		}
		entry.Replicas = replicas

		// Check probes
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				entry.HasProbe = true
				result.Summary.HasProbes++
				break
			}
		}

		// Check PDB
		entry.HasPDB = pdbMap[d.Namespace+"/"+d.Name]
		if entry.HasPDB {
			result.Summary.HasMultiReplica++ // reuse as PDB count proxy
		}

		// Strategy analysis
		entry.Strategy = string(d.Spec.Strategy.Type)
		if d.Spec.Strategy.RollingUpdate != nil {
			if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
				entry.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()
			}
			if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
				entry.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()
			}
		}

		// Multi-replica
		if replicas >= 2 {
			result.Summary.HasMultiReplica++
		}

		// Affinity
		hasAff := d.Spec.Template.Spec.Affinity != nil
		if hasAff {
			result.Summary.HasAffinity++
		}

		// Canary readiness: needs probe + multi-replica + rolling strategy
		canaryBlockers := []string{}
		if !entry.HasProbe {
			canaryBlockers = append(canaryBlockers, "missing-readiness-probe")
		}
		if replicas < 2 {
			canaryBlockers = append(canaryBlockers, "single-replica")
		}
		if entry.MaxSurge == 0 {
			canaryBlockers = append(canaryBlockers, "maxSurge=0")
		}
		entry.CanaryReady = len(canaryBlockers) == 0
		if entry.CanaryReady {
			result.Summary.CanaryReady++
		}

		// Blue-green readiness: needs multi-replica + PDB + service selector
		bgBlockers := []string{}
		if replicas < 2 {
			bgBlockers = append(bgBlockers, "single-replica")
		}
		if !entry.HasPDB {
			bgBlockers = append(bgBlockers, "missing-PDB")
		}
		entry.BlueGreenReady = len(bgBlockers) == 0
		if entry.BlueGreenReady {
			result.Summary.BlueGreenReady++
		}

		// Combine blockers
		entry.Blockers = append(canaryBlockers, bgBlockers...)
		if len(entry.Blockers) > 0 {
			result.Summary.MissingBlocks++
		}

		// Readiness percentage
		checks := 5
		passed := 0
		if entry.HasProbe {
			passed++
		}
		if replicas >= 2 {
			passed++
		}
		if entry.MaxSurge > 0 {
			passed++
		}
		if entry.HasPDB {
			passed++
		}
		if entry.Strategy == "RollingUpdate" {
			passed++
		}
		entry.ReadinessPct = passed * 100 / checks

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ReadinessPct < entries[j].ReadinessPct })
	result.ByWorkload = entries

	for _, e := range entries {
		if e.CanaryReady || e.BlueGreenReady {
			result.ReadyWorkloads = append(result.ReadyWorkloads, e)
		}
	}

	if result.Summary.TotalWorkloads > 0 {
		result.ReadinessScore = result.Summary.CanaryReady * 100 / result.Summary.TotalWorkloads
	}
	switch {
	case result.ReadinessScore >= 70:
		result.Grade = "A"
	case result.ReadinessScore >= 50:
		result.Grade = "B"
	case result.ReadinessScore >= 30:
		result.Grade = "C"
	case result.ReadinessScore >= 15:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildProgressiveRecs(&result)
	writeJSON(w, result)
}

func buildProgressiveRecs(r *ProgressiveRolloutResult) []string {
	recs := []string{
		fmt.Sprintf("渐进式发布就绪: %d 工作负载, %d Canary 就绪, %d Blue-Green 就绪", r.Summary.TotalWorkloads, r.Summary.CanaryReady, r.Summary.BlueGreenReady),
	}
	if r.Summary.MissingBlocks > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载缺少渐进式发布前提条件", r.Summary.MissingBlocks))
	}
	if r.Summary.HasProbes < r.Summary.TotalWorkloads {
		recs = append(recs, fmt.Sprintf("仅 %d/%d 有 readiness probe (渐进式发布必需)", r.Summary.HasProbes, r.Summary.TotalWorkloads))
	}
	return recs
}

var _ appsv1.Deployment
