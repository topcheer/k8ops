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
// v20.23 — Product Dimension (Round 23)
// 1. PVC Resize Tracking — PVC capacity change & expansion tracking
// 2. Service Type Distribution — service type breakdown per cluster
// 3. Pod QoS Distribution — Guaranteed/Burstable/BestEffort breakdown
// ============================================================

// ---------------------------------------------------------------
// 1. PVC Resize Tracking
// ---------------------------------------------------------------

type PVCResizeResult2023 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVCResizeSummary2023 `json:"summary"`
	Resized         []PVCResizeEntry2023 `json:"resizedPVCs"`
	Recommendations []string             `json:"recommendations"`
}

type PVCResizeSummary2023 struct {
	TotalPVCs int `json:"totalPVCs"`
	Resized   int `json:"resizedPVCs"`
	Expanding int `json:"currentlyExpanding"`
}

type PVCResizeEntry2023 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Requested string `json:"requestedSize"`
	Capacity  string `json:"capacity"`
	Phase     string `json:"phase"`
}

func (s *Server) handlePVCResize(w http.ResponseWriter, r *http.Request) {
	result := PVCResizeResult2023{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		reqSize := ""
		capSize := ""
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			reqSize = qty.String()
		}
		if qty, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			capSize = qty.String()
		}

		// Detect resize: requested != capacity means expansion in progress
		isResized := reqSize != capSize && reqSize != "" && capSize != ""
		isExpanding := false
		for _, cond := range pvc.Status.Conditions {
			if cond.Type == corev1.PersistentVolumeClaimResizing && cond.Status == corev1.ConditionTrue {
				isExpanding = true
			}
		}

		if isResized {
			result.Summary.Resized++
			result.Resized = append(result.Resized, PVCResizeEntry2023{
				Name: pvc.Name, Namespace: pvc.Namespace,
				Requested: reqSize, Capacity: capSize,
				Phase: string(pvc.Status.Phase),
			})
		}
		if isExpanding {
			result.Summary.Expanding++
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs: %d resized, %d expanding", result.Summary.TotalPVCs, result.Summary.Resized, result.Summary.Expanding))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Type Distribution
// ---------------------------------------------------------------

type SvcTypeResult2023 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SvcTypeSummary2023 `json:"summary"`
	PerNS           []SvcTypeEntry2023 `json:"perNamespace"`
	Recommendations []string           `json:"recommendations"`
}

type SvcTypeSummary2023 struct {
	TotalServices int `json:"totalServices"`
	ClusterIP     int `json:"clusterIPCount"`
	NodePort      int `json:"nodePortCount"`
	LoadBalancer  int `json:"loadBalancerCount"`
	ExternalName  int `json:"externalNameCount"`
}

type SvcTypeEntry2023 struct {
	Namespace    string `json:"namespace"`
	ClusterIP    int    `json:"clusterIP"`
	NodePort     int    `json:"nodePort"`
	LoadBalancer int    `json:"loadBalancer"`
}

func (s *Server) handleSvcTypeDist(w http.ResponseWriter, r *http.Request) {
	result := SvcTypeResult2023{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsStats := make(map[string]*SvcTypeEntry2023)

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		entry, ok := nsStats[svc.Namespace]
		if !ok {
			entry = &SvcTypeEntry2023{Namespace: svc.Namespace}
			nsStats[svc.Namespace] = entry
		}

		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			result.Summary.ClusterIP++
			entry.ClusterIP++
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePort++
			entry.NodePort++
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancer++
			entry.LoadBalancer++
		case corev1.ServiceTypeExternalName:
			result.Summary.ExternalName++
		}
	}

	for _, e := range nsStats {
		result.PerNS = append(result.PerNS, *e)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].ClusterIP+result.PerNS[i].NodePort+result.PerNS[i].LoadBalancer >
			result.PerNS[j].ClusterIP+result.PerNS[j].NodePort+result.PerNS[j].LoadBalancer
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services: %d ClusterIP, %d NodePort, %d LB, %d ExternalName", result.Summary.TotalServices, result.Summary.ClusterIP, result.Summary.NodePort, result.Summary.LoadBalancer, result.Summary.ExternalName))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod QoS Distribution
// ---------------------------------------------------------------

type QoSDistResult2023 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         QoSDistSummary2023 `json:"summary"`
	PerNS           []QoSDistEntry2023 `json:"perNamespace"`
	Recommendations []string           `json:"recommendations"`
}

type QoSDistSummary2023 struct {
	TotalPods  int `json:"totalPods"`
	Guaranteed int `json:"guaranteed"`
	Burstable  int `json:"burstable"`
	BestEffort int `json:"bestEffort"`
}

type QoSDistEntry2023 struct {
	Namespace  string `json:"namespace"`
	Guaranteed int    `json:"guaranteed"`
	Burstable  int    `json:"burstable"`
	BestEffort int    `json:"bestEffort"`
}

func (s *Server) handleQoSDist(w http.ResponseWriter, r *http.Request) {
	result := QoSDistResult2023{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsStats := make(map[string]*QoSDistEntry2023)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		// Determine QoS from pod's QOSClass
		qos := string(pod.Status.QOSClass)
		if qos == "" {
			// Fallback: compute manually
			qos = computeQoS2023(pod)
		}

		entry, ok := nsStats[pod.Namespace]
		if !ok {
			entry = &QoSDistEntry2023{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = entry
		}

		switch qos {
		case "Guaranteed":
			result.Summary.Guaranteed++
			entry.Guaranteed++
		case "Burstable":
			result.Summary.Burstable++
			entry.Burstable++
		default:
			result.Summary.BestEffort++
			entry.BestEffort++
		}
	}

	for _, e := range nsStats {
		result.PerNS = append(result.PerNS, *e)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].Guaranteed+result.PerNS[i].Burstable+result.PerNS[i].BestEffort >
			result.PerNS[j].Guaranteed+result.PerNS[j].Burstable+result.PerNS[j].BestEffort
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	if result.Summary.BestEffort > result.Summary.TotalPods/3 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d Guaranteed, %d Burstable, %d BestEffort", result.Summary.TotalPods, result.Summary.Guaranteed, result.Summary.Burstable, result.Summary.BestEffort))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func computeQoS2023(pod corev1.Pod) string {
	hasRequest := false
	hasLimit := false
	allEqual := true

	for _, c := range pod.Spec.Containers {
		reqCPU := c.Resources.Requests.Cpu().Value()
		reqMem := c.Resources.Requests.Memory().Value()
		limCPU := c.Resources.Limits.Cpu().Value()
		limMem := c.Resources.Limits.Memory().Value()

		if reqCPU > 0 || reqMem > 0 {
			hasRequest = true
		}
		if limCPU > 0 || limMem > 0 {
			hasLimit = true
		}
		if reqCPU != limCPU || reqMem != limMem {
			allEqual = false
		}
	}

	if !hasRequest && !hasLimit {
		return "BestEffort"
	}
	if hasRequest && hasLimit && allEqual {
		return "Guaranteed"
	}
	return "Burstable"
}
