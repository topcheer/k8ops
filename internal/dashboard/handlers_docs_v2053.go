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
// v20.53 — Documentation Dimension (Round 28)
// 1. Resource Age Timeline — resource creation timeline doc
// 2. Node Label Standardization — node label consistency report
// 3. Cluster Component Inventory — k8s component version catalog
// ============================================================

// ---------------------------------------------------------------
// 1. Resource Age Timeline
// ---------------------------------------------------------------

type AgeTimelineResult2053 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         AgeTimelineSummary2053 `json:"summary"`
	Timeline        []AgeTimelineEntry2053 `json:"timeline"`
	Recommendations []string               `json:"recommendations"`
}

type AgeTimelineSummary2053 struct {
	TotalResources int `json:"totalResources"`
	Old            int `json:"olderThan1Year"`
	Recent         int `json:"newerThan1Week"`
}

type AgeTimelineEntry2053 struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
}

func (s *Server) handleAgeTimeline(w http.ResponseWriter, r *http.Request) {
	result := AgeTimelineResult2053{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()

	for _, dep := range deployList.Items {
		ageDays := int(now.Sub(dep.CreationTimestamp.Time).Hours() / 24)
		entry := AgeTimelineEntry2053{Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace, AgeDays: ageDays}
		result.Summary.TotalResources++
		if ageDays > 365 {
			result.Summary.Old++
		} else if ageDays < 7 {
			result.Summary.Recent++
		}
		if ageDays > 365 {
			result.Timeline = append(result.Timeline, entry)
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Timeline, func(i, j int) bool {
		return result.Timeline[i].AgeDays > result.Timeline[j].AgeDays
	})

	if result.Summary.Old > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d resources older than 1 year — review lifecycle", result.Summary.Old))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Label Standardization
// ---------------------------------------------------------------

type NodeLabelResult2053 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeLabelSummary2053 `json:"summary"`
	Inconsistent    []NodeLabelEntry2053 `json:"inconsistentNodes"`
	Recommendations []string             `json:"recommendations"`
}

type NodeLabelSummary2053 struct {
	TotalNodes      int `json:"totalNodes"`
	UniqueLabelKeys int `json:"uniqueLabelKeys"`
	Inconsistent    int `json:"inconsistentNodes"`
}

type NodeLabelEntry2053 struct {
	Node string `json:"node"`
	Keys int    `json:"labelCount"`
}

func (s *Server) handleNodeLabelStd(w http.ResponseWriter, r *http.Request) {
	result := NodeLabelResult2053{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	allKeys := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		labelCount := len(node.Labels)

		for k := range node.Labels {
			allKeys[k]++
		}

		// Flag nodes with very few labels (missing standard labels)
		if labelCount < 10 {
			result.Summary.Inconsistent++
			result.Inconsistent = append(result.Inconsistent, NodeLabelEntry2053{
				Node: node.Name, Keys: labelCount,
			})
			score -= 5
		}
	}

	result.Summary.UniqueLabelKeys = len(allKeys)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Inconsistent > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have fewer than 10 labels — add standard labels for scheduling", result.Summary.Inconsistent))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Component Inventory
// ------------------------------------------------===========

type ClusterCompResult2053 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ClusterCompSummary2053 `json:"summary"`
	Components      []ClusterCompEntry2053 `json:"components"`
	Recommendations []string               `json:"recommendations"`
}

type ClusterCompSummary2053 struct {
	K8sVersion string `json:"k8sVersion"`
	NodeCount  int    `json:"nodeCount"`
	PodCount   int    `json:"podCount"`
	NsCount    int    `json:"namespaceCount"`
}

type ClusterCompEntry2053 struct {
	Component string `json:"component"`
	Version   string `json:"version"`
}

func (s *Server) handleClusterCompInv(w http.ResponseWriter, r *http.Request) {
	result := ClusterCompResult2053{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	result.Summary.NodeCount = len(nodeList.Items)
	result.Summary.PodCount = len(podList.Items)
	result.Summary.NsCount = len(nsList.Items)

	// Get k8s version from first node
	if len(nodeList.Items) > 0 {
		result.Summary.K8sVersion = nodeList.Items[0].Status.NodeInfo.KubeletVersion

		// Component versions
		result.Components = append(result.Components, ClusterCompEntry2053{
			Component: "kubelet", Version: nodeList.Items[0].Status.NodeInfo.KubeletVersion,
		})
		result.Components = append(result.Components, ClusterCompEntry2053{
			Component: "containerRuntime", Version: nodeList.Items[0].Status.NodeInfo.ContainerRuntimeVersion,
		})
		result.Components = append(result.Components, ClusterCompEntry2053{
			Component: "osImage", Version: nodeList.Items[0].Status.NodeInfo.OSImage,
		})
		result.Components = append(result.Components, ClusterCompEntry2053{
			Component: "kernelVersion", Version: nodeList.Items[0].Status.NodeInfo.KernelVersion,
		})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	writeJSON(w, result)
}

// keep import
var _ = corev1.Pod{}
