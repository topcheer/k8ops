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

// ============================================================
// v20.43 — Product Dimension (Round 27)
// 1. Deployment Update Strategy Audit — RollingUpdate vs Recreate compliance
// 2. ConfigMap Hot Reload Detector — ConfigMap consumers without reload
// 3. Pod Disruption Readiness — multi-replica + PDB + anti-affinity score
// ============================================================

// ---------------------------------------------------------------
// 1. Deployment Update Strategy Audit
// ---------------------------------------------------------------

type StrategyResult2043 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         StrategySummary2043 `json:"summary"`
	RiskyStrategies []StrategyEntry2043 `json:"riskyStrategies"`
	Recommendations []string            `json:"recommendations"`
}

type StrategySummary2043 struct {
	TotalDeploys  int `json:"totalDeployments"`
	RollingUpdate int `json:"rollingUpdate"`
	Recreate      int `json:"recreate"`
}

type StrategyEntry2043 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
	Replicas  int32  `json:"replicas"`
}

func (s *Server) handleStrategyAudit2043(w http.ResponseWriter, r *http.Request) {
	result := StrategyResult2043{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		strategy := string(dep.Spec.Strategy.Type)
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if strategy == "RollingUpdate" || strategy == "" {
			result.Summary.RollingUpdate++
		} else if strategy == "Recreate" {
			result.Summary.Recreate++
			// Recreate causes downtime for multi-replica deployments
			if replicas > 1 {
				result.RiskyStrategies = append(result.RiskyStrategies, StrategyEntry2043{
					Name: dep.Name, Namespace: dep.Namespace,
					Strategy: "Recreate", Replicas: replicas,
				})
				score -= 5
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.RiskyStrategies, func(i, j int) bool {
		return result.RiskyStrategies[i].Replicas > result.RiskyStrategies[j].Replicas
	})

	if len(result.RiskyStrategies) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica deployments use Recreate — switch to RollingUpdate for zero downtime", len(result.RiskyStrategies)))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. ConfigMap Hot Reload Detector
// ---------------------------------------------------------------

type CMReloadResult2043 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CMReloadSummary2043 `json:"summary"`
	StaleConsumers  []CMReloadEntry2043 `json:"staleConsumers"`
	Recommendations []string            `json:"recommendations"`
}

type CMReloadSummary2043 struct {
	TotalCMs        int `json:"totalConfigMaps"`
	CMsWithConsumer int `json:"cmsWithConsumer"`
	ConsumersEnv    int `json:"envVarConsumers"`
	ConsumersVol    int `json:"volumeConsumers"`
}

type CMReloadEntry2043 struct {
	ConfigMap string `json:"configMap"`
	Namespace string `json:"namespace"`
	Consumer  string `json:"consumer"`
	MountType string `json:"mountType"`
}

func (s *Server) handleCMReloadDetector(w http.ResponseWriter, r *http.Request) {
	result := CMReloadResult2043{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalCMs = len(cmList.Items)

	consumedCMs := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Volume-mounted ConfigMaps (auto-updated by kubelet)
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				key := pod.Namespace + "/" + vol.ConfigMap.Name
				consumedCMs[key] = true
				result.Summary.ConsumersVol++
			}
		}

		// Env var ConfigMaps (requires pod restart to update)
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					key := pod.Namespace + "/" + env.ValueFrom.ConfigMapKeyRef.Name
					consumedCMs[key] = true
					result.Summary.ConsumersEnv++
					result.StaleConsumers = append(result.StaleConsumers, CMReloadEntry2043{
						ConfigMap: env.ValueFrom.ConfigMapKeyRef.Name,
						Namespace: pod.Namespace,
						Consumer:  pod.Name,
						MountType: "env",
					})
					score -= 1
				}
			}
		}
	}

	result.Summary.CMsWithConsumer = len(consumedCMs)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.ConsumersEnv > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ConfigMaps consumed via env vars — these require pod restart to update", result.Summary.ConsumersEnv))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Disruption Readiness
// ---------------------------------------------------------------

type PDBReadinessResult2043 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PDBReadinessSummary2043 `json:"summary"`
	NotReady        []PDBReadinessEntry2043 `json:"notReadyWorkloads"`
	Recommendations []string                `json:"recommendations"`
}

type PDBReadinessSummary2043 struct {
	TotalWorkloads int `json:"totalWorkloads"`
	Ready          int `json:"ready"`
	NotReady       int `json:"notReady"`
}

type PDBReadinessEntry2043 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issues    string `json:"issues"`
}

func (s *Server) handlePDBReadiness(w http.ResponseWriter, r *http.Request) {
	result := PDBReadinessResult2043{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})

	// Build PDB namespace coverage
	nsWithPDB := make(map[string]bool)
	for _, pdb := range pdbList.Items {
		nsWithPDB[pdb.Namespace] = true
	}

	for _, dep := range deployList.Items {
		result.Summary.TotalWorkloads++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if replicas <= 1 {
			continue
		}

		issues := []string{}
		if !nsWithPDB[dep.Namespace] {
			issues = append(issues, "no PDB")
		}
		if dep.Spec.Template.Spec.Affinity == nil ||
			dep.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
			if len(dep.Spec.Template.Spec.TopologySpreadConstraints) == 0 {
				issues = append(issues, "no anti-affinity")
			}
		}

		if len(issues) > 0 {
			result.Summary.NotReady++
			result.NotReady = append(result.NotReady, PDBReadinessEntry2043{
				Name: dep.Name, Namespace: dep.Namespace,
				Issues: fmt.Sprintf("%v", issues),
			})
			score -= 3
		} else {
			result.Summary.Ready++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.NotReady, func(i, j int) bool {
		return result.NotReady[i].Namespace < result.NotReady[j].Namespace
	})

	if result.Summary.NotReady > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica workloads lack disruption readiness (PDB + anti-affinity)", result.Summary.NotReady))
	}

	writeJSON(w, result)
}

// keep import
var _ = appsv1.Deployment{}
