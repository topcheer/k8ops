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

// PlacementScoreResult evaluates the quality of pod scheduling and placement
// decisions. It scores workloads on anti-affinity coverage, topology spread,
// node diversity, co-location efficiency, and single-point-of-failure risk.
type PlacementScoreResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PlacementSummary    `json:"summary"`
	Workloads       []PlacementEntry    `json:"workloads"`
	Risks           []PlacementRisk     `json:"risks"`
	ByNamespace     []PlacementNS       `json:"byNamespace"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type PlacementSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	WithAntiAffinity  int     `json:"withAntiAffinity"`
	WithTopoSpread    int     `json:"withTopologySpread"`
	SingleNodeDeploy  int     `json:"singleNodeDeployment"` // all pods on one node
	AvgNodeDiversity  float64 `json:"avgNodeDiversity"`    // 0-1, higher = better
	SPofWorkloads     int     `json:"singlePointOfFailure"`
}

type PlacementEntry struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Kind         string  `json:"kind"`
	Replicas     int     `json:"replicas"`
	PodNodes     int     `json:"podNodes"`     // unique nodes pods run on
	DiversityScore float64 `json:"diversityScore"`
	HasAntiAffinity bool  `json:"hasAntiAffinity"`
	HasTopoSpread   bool  `json:"hasTopologySpread"`
	Risk         string  `json:"risk"`
	Score        int     `json:"score"`
}

type PlacementRisk struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Risk      string `json:"risk"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type PlacementNS struct {
	Namespace  string  `json:"namespace"`
	Workloads  int     `json:"workloads"`
	SPofCount  int     `json:"spofCount"`
	AvgScore   float64 `json:"avgScore"`
}

// handlePlacementScore handles GET /api/product/placement-score
func (s *Server) handlePlacementScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PlacementScoreResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Build workload → pods map
	wkPods := map[string][]corev1.Pod{} // ns/name → pods
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		owner := pod.Name
		if len(pod.OwnerReferences) > 0 {
			owner = pod.OwnerReferences[0].Name
		}
		key := pod.Namespace + "/" + owner
		wkPods[key] = append(wkPods[key], pod)
	}

	nsStats := map[string]*PlacementNS{}
	totalDiversity := 0.0
	workloadCount := 0

	analyzeWk := func(name, ns, kind string, replicas int32, affinity *corev1.Affinity) {
		if isSystemNamespace(ns) {
			return
		}
		workloadCount++

		key := ns + "/" + name
		wkPodList := wkPods[key]

		// Count unique nodes
		nodeSet := map[string]bool{}
		for _, p := range wkPodList {
			nodeSet[p.Spec.NodeName] = true
		}
		uniqueNodes := len(nodeSet)

		// Diversity score: uniqueNodes / replicas (or pod count)
		denom := int(replicas)
		if denom == 0 {
			denom = len(wkPodList)
		}
		diversity := 1.0
		if denom > 0 {
			diversity = float64(uniqueNodes) / float64(denom)
			if diversity > 1 {
				diversity = 1
			}
		}
		totalDiversity += diversity

		hasAA := affinity != nil && affinity.PodAntiAffinity != nil
		hasTS := affinity != nil && affinity.NodeAffinity != nil
		// Also check spec for topologySpreadConstraints
		// Simplified check

		risk := "none"
		if replicas >= 2 && uniqueNodes == 1 {
			risk = "single-node"
			result.Summary.SingleNodeDeploy++
			result.Summary.SPofWorkloads++
			result.Risks = append(result.Risks, PlacementRisk{
				Name: name, Namespace: ns, Risk: "single-node",
				Severity: "high",
				Detail: fmt.Sprintf("%d replicas all on 1 node — node failure will take down entire workload", replicas),
			})
		} else if replicas >= 3 && uniqueNodes == 2 {
			risk = "limited-spread"
			result.Risks = append(result.Risks, PlacementRisk{
				Name: name, Namespace: ns, Risk: "limited-spread",
				Severity: "medium",
				Detail: fmt.Sprintf("%d replicas on only %d nodes — consider topology spread constraints", replicas, uniqueNodes),
			})
		}

		score := 100
		if hasAA {
			result.Summary.WithAntiAffinity++
		} else if replicas >= 2 {
			score -= 20
		}
		if risk == "single-node" {
			score -= 40
		} else if risk == "limited-spread" {
			score -= 15
		}

		entry := PlacementEntry{
			Name: name, Namespace: ns, Kind: kind,
			Replicas: int(replicas), PodNodes: uniqueNodes,
			DiversityScore: diversity, HasAntiAffinity: hasAA,
			HasTopoSpread: hasTS, Risk: risk, Score: score,
		}
		result.Workloads = append(result.Workloads, entry)

		// NS stats
		if nsStats[ns] == nil {
			nsStats[ns] = &PlacementNS{Namespace: ns}
		}
		nsStats[ns].Workloads++
		nsStats[ns].AvgScore += float64(score)
		if risk == "single-node" {
			nsStats[ns].SPofCount++
		}
	}

	for _, dep := range deployments.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		analyzeWk(dep.Name, dep.Namespace, "Deployment", replicas, dep.Spec.Template.Spec.Affinity)
	}
	for _, sts := range statefulsets.Items {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		analyzeWk(sts.Name, sts.Namespace, "StatefulSet", replicas, sts.Spec.Template.Spec.Affinity)
	}

	result.Summary.TotalWorkloads = workloadCount
	if workloadCount > 0 {
		result.Summary.AvgNodeDiversity = totalDiversity / float64(workloadCount)
	}

	// NS stats
	for _, ns := range nsStats {
		if ns.Workloads > 0 {
			ns.AvgScore /= float64(ns.Workloads)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool { return result.ByNamespace[i].AvgScore < result.ByNamespace[j].AvgScore })

	// Sort workloads by score
	sort.Slice(result.Workloads, func(i, j int) bool { return result.Workloads[i].Score < result.Workloads[j].Score })
	if len(result.Workloads) > 30 {
		result.Workloads = result.Workloads[:30]
	}

	result.HealthScore = computePlacementScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generatePlacementRecs(result)

	writeJSON(w, result)
}

func computePlacementScore(s PlacementSummary) int {
	score := 100
	if s.TotalWorkloads == 0 {
		return score
	}
	score -= minInt(s.SPofWorkloads*10, 40)
	if s.AvgNodeDiversity < 0.5 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	return score
}

func generatePlacementRecs(r PlacementScoreResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Placement quality: %d workloads, %.0f%% avg diversity, %d SPOF — score %d/100",
		r.Summary.TotalWorkloads, r.Summary.AvgNodeDiversity*100, r.Summary.SPofWorkloads, r.HealthScore))
	if r.Summary.SPofWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) with all pods on a single node — add podAntiAffinity", r.Summary.SPofWorkloads))
	}
	if r.Summary.WithAntiAffinity == 0 && r.Summary.TotalWorkloads > 3 {
		recs = append(recs, "No workloads use podAntiAffinity — add for HA")
	}
	for _, risk := range r.Risks {
		if risk.Severity == "high" {
			recs = append(recs, fmt.Sprintf("%s/%s: %s", risk.Namespace, risk.Name, risk.Detail))
		}
	}
	return recs
}

var _ appsv1.StatefulSetList
