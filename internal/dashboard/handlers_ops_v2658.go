package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.58 Operations: PodPhaseDist, NodeMemoryPressure, LimitRangeAudit

type PodPhaseDist2658Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     PodPhaseDist2658Summary `json:"summary"`
	Items       []PodPhaseDist2658Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type PodPhaseDist2658Summary struct {
	TotalPods int `json:"totalPods"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
}

type PodPhaseDist2658Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
}

func (s *Server) handlePodPhaseDist2658(w http.ResponseWriter, r *http.Request) {
	result := PodPhaseDist2658Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			ph := string(pod.Status.Phase)
			switch pod.Status.Phase {
			case corev1.PodRunning:
				result.Summary.Running++
			case corev1.PodPending:
				result.Summary.Pending++
			case corev1.PodFailed:
				result.Summary.Failed++
			case corev1.PodSucceeded:
				result.Summary.Succeeded++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodPhaseDist2658Item{
					Name: pod.Name, Namespace: pod.Namespace, Phase: ph,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemPressure2658Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     NodeMemPressure2658Summary `json:"summary"`
	Items       []NodeMemPressure2658Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type NodeMemPressure2658Summary struct {
	TotalNodes    int `json:"totalNodes"`
	PressureOK    int `json:"pressureOK"`
	UnderPressure int `json:"underPressure"`
}

type NodeMemPressure2658Item struct {
	Name           string `json:"name"`
	MemoryPressure string `json:"memoryPressure"`
}

func (s *Server) handleNodeMemPressure2658(w http.ResponseWriter, r *http.Request) {
	result := NodeMemPressure2658Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			status := "False"
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeMemoryPressure {
					status = string(cond.Status)
					break
				}
			}
			if status == "False" {
				result.Summary.PressureOK++
			} else {
				result.Summary.UnderPressure++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeMemPressure2658Item{
					Name: node.Name, MemoryPressure: status,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LimitRangeAudit2658Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     LimitRangeAudit2658Summary `json:"summary"`
	Items       []LimitRangeAudit2658Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type LimitRangeAudit2658Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithLimitRanges int `json:"withLimitRanges"`
	WithoutLimit    int `json:"withoutLimitRanges"`
}

type LimitRangeAudit2658Item struct {
	Namespace       string `json:"namespace"`
	LimitRangeCount int    `json:"limitRangeCount"`
}

func (s *Server) handleLimitRangeAudit2658(w http.ResponseWriter, r *http.Request) {
	result := LimitRangeAudit2658Result{ScannedAt: time.Now()}
	nss, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ns := range nss.Items {
			result.Summary.TotalNamespaces++
			lrs, _ := s.clientset.CoreV1().LimitRanges(ns.Name).List(r.Context(), metav1.ListOptions{})
			if len(lrs.Items) > 0 {
				result.Summary.WithLimitRanges++
				if len(result.Items) < 50 {
					result.Items = append(result.Items, LimitRangeAudit2658Item{
						Namespace: ns.Name, LimitRangeCount: len(lrs.Items),
					})
				}
			} else {
				result.Summary.WithoutLimit++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
