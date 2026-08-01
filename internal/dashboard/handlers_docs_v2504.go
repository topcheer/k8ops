package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.04 Documentation: Node SystemUUID, Pod Spec SetHostnameAsFQDN, Namespace Annotation Count
type NodeSystemUUIDResult2504 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		UniqueUUID int `json:"uniqueSystemUUIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeSystemUUID2504(w http.ResponseWriter, r *http.Request) {
	result := NodeSystemUUIDResult2504{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		uuid := node.Status.NodeInfo.SystemUUID
		if uuid != "" && !seen[uuid] {
			seen[uuid] = true
			result.Summary.UniqueUUID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SetHostnameFQDNResult2504 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithFQDN  int `json:"withSetHostnameAsFQDN"`
	} `json:"summary"`
}

func (s *Server) handleSetHostnameFQDN2504(w http.ResponseWriter, r *http.Request) {
	result := SetHostnameFQDNResult2504{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SetHostnameAsFQDN != nil && *pod.Spec.SetHostnameAsFQDN {
			result.Summary.WithFQDN++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSAnnotationResult2504 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS     int `json:"totalNamespaces"`
		TotalAnnots int `json:"totalAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleNSAnnotation2504(w http.ResponseWriter, r *http.Request) {
	result := NSAnnotationResult2504{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.TotalAnnots += len(ns.Annotations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
