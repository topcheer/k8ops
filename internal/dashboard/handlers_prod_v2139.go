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
// v21.39 — Product Dimension (Round 43)
// 1. Pod DNS Policy Audit
// 2. Container Resource Oversubscription Detector
// 3. Service Publish NotReady Addresses Audit
// ============================================================

type DNSPolicyResult2139 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSPolicySummary2139 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type DNSPolicySummary2139 struct {
	TotalPods int            `json:"totalPods"`
	ByPolicy  map[string]int `json:"byDNSPolicy"`
}

func (s *Server) handleDNSPolicy2139(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyResult2139{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byP := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		policy := string(pod.Spec.DNSPolicy)
		if policy == "" {
			policy = "ClusterFirst"
		}
		byP[policy]++
	}
	result.Summary.ByPolicy = byP
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Resource Oversubscription
type OversubResult2139 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         OversubSummary2139 `json:"summary"`
	Oversubbed      []OversubEntry2139 `json:"oversubscribed"`
	Recommendations []string           `json:"recommendations"`
}

type OversubSummary2139 struct {
	TotalContainers int `json:"totalContainers"`
	Oversubbed      int `json:"oversubscribed"`
}

type OversubEntry2139 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	LimCPU    float64 `json:"limitCPU"`
}

func (s *Server) handleOversub2139(w http.ResponseWriter, r *http.Request) {
	result := OversubResult2139{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			lim := c.Resources.Limits.Cpu()
			if !lim.IsZero() && lim.AsApproximateFloat64() > 4 {
				result.Summary.Oversubbed++
				result.Oversubbed = append(result.Oversubbed, OversubEntry2139{Pod: pod.Name, Namespace: pod.Namespace, LimCPU: lim.AsApproximateFloat64()})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Oversubbed, func(i, j int) bool { return result.Oversubbed[i].LimCPU > result.Oversubbed[j].LimCPU })

	if result.Summary.Oversubbed > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with >4 CPU cores limit — potential oversubscription", result.Summary.Oversubbed))
	}
	writeJSON(w, result)
}

// 3. Publish NotReady Addresses
type PubNotReadyResult2139 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PubNotReadySummary2139 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type PubNotReadySummary2139 struct {
	TotalServices   int `json:"totalServices"`
	PublishNotReady int `json:"publishNotReadyAddresses"`
}

func (s *Server) handlePubNotReady2139(w http.ResponseWriter, r *http.Request) {
	result := PubNotReadyResult2139{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PublishNotReady++
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
