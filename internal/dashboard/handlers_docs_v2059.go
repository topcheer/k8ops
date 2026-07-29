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
// v20.59 — Documentation Dimension (Round 29)
// 1. Service Port Mapping — service to container port catalog
// 2. Node Taint Effect Catalog — taint scheduling impact documentation
// 3. ConfigMap Key Inventory — configmap data key documentation
// ============================================================

type SvcPortResult2059 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SvcPortSummary2059 `json:"summary"`
	PortMappings    []SvcPortEntry2059 `json:"portMappings"`
	Recommendations []string           `json:"recommendations"`
}

type SvcPortSummary2059 struct {
	TotalServices   int `json:"totalServices"`
	TotalPorts      int `json:"totalPorts"`
	PrivilegedPorts int `json:"privilegedPorts"`
}

type SvcPortEntry2059 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Ports     string `json:"ports"`
}

func (s *Server) handleSvcPortMapping(w http.ResponseWriter, r *http.Request) {
	result := SvcPortResult2059{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		ports := []string{}
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			portStr := fmt.Sprintf("%d/%s", p.Port, string(p.Protocol))
			if p.Port < 1024 {
				result.Summary.PrivilegedPorts++
			}
			ports = append(ports, portStr)
		}
		result.PortMappings = append(result.PortMappings, SvcPortEntry2059{
			Service: svc.Name, Namespace: svc.Namespace,
			Ports: fmt.Sprintf("%v", ports),
		})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PortMappings, func(i, j int) bool {
		return result.PortMappings[i].Namespace < result.PortMappings[j].Namespace
	})

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Taint Effect Catalog
// ---------------------------------------------------------------

type TaintEffectResult2059 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         TaintEffectSummary2059 `json:"summary"`
	Taints          []TaintEffectEntry2059 `json:"taints"`
	Recommendations []string               `json:"recommendations"`
}

type TaintEffectSummary2059 struct {
	TotalNodes      int `json:"totalNodes"`
	NodesWithTaints int `json:"nodesWithTaints"`
	TotalTaints     int `json:"totalTaints"`
}

type TaintEffectEntry2059 struct {
	Node  string `json:"node"`
	Taint string `json:"taint"`
}

func (s *Server) handleTaintEffectCatalog(w http.ResponseWriter, r *http.Request) {
	result := TaintEffectResult2059{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if len(node.Spec.Taints) > 0 {
			result.Summary.NodesWithTaints++
			result.Summary.TotalTaints += len(node.Spec.Taints)
			for _, taint := range node.Spec.Taints {
				result.Taints = append(result.Taints, TaintEffectEntry2059{
					Node:  node.Name,
					Taint: fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, string(taint.Effect)),
				})
				if taint.Effect == corev1.TaintEffectNoSchedule {
					score -= 2
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Taints, func(i, j int) bool { return result.Taints[i].Node < result.Taints[j].Node })

	if result.Summary.NodesWithTaints > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have taints — review scheduling impact", result.Summary.NodesWithTaints))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. ConfigMap Key Inventory
// ---------------------------------------------------------------

type CMKeyResult2059 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         CMKeySummary2059 `json:"summary"`
	LargeCMs        []CMKeyEntry2059 `json:"largeConfigMaps"`
	Recommendations []string         `json:"recommendations"`
}

type CMKeySummary2059 struct {
	TotalCMs  int `json:"totalConfigMaps"`
	TotalKeys int `json:"totalKeys"`
	LargeCMs  int `json:"largeConfigMaps"`
}

type CMKeyEntry2059 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	KeyCount  int    `json:"keyCount"`
	SizeKB    int    `json:"sizeKB"`
}

func (s *Server) handleCMKeyInventory(w http.ResponseWriter, r *http.Request) {
	result := CMKeyResult2059{ScannedAt: time.Now()}
	score := 100

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		keyCount := len(cm.Data) + len(cm.BinaryData)
		result.Summary.TotalKeys += keyCount

		sizeBytes := 0
		for _, v := range cm.Data {
			sizeBytes += len(v)
		}
		for _, v := range cm.BinaryData {
			sizeBytes += len(v)
		}
		sizeKB := sizeBytes / 1024

		if sizeKB > 100 {
			result.Summary.LargeCMs++
			result.LargeCMs = append(result.LargeCMs, CMKeyEntry2059{
				Name: cm.Name, Namespace: cm.Namespace,
				KeyCount: keyCount, SizeKB: sizeKB,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.LargeCMs, func(i, j int) bool { return result.LargeCMs[i].SizeKB > result.LargeCMs[j].SizeKB })

	if result.Summary.LargeCMs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ConfigMaps >100KB — consider splitting for performance", result.Summary.LargeCMs))
	}
	writeJSON(w, result)
}
