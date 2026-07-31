package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.70 Documentation: Pod Affinity Rules Catalog, NS Label Inventory, Service Type Distribution
type AffinityRulesResult2270 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		WithAffinity     int `json:"withAffinity"`
		WithAntiAffinity int `json:"withAntiAffinity"`
		WithNodeAffinity int `json:"withNodeAffinity"`
	} `json:"summary"`
}

func (s *Server) handleAffinityRules2270(w http.ResponseWriter, r *http.Request) {
	result := AffinityRulesResult2270{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil {
			if pod.Spec.Affinity.PodAffinity != nil {
				result.Summary.WithAffinity++
			}
			if pod.Spec.Affinity.PodAntiAffinity != nil {
				result.Summary.WithAntiAffinity++
			}
			if pod.Spec.Affinity.NodeAffinity != nil {
				result.Summary.WithNodeAffinity++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelResult2270 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS        int `json:"totalNS"`
		WithLabels     int `json:"withLabels"`
		TotalLabelKeys int `json:"totalLabelKeys"`
	} `json:"summary"`
}

func (s *Server) handleNSLabel2270(w http.ResponseWriter, r *http.Request) {
	result := NSLabelResult2270{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	seenKeys := make(map[string]bool)
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Labels) > 0 {
			result.Summary.WithLabels++
			for k := range ns.Labels {
				seenKeys[k] = true
			}
		}
	}
	result.Summary.TotalLabelKeys = len(seenKeys)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcTypeResult2270 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByType        map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleSvcType2270(w http.ResponseWriter, r *http.Request) {
	result := SvcTypeResult2270{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByType[string(svc.Spec.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
