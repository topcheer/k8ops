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
// v20.49 — Product Dimension (Round 28)
// 1. Workload Owner Reference Audit — orphaned resource detection
// 2. Service Type Compliance — LoadBalancer vs ClusterIP usage
// 3. Pod resource Gap Analyzer — request vs limit ratio
// ============================================================

// ---------------------------------------------------------------
// 1. Workload Owner Reference Audit
// ---------------------------------------------------------------

type OwnerRefResult2049 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         OwnerRefSummary2049 `json:"summary"`
	OrphanedPods    []OwnerRefEntry2049 `json:"orphanedPods"`
	Recommendations []string            `json:"recommendations"`
}

type OwnerRefSummary2049 struct {
	TotalPods int `json:"totalPods"`
	WithOwner int `json:"withOwner"`
	Orphaned  int `json:"orphaned"`
}

type OwnerRefEntry2049 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleOwnerRefAudit(w http.ResponseWriter, r *http.Request) {
	result := OwnerRefResult2049{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if len(pod.OwnerReferences) > 0 {
			result.Summary.WithOwner++
		} else {
			// Bare pods without owner
			result.Summary.Orphaned++
			result.OrphanedPods = append(result.OrphanedPods, OwnerRefEntry2049{
				Pod: pod.Name, Namespace: pod.Namespace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OrphanedPods, func(i, j int) bool {
		return result.OrphanedPods[i].Namespace < result.OrphanedPods[j].Namespace
	})

	if result.Summary.Orphaned > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d orphaned pods without owner — consider using Deployments", result.Summary.Orphaned))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Type Compliance
// ---------------------------------------------------------------

type SvcTypeResult2049 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SvcTypeSummary2049 `json:"summary"`
	ExternalSvcs    []SvcTypeEntry2049 `json:"externalServices"`
	Recommendations []string           `json:"recommendations"`
}

type SvcTypeSummary2049 struct {
	TotalServices int `json:"totalServices"`
	ClusterIP     int `json:"clusterIP"`
	NodePort      int `json:"nodePort"`
	LoadBalancer  int `json:"loadBalancer"`
	ExternalName  int `json:"externalName"`
}

type SvcTypeEntry2049 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

func (s *Server) handleSvcTypeCompliance(w http.ResponseWriter, r *http.Request) {
	result := SvcTypeResult2049{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			result.Summary.ClusterIP++
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePort++
			result.ExternalSvcs = append(result.ExternalSvcs, SvcTypeEntry2049{
				Name: svc.Name, Namespace: svc.Namespace, Type: "NodePort",
			})
			score -= 2
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancer++
			result.ExternalSvcs = append(result.ExternalSvcs, SvcTypeEntry2049{
				Name: svc.Name, Namespace: svc.Namespace, Type: "LoadBalancer",
			})
			score -= 1
		case corev1.ServiceTypeExternalName:
			result.Summary.ExternalName++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.ExternalSvcs, func(i, j int) bool {
		return result.ExternalSvcs[i].Type < result.ExternalSvcs[j].Type
	})

	if result.Summary.NodePort > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d NodePort services — use Ingress for centralized traffic management", result.Summary.NodePort))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Resource Gap Analyzer
// ---------------------------------------------------------------

type ResGapResult2049 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         ResGapSummary2049 `json:"summary"`
	WideGap         []ResGapEntry2049 `json:"wideGapContainers"`
	Recommendations []string          `json:"recommendations"`
}

type ResGapSummary2049 struct {
	TotalContainers int `json:"totalContainers"`
	WideGap         int `json:"wideGap"`
	NarrowGap       int `json:"narrowGap"`
}

type ResGapEntry2049 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Container string  `json:"container"`
	CPURatio  float64 `json:"cpuRequestToLimit"`
	MemRatio  float64 `json:"memRequestToLimit"`
}

func (s *Server) handleResGapAnalyzer(w http.ResponseWriter, r *http.Request) {
	result := ResGapResult2049{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			cpuReq := c.Resources.Requests.Cpu()
			cpuLim := c.Resources.Limits.Cpu()
			memReq := c.Resources.Requests.Memory()
			memLim := c.Resources.Limits.Memory()

			if cpuLim.IsZero() || cpuReq.IsZero() {
				continue
			}

			cpuRatio := cpuReq.AsApproximateFloat64() / cpuLim.AsApproximateFloat64()
			memRatio := 1.0
			if !memLim.IsZero() && !memReq.IsZero() {
				memRatio = memReq.AsApproximateFloat64() / memLim.AsApproximateFloat64()
			}

			// Wide gap: request < 10% of limit (over-provisioned limits)
			if cpuRatio < 0.1 || memRatio < 0.1 {
				result.Summary.WideGap++
				result.WideGap = append(result.WideGap, ResGapEntry2049{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					CPURatio: cpuRatio, MemRatio: memRatio,
				})
				score -= 1
			} else if cpuRatio > 0.9 {
				result.Summary.NarrowGap++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.WideGap, func(i, j int) bool {
		return result.WideGap[i].CPURatio < result.WideGap[j].CPURatio
	})

	if result.Summary.WideGap > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have wide request/limit gap — tighten limits for better scheduling", result.Summary.WideGap))
	}

	writeJSON(w, result)
}
