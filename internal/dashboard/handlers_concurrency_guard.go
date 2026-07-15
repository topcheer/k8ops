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

// ConcurrencyGuardResult is the deployment concurrency & rolling update collision audit.
type ConcurrencyGuardResult struct {
	Timestamp       time.Time          `json:"timestamp"`
	Score           int                `json:"score"`
	Status          string             `json:"status"`
	Summary         ConcurrencySummary `json:"summary"`
	ActiveRollouts  []ActiveRollout    `json:"activeRollouts"`
	CollisionRisks  []CollisionRisk    `json:"collisionRisks"`
	SurgeBudget     SurgeBudget        `json:"surgeBudget"`
	SafeToDeploy    bool               `json:"safeToDeploy"`
	Blockers        []string           `json:"blockers"`
	Recommendations []string           `json:"recommendations"`
}

// ConcurrencySummary holds aggregate deployment concurrency metrics.
type ConcurrencySummary struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	ActiveRollouts     int `json:"activeRollouts"`
	CollisionRisks     int `json:"collisionRisks"`
	TotalSurgePods     int `json:"totalSurgePods"`
	MaxSurgeAcrossAll  int `json:"maxSurgeAcrossAll"`
	WorkloadsWithSurge int `json:"workloadsWithSurge"`
}

// ActiveRollout describes a workload currently in rollout.
type ActiveRollout struct {
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	UpdatedReplicas   int32  `json:"updatedReplicas"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	ReadyReplicas     int32  `json:"readyReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	MaxSurge          int    `json:"maxSurge"`
	MaxUnavailable    int    `json:"maxUnavailable"`
	Strategy          string `json:"strategy"`
	Progress          int    `json:"progress"`
}

// CollisionRisk identifies a potential resource collision during concurrent rollouts.
type CollisionRisk struct {
	Type        string `json:"type"`
	Namespace   string `json:"namespace"`
	Workloads   string `json:"workloads"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// SurgeBudget calculates the total surge capacity across all workloads.
type SurgeBudget struct {
	TotalDesiredReplicas int     `json:"totalDesiredReplicas"`
	MaxPossiblePods      int     `json:"maxPossiblePods"`
	SurgeRatio           float64 `json:"surgeRatio"`
	RiskLevel            string  `json:"riskLevel"`
}

func (s *Server) handleDeploymentConcurrencyGuard(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list deployments: %v", err))
		return
	}

	statefulSets, err := rc.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		statefulSets = &appsv1.StatefulSetList{}
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		nodes = &corev1.NodeList{}
	}

	result := analyzeDeploymentConcurrency(deployments.Items, statefulSets.Items, nodes.Items)
	writeJSON(w, result)
}

func analyzeDeploymentConcurrency(deployments []appsv1.Deployment, sts []appsv1.StatefulSet, nodes []corev1.Node) ConcurrencyGuardResult {
	now := time.Now()
	_ = now

	var activeRollouts []ActiveRollout
	var collisionRisks []CollisionRisk
	totalSurge := 0
	maxSurgeAll := 0
	totalDesired := 0
	workloadsWithSurge := 0

	// Track rollouts by namespace for collision detection
	nsRollouts := make(map[string][]string)

	for _, dep := range deployments {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		totalDesired += int(replicas)

		updated := dep.Status.UpdatedReplicas
		ready := dep.Status.ReadyReplicas
		available := dep.Status.AvailableReplicas

		// Determine if this deployment is actively rolling
		isRolling := updated < replicas || (dep.Status.Replicas > replicas)

		// Calculate surge/unavailable
		maxSurge := 1
		maxUnavailable := 0
		if dep.Spec.Strategy.RollingUpdate != nil {
			if dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
				maxSurge = int(dep.Spec.Strategy.RollingUpdate.MaxSurge.IntValue())
				if maxSurge < 0 {
					maxSurge = int(replicas) + maxSurge // percentage-based
				}
			}
			if dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
				maxUnavailable = int(dep.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue())
				if maxUnavailable < 0 {
					maxUnavailable = int(replicas) * (-maxUnavailable) / 100
				}
			}
		}

		if maxSurge > 0 {
			totalSurge += maxSurge
			workloadsWithSurge++
		}
		if maxSurge > maxSurgeAll {
			maxSurgeAll = maxSurge
		}

		progress := 100
		if replicas > 0 {
			progress = int(updated * 100 / replicas)
		}

		if isRolling {
			rollout := ActiveRollout{
				Namespace:         dep.Namespace,
				Name:              dep.Name,
				Kind:              "Deployment",
				UpdatedReplicas:   updated,
				DesiredReplicas:   replicas,
				ReadyReplicas:     ready,
				AvailableReplicas: available,
				MaxSurge:          maxSurge,
				MaxUnavailable:    maxUnavailable,
				Strategy:          string(dep.Spec.Strategy.Type),
				Progress:          progress,
			}
			activeRollouts = append(activeRollouts, rollout)
			nsRollouts[dep.Namespace] = append(nsRollouts[dep.Namespace], dep.Name)
		}
	}

	// Check StatefulSets too
	for _, ss := range sts {
		replicas := int32(1)
		if ss.Spec.Replicas != nil {
			replicas = *ss.Spec.Replicas
		}
		totalDesired += int(replicas)

		if ss.Status.UpdatedReplicas < replicas {
			progress := 100
			if replicas > 0 {
				progress = int(ss.Status.UpdatedReplicas * 100 / replicas)
			}
			activeRollouts = append(activeRollouts, ActiveRollout{
				Namespace:         ss.Namespace,
				Name:              ss.Name,
				Kind:              "StatefulSet",
				UpdatedReplicas:   ss.Status.UpdatedReplicas,
				DesiredReplicas:   replicas,
				ReadyReplicas:     ss.Status.ReadyReplicas,
				AvailableReplicas: ss.Status.AvailableReplicas,
				MaxSurge:          0,
				MaxUnavailable:    1,
				Strategy:          string(ss.Spec.UpdateStrategy.Type),
				Progress:          progress,
			})
			nsRollouts[ss.Namespace] = append(nsRollouts[ss.Namespace], ss.Name)
		}
	}

	// Detect collisions: multiple rollouts in same namespace
	for ns, names := range nsRollouts {
		if len(names) > 2 {
			collisionRisks = append(collisionRisks, CollisionRisk{
				Type:        "NamespaceConcurrency",
				Namespace:   ns,
				Workloads:   fmt.Sprintf("%d concurrent rollouts", len(names)),
				Description: fmt.Sprintf("Multiple workloads (%v) are rolling simultaneously in namespace %s; resource contention likely", names[:min(5, len(names))], ns),
				Severity:    "medium",
			})
		}
	}

	// Detect surge budget exhaustion
	schedulableNodes := 0
	for _, n := range nodes {
		if n.Spec.Unschedulable {
			continue
		}
		isReady := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if isReady {
			schedulableNodes++
		}
	}

	maxPossiblePods := totalDesired + totalSurge
	surgeRatio := 0.0
	if totalDesired > 0 {
		surgeRatio = float64(maxPossiblePods) / float64(totalDesired)
	}
	surgeRisk := "low"
	if surgeRatio > 1.5 {
		surgeRisk = "high"
	} else if surgeRatio > 1.2 {
		surgeRisk = "medium"
	}

	// Check if too many active rollouts relative to nodes
	if len(activeRollouts) > schedulableNodes && schedulableNodes > 0 {
		collisionRisks = append(collisionRisks, CollisionRisk{
			Type:        "NodeSaturation",
			Namespace:   "",
			Workloads:   fmt.Sprintf("%d active rollouts on %d nodes", len(activeRollouts), schedulableNodes),
			Description: "Number of concurrent rollouts exceeds schedulable nodes; pod scheduling may stall",
			Severity:    "high",
		})
	}

	// Sort active rollouts by progress (ascending = least progress first)
	sort.Slice(activeRollouts, func(i, j int) bool {
		return activeRollouts[i].Progress < activeRollouts[j].Progress
	})

	// Score
	score := 100
	score -= len(collisionRisks) * 10
	if surgeRisk == "high" {
		score -= 10
	}
	if len(activeRollouts) > 5 {
		score -= 5
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

	// Determine if safe to deploy
	safeToDeploy := len(activeRollouts) == 0
	var blockers []string
	if len(activeRollouts) > 0 {
		blockers = append(blockers, fmt.Sprintf("%d rollout(s) in progress; wait for completion", len(activeRollouts)))
		safeToDeploy = false
	}
	if surgeRisk == "high" {
		blockers = append(blockers, "Surge budget ratio is high; deploying now may cause scheduling pressure")
		safeToDeploy = false
	}

	// Recommendations
	var recs []string
	if len(activeRollouts) > 0 {
		recs = append(recs, fmt.Sprintf("%d active rollout(s) detected; monitor progress before triggering new deployments", len(activeRollouts)))
	}
	if len(collisionRisks) > 0 {
		recs = append(recs, fmt.Sprintf("%d collision risk(s) identified; consider staggering deployments", len(collisionRisks)))
	}
	if surgeRisk != "low" {
		recs = append(recs, fmt.Sprintf("Surge ratio %.1fx is %s; reduce maxSurge or deploy sequentially", surgeRatio, surgeRisk))
	}
	if safeToDeploy {
		recs = append(recs, "No active rollouts; deployment window is clear")
	}
	if len(recs) == 0 {
		recs = append(recs, "Deployment concurrency looks healthy; no collision risks detected")
	}

	return ConcurrencyGuardResult{
		Timestamp: time.Now(),
		Score:     score,
		Status:    status,
		Summary: ConcurrencySummary{
			TotalWorkloads:     len(deployments) + len(sts),
			ActiveRollouts:     len(activeRollouts),
			CollisionRisks:     len(collisionRisks),
			TotalSurgePods:     totalSurge,
			MaxSurgeAcrossAll:  maxSurgeAll,
			WorkloadsWithSurge: workloadsWithSurge,
		},
		ActiveRollouts:  activeRollouts,
		CollisionRisks:  collisionRisks,
		SurgeBudget:     SurgeBudget{TotalDesiredReplicas: totalDesired, MaxPossiblePods: maxPossiblePods, SurgeRatio: surgeRatio, RiskLevel: surgeRisk},
		SafeToDeploy:    safeToDeploy,
		Blockers:        blockers,
		Recommendations: recs,
	}
}
