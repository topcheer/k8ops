package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.60 — Scalability & HA Dimension (Round 29)
// 1. Replica Availability Budget — desired vs ready replicas gap
// 2. Workload Distribution Score — pods per deployment balance
// 3. Failover Readiness — controller manager & scheduler HA
// ============================================================

type ReplicaAvailResult2060 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ReplicaAvailSummary2060 `json:"summary"`
	UnderReplicated []ReplicaAvailEntry2060 `json:"underReplicated"`
	Recommendations []string                `json:"recommendations"`
}

type ReplicaAvailSummary2060 struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	FullyAvailable  int `json:"fullyAvailable"`
	UnderReplicated int `json:"underReplicated"`
}

type ReplicaAvailEntry2060 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Desired   int32  `json:"desired"`
	Ready     int32  `json:"ready"`
}

func (s *Server) handleReplicaAvailBudget(w http.ResponseWriter, r *http.Request) {
	result := ReplicaAvailResult2060{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalWorkloads++
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		ready := dep.Status.ReadyReplicas

		if ready >= desired {
			result.Summary.FullyAvailable++
		} else {
			result.Summary.UnderReplicated++
			result.UnderReplicated = append(result.UnderReplicated, ReplicaAvailEntry2060{
				Name: dep.Name, Namespace: dep.Namespace, Desired: desired, Ready: ready,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.UnderReplicated, func(i, j int) bool {
		return result.UnderReplicated[i].Namespace < result.UnderReplicated[j].Namespace
	})

	if result.Summary.UnderReplicated > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d workloads under-replicated — check pod health", result.Summary.UnderReplicated))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Workload Distribution Score
// ---------------------------------------------------------------

type WkldDistResult2060 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         WkldDistSummary2060 `json:"summary"`
	Unbalanced      []WkldDistEntry2060 `json:"unbalancedNodes"`
	Recommendations []string            `json:"recommendations"`
}

type WkldDistSummary2060 struct {
	TotalNodes     int `json:"totalNodes"`
	TotalPods      int `json:"totalPods"`
	AvgPodsPerNode int `json:"avgPodsPerNode"`
	Unbalanced     int `json:"unbalancedNodes"`
}

type WkldDistEntry2060 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

func (s *Server) handleWkldDistScore(w http.ResponseWriter, r *http.Request) {
	result := WkldDistResult2060{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}

	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]
		if result.Summary.AvgPodsPerNode > 0 {
			ratio := float64(podCount) / float64(result.Summary.AvgPodsPerNode)
			if ratio > 1.5 || ratio < 0.5 {
				result.Summary.Unbalanced++
				result.Unbalanced = append(result.Unbalanced, WkldDistEntry2060{
					Node: node.Name, PodCount: podCount,
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
	sort.Slice(result.Unbalanced, func(i, j int) bool {
		return result.Unbalanced[i].PodCount > result.Unbalanced[j].PodCount
	})

	if result.Summary.Unbalanced > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have unbalanced workload — check scheduler", result.Summary.Unbalanced))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Failover Readiness
// ---------------------------------------------------------------

type FailoverResult2060 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         FailoverSummary2060 `json:"summary"`
	Components      []FailoverEntry2060 `json:"components"`
	Recommendations []string            `json:"recommendations"`
}

type FailoverSummary2060 struct {
	CMReplicas        int `json:"controllerManagerReplicas"`
	SchedReplicas     int `json:"schedulerReplicas"`
	EtcdReplicas      int `json:"etcdReplicas"`
	TotalHAComponents int `json:"totalHAComponents"`
}

type FailoverEntry2060 struct {
	Component string `json:"component"`
	Replicas  int    `json:"replicas"`
	HA        bool   `json:"ha"`
}

func (s *Server) handleFailoverReadiness(w http.ResponseWriter, r *http.Request) {
	result := FailoverResult2060{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("kube-system").List(r.Context(), metav1.ListOptions{})

	cmCount := 0
	schedCount := 0
	etcdCount := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		name := pod.Name
		if containsStr2039(name, "kube-controller-manager") {
			cmCount++
		}
		if containsStr2039(name, "kube-scheduler") {
			schedCount++
		}
		if containsStr2039(name, "etcd") {
			etcdCount++
		}
	}

	result.Summary.CMReplicas = cmCount
	result.Summary.SchedReplicas = schedCount
	result.Summary.EtcdReplicas = etcdCount

	result.Components = append(result.Components, FailoverEntry2060{Component: "kube-controller-manager", Replicas: cmCount, HA: cmCount > 1})
	result.Components = append(result.Components, FailoverEntry2060{Component: "kube-scheduler", Replicas: schedCount, HA: schedCount > 1})
	result.Components = append(result.Components, FailoverEntry2060{Component: "etcd", Replicas: etcdCount, HA: etcdCount >= 3})

	if cmCount > 1 {
		result.Summary.TotalHAComponents++
	}
	if schedCount > 1 {
		result.Summary.TotalHAComponents++
	}
	if etcdCount >= 3 {
		result.Summary.TotalHAComponents++
	}

	if result.Summary.TotalHAComponents < 2 {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.TotalHAComponents < 3 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/3 HA components — single-node cluster lacks failover", result.Summary.TotalHAComponents))
	}
	writeJSON(w, result)
}
