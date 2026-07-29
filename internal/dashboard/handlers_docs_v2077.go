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
// v20.77 — Documentation Dimension (Round 32)
// 1. Pod IP Range Inventory — pod CIDR distribution
// 2. Node OS Image Catalog — node OS version documentation
// 3. Service External IP Tracker — LoadBalancer external IP inventory
// ============================================================

type PodIPResult2077 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         PodIPSummary2077 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type PodIPSummary2077 struct {
	TotalPods int `json:"totalPods"`
	WithIP    int `json:"podsWithIP"`
	NoIP      int `json:"podsWithoutIP"`
}

func (s *Server) handlePodIPInv2077(w http.ResponseWriter, r *http.Request) {
	result := PodIPResult2077{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.PodIP != "" {
			result.Summary.WithIP++
		} else if pod.Status.Phase == corev1.PodRunning {
			result.Summary.NoIP++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node OS Image Catalog
// ---------------------------------------------------------------

type NodeOSResult2077 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         NodeOSSummary2077 `json:"summary"`
	OSImages        []NodeOSEntry2077 `json:"osImages"`
	Recommendations []string          `json:"recommendations"`
}

type NodeOSSummary2077 struct {
	TotalNodes int `json:"totalNodes"`
	UniqueOS   int `json:"uniqueOSImages"`
}

type NodeOSEntry2077 struct {
	Node    string `json:"node"`
	OSImage string `json:"osImage"`
}

func (s *Server) handleNodeOSCat2077(w http.ResponseWriter, r *http.Request) {
	result := NodeOSResult2077{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	osSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		osImage := node.Status.NodeInfo.OSImage
		osSet[osImage] = true
		result.OSImages = append(result.OSImages, NodeOSEntry2077{
			Node: node.Name, OSImage: osImage,
		})
	}
	result.Summary.UniqueOS = len(osSet)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OSImages, func(i, j int) bool { return result.OSImages[i].OSImage < result.OSImages[j].OSImage })

	if result.Summary.UniqueOS > 2 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d different OS images — consider standardizing", result.Summary.UniqueOS))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Service External IP Tracker
// ---------------------------------------------------------------

type ExtIPResult2077 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         ExtIPSummary2077 `json:"summary"`
	PendingSvcs     []ExtIPEntry2077 `json:"pendingExternalIP"`
	Recommendations []string         `json:"recommendations"`
}

type ExtIPSummary2077 struct {
	TotalLB   int `json:"totalLoadBalancers"`
	WithExtIP int `json:"withExternalIP"`
	PendingIP int `json:"pendingExternalIP"`
}

type ExtIPEntry2077 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleExtIPTracker(w http.ResponseWriter, r *http.Request) {
	result := ExtIPResult2077{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++

		hasExtIP := false
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" || ing.Hostname != "" {
				hasExtIP = true
				break
			}
		}

		if hasExtIP {
			result.Summary.WithExtIP++
		} else {
			result.Summary.PendingIP++
			result.PendingSvcs = append(result.PendingSvcs, ExtIPEntry2077{
				Service: svc.Name, Namespace: svc.Namespace,
			})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.PendingIP > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d LoadBalancers without external IP — check cloud provider", result.Summary.PendingIP))
	}
	writeJSON(w, result)
}
