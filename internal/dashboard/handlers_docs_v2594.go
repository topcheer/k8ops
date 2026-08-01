package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.94 Documentation: Node Architecture Stable Label, Pod Spec Affinity PodAffinity, Namespace Spec Finalizer Detail
type NodeArchStableResult2594 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	}
}

func (s *Server) handleNodeArchStable2594(w http.ResponseWriter, r *http.Request) {
	result := NodeArchStableResult2594{ScannedAt: time.Now()}
	result.Summary.ByArch = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		arch := node.Labels[corev1.LabelArchStable]
		if arch == "" {
			arch = "<unknown>"
		}
		result.Summary.ByArch[arch]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodAffinityResult2594 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithAff   int `json:"withPodAffinity"`
	}
}

func (s *Server) handlePodAffinity2594(w http.ResponseWriter, r *http.Request) {
	result := PodAffinityResult2594{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil && pod.Spec.Affinity.PodAffinity != nil {
			result.Summary.WithAff++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSFinalizerDetailResult2594 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int `json:"totalNamespaces"`
		WithFinal  int `json:"withFinalizers"`
		TotalFinal int `json:"totalFinalizers"`
	}
}

func (s *Server) handleNSFinalizerDetail2594(w http.ResponseWriter, r *http.Request) {
	result := NSFinalizerDetailResult2594{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Spec.Finalizers) > 0 {
			result.Summary.WithFinal++
			result.Summary.TotalFinal += len(ns.Spec.Finalizers)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
