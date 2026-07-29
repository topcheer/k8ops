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
// v20.65 — Documentation Dimension (Round 30)
// 1. PV Reclaim Policy Catalog — PV reclaim policy documentation
// 2. Service Account Inventory — SA usage and token age catalog
// 3. Node Condition Timeline — node condition change history doc
// ============================================================

type PVReclaimResult2065 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVReclaimSummary2065 `json:"summary"`
	PVs             []PVReclaimEntry2065 `json:"pvs"`
	Recommendations []string             `json:"recommendations"`
}

type PVReclaimSummary2065 struct {
	TotalPVs  int `json:"totalPVs"`
	RetainPVs int `json:"retainPVs"`
	DeletePVs int `json:"deletePVs"`
}

type PVReclaimEntry2065 struct {
	Name          string `json:"name"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	Phase         string `json:"phase"`
	Capacity      string `json:"capacity"`
}

func (s *Server) handlePVReclaimCatalog(w http.ResponseWriter, r *http.Request) {
	result := PVReclaimResult2065{ScannedAt: time.Now()}
	score := 100

	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})

	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		policy := string(pv.Spec.PersistentVolumeReclaimPolicy)
		if policy == "Retain" {
			result.Summary.RetainPVs++
		} else if policy == "Delete" {
			result.Summary.DeletePVs++
		}
		result.PVs = append(result.PVs, PVReclaimEntry2065{
			Name: pv.Name, ReclaimPolicy: policy,
			Phase:    string(pv.Status.Phase),
			Capacity: pv.Spec.Capacity.Storage().String(),
		})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PVs, func(i, j int) bool { return result.PVs[i].ReclaimPolicy < result.PVs[j].ReclaimPolicy })

	if result.Summary.RetainPVs > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVs with Retain policy — review for cleanup", result.Summary.RetainPVs))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Account Inventory
// ---------------------------------------------------------------

type SAInvResult2065 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         SAInvSummary2065 `json:"summary"`
	SAs             []SAInvEntry2065 `json:"serviceAccounts"`
	Recommendations []string         `json:"recommendations"`
}

type SAInvSummary2065 struct {
	TotalSAs   int `json:"totalServiceAccounts"`
	UnusedSAs  int `json:"unusedSAs"`
	DefaultSAs int `json:"defaultSAs"`
}

type SAInvEntry2065 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
}

func (s *Server) handleSAInventory2065(w http.ResponseWriter, r *http.Request) {
	result := SAInvResult2065{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	usedSAs := make(map[string]bool)
	for _, pod := range podList.Items {
		key := pod.Namespace + "/" + pod.Spec.ServiceAccountName
		usedSAs[key] = true
	}

	now := time.Now()
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		key := sa.Namespace + "/" + sa.Name

		if sa.Name == "default" {
			result.Summary.DefaultSAs++
		}
		if !usedSAs[key] && sa.Name != "default" {
			result.Summary.UnusedSAs++
			score -= 1
		}

		ageDays := int(now.Sub(sa.CreationTimestamp.Time).Hours() / 24)
		result.SAs = append(result.SAs, SAInvEntry2065{
			Name: sa.Name, Namespace: sa.Namespace, AgeDays: ageDays,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.SAs, func(i, j int) bool { return result.SAs[i].AgeDays > result.SAs[j].AgeDays })

	if result.Summary.UnusedSAs > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unused service accounts — clean up", result.Summary.UnusedSAs))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Condition Timeline
// ------------------------------------------------===============

type NodeCondTLResult2065 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeCondTLSummary2065 `json:"summary"`
	Nodes           []NodeCondTLEntry2065 `json:"nodes"`
	Recommendations []string              `json:"recommendations"`
}

type NodeCondTLSummary2065 struct {
	TotalNodes     int `json:"totalNodes"`
	NodesWithConds int `json:"nodesWithConditions"`
}

type NodeCondTLEntry2065 struct {
	Node       string   `json:"node"`
	Conditions []string `json:"activeConditions"`
	Ready      bool     `json:"ready"`
}

func (s *Server) handleNodeCondTimeline(w http.ResponseWriter, r *http.Request) {
	result := NodeCondTLResult2065{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		conds := []string{}
		ready := true

		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ready = cond.Status == corev1.ConditionTrue
			}
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				conds = append(conds, string(cond.Type))
			}
		}

		if len(conds) > 0 {
			result.Summary.NodesWithConds++
			score -= 5
		}

		result.Nodes = append(result.Nodes, NodeCondTLEntry2065{
			Node: node.Name, Conditions: conds, Ready: ready,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NodesWithConds > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have active conditions — monitor health", result.Summary.NodesWithConds))
	}
	writeJSON(w, result)
}
