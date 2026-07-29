package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.48 — Scalability & HA Dimension (Round 27)
// 1. Autoscale Behavior Analyzer — HPA scaling behavior policy audit
// 2. Node Pool Diversification — node SKU/zone distribution
// 3. CSI Driver Capacity — storage driver provisioner coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Autoscale Behavior Analyzer
// ---------------------------------------------------------------

type AutoBehaviorResult2048 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         AutoBehaviorSummary2048 `json:"summary"`
	NoBehavior      []AutoBehaviorEntry2048 `json:"noBehaviorHPAs"`
	Recommendations []string                `json:"recommendations"`
}

type AutoBehaviorSummary2048 struct {
	TotalHPAs    int `json:"totalHPAs"`
	WithBehavior int `json:"withBehavior"`
	NoBehavior   int `json:"noBehavior"`
	WithPolicies int `json:"withPolicies"`
}

type AutoBehaviorEntry2048 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleAutoBehavior(w http.ResponseWriter, r *http.Request) {
	result := AutoBehaviorResult2048{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++

		if hpa.Spec.Behavior != nil {
			result.Summary.WithBehavior++
			if len(hpa.Spec.Behavior.ScaleUp.Policies) > 0 || len(hpa.Spec.Behavior.ScaleDown.Policies) > 0 {
				result.Summary.WithPolicies++
			}
		} else {
			result.Summary.NoBehavior++
			result.NoBehavior = append(result.NoBehavior, AutoBehaviorEntry2048{
				Name: hpa.Name, Namespace: hpa.Namespace,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NoBehavior > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d HPAs without scaling behavior — add policies to control scaling velocity", result.Summary.NoBehavior))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Pool Diversification
// ---------------------------------------------------------------

type NodePoolResult2048 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodePoolSummary2048 `json:"summary"`
	NodeTypes       []NodePoolEntry2048 `json:"nodeTypes"`
	Recommendations []string            `json:"recommendations"`
}

type NodePoolSummary2048 struct {
	TotalNodes  int `json:"totalNodes"`
	UniqueTypes int `json:"uniqueNodeTypes"`
	Zones       int `json:"zones"`
	SingleZone  int `json:"singleZoneNodes"`
}

type NodePoolEntry2048 struct {
	InstanceType string `json:"instanceType"`
	Count        int    `json:"count"`
	Zone         string `json:"zone"`
}

func (s *Server) handleNodePoolDivers(w http.ResponseWriter, r *http.Request) {
	result := NodePoolResult2048{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	typeMap := make(map[string]int)
	zoneSet := make(map[string]bool)
	nodeInZone := make(map[string]int)

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		instanceType := node.Status.NodeInfo.MachineID
		if instanceType == "" {
			instanceType = "unknown"
		}
		// Use node labels for instance type
		if t := node.Labels["node.kubernetes.io/instance-type"]; t != "" {
			instanceType = t
		} else if t := node.Labels["beta.kubernetes.io/instance-type"]; t != "" {
			instanceType = t
		} else {
			instanceType = "self-managed"
		}

		typeMap[instanceType]++

		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = node.Labels["failure-domain.beta.kubernetes.io/zone"]
		}
		if zone != "" {
			zoneSet[zone] = true
			nodeInZone[zone]++
		}
	}

	result.Summary.UniqueTypes = len(typeMap)
	result.Summary.Zones = len(zoneSet)

	for t, c := range typeMap {
		result.NodeTypes = append(result.NodeTypes, NodePoolEntry2048{
			InstanceType: t, Count: c,
		})
	}

	// Single-zone risk
	if len(zoneSet) <= 1 && result.Summary.TotalNodes > 1 {
		result.Summary.SingleZone = result.Summary.TotalNodes
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.NodeTypes, func(i, j int) bool {
		return result.NodeTypes[i].Count > result.NodeTypes[j].Count
	})

	if result.Summary.SingleZone > 0 {
		result.Recommendations = append(result.Recommendations,
			"All nodes in single zone — add multi-zone nodes for HA")
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. CSI Driver Capacity
// ---------------------------------------------------------------

type CSIDriverResult2048 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CSIDriverSummary2048 `json:"summary"`
	Drivers         []CSIDriverEntry2048 `json:"drivers"`
	Recommendations []string             `json:"recommendations"`
}

type CSIDriverSummary2048 struct {
	TotalDrivers   int `json:"totalDrivers"`
	StorageClasses int `json:"storageClasses"`
}

type CSIDriverEntry2048 struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
}

func (s *Server) handleCSIDriverCap(w http.ResponseWriter, r *http.Request) {
	result := CSIDriverResult2048{ScannedAt: time.Now()}
	score := 100

	driverList, _ := s.clientset.StorageV1().CSIDrivers().List(r.Context(), metav1.ListOptions{})
	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalDrivers = len(driverList.Items)
	result.Summary.StorageClasses = len(scList.Items)

	driverSet := make(map[string]bool)
	for _, d := range driverList.Items {
		result.Drivers = append(result.Drivers, CSIDriverEntry2048{
			Name: d.Name, Provisioner: d.Name,
		})
		driverSet[d.Name] = true
	}

	// Check if storage classes reference CSI drivers
	csiSCCount := 0
	for _, sc := range scList.Items {
		if driverSet[sc.Provisioner] {
			csiSCCount++
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.TotalDrivers == 0 {
		result.Recommendations = append(result.Recommendations,
			"No CSI drivers found — in-tree storage may limit features")
	}

	writeJSON(w, result)
}

// keep imports
var _ = autoscalingv2.HorizontalPodAutoscaler{}
var _ = corev1.Pod{}
