package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.70 Documentation: Node KubeProxyVersion Dist, Pod Spec Affinity NodeAffinity, Namespace Annotation vs Finalizer
type NodeKubeProxyDistResult2570 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	}
}

func (s *Server) handleNodeKubeProxyDist2570(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeProxyDistResult2570{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KubeProxyVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByVersion[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodNodeAffinityResult2570 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithNodeAff int `json:"withNodeAffinity"`
	}
}

func (s *Server) handlePodNodeAffinity2570(w http.ResponseWriter, r *http.Request) {
	result := PodNodeAffinityResult2570{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil {
			result.Summary.WithNodeAff++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSAnnotVsFinResult2570 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS   int `json:"totalNamespaces"`
		WithAnnot int `json:"withAnnotations"`
		WithFinal int `json:"withFinalizers"`
	}
}

func (s *Server) handleNSAnnotVsFin2570(w http.ResponseWriter, r *http.Request) {
	result := NSAnnotVsFinResult2570{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if len(ns.Annotations) > 0 {
			result.Summary.WithAnnot++
		}
		if len(ns.Spec.Finalizers) > 0 {
			result.Summary.WithFinal++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
