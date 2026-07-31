package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.94 Documentation: Volume Type Census, Pod NodeSelector Key Inventory, Namespace Finalizer Catalog
type VolTypeResult2294 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalVolumes int            `json:"totalVolumes"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleVolType2294(w http.ResponseWriter, r *http.Request) {
	result := VolTypeResult2294{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			result.Summary.TotalVolumes++
			volType := "other"
			switch {
			case vol.ConfigMap != nil:
				volType = "configMap"
			case vol.Secret != nil:
				volType = "secret"
			case vol.EmptyDir != nil:
				volType = "emptyDir"
			case vol.PersistentVolumeClaim != nil:
				volType = "pvc"
			case vol.HostPath != nil:
				volType = "hostPath"
			case vol.DownwardAPI != nil:
				volType = "downwardAPI"
			case vol.Projected != nil:
				volType = "projected"
			}
			result.Summary.ByType[volType]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeSelectorKeyResult2294 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		WithNodeSel int            `json:"withNodeSelector"`
		ByKey       map[string]int `json:"byKey"`
	} `json:"summary"`
}

func (s *Server) handleNodeSelectorKey2294(w http.ResponseWriter, r *http.Request) {
	result := NodeSelectorKeyResult2294{ScannedAt: time.Now()}
	result.Summary.ByKey = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.NodeSelector) > 0 {
			result.Summary.WithNodeSel++
			for k := range pod.Spec.NodeSelector {
				result.Summary.ByKey[k]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSFinalizerResult2294 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS       int            `json:"totalNS"`
		WithFinalizer int            `json:"withFinalizer"`
		ByFinalizer   map[string]int `json:"byFinalizer"`
	} `json:"summary"`
}

func (s *Server) handleNSFinalizer2294(w http.ResponseWriter, r *http.Request) {
	result := NSFinalizerResult2294{ScannedAt: time.Now()}
	result.Summary.ByFinalizer = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Spec.Finalizers) > 0 {
			result.Summary.WithFinalizer++
			for _, f := range ns.Spec.Finalizers {
				result.Summary.ByFinalizer[string(f)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
