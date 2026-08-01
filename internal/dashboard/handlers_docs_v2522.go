package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.22 Documentation: Node ContainerRuntimeVersion vs KubeletVersion, Pod Spec ServiceAccount, Namespace DeletionTimestamp
type RuntimeVsKubeletResult2522 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byRuntime"`
	} `json:"summary"`
}

func (s *Server) handleRuntimeVsKubelet2522(w http.ResponseWriter, r *http.Request) {
	result := RuntimeVsKubeletResult2522{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		rt := node.Status.NodeInfo.ContainerRuntimeVersion
		if rt == "" {
			rt = "<unknown>"
		}
		result.Summary.ByRuntime[rt]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodSAResult2522 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		BySA      map[string]int `json:"byServiceAccount"`
	} `json:"summary"`
}

func (s *Server) handlePodSA2522(w http.ResponseWriter, r *http.Request) {
	result := PodSAResult2522{ScannedAt: time.Now()}
	result.Summary.BySA = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sa := pod.Spec.ServiceAccountName
		if sa == "" {
			sa = "<default>"
		}
		result.Summary.BySA[sa]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSDeletionResult2522 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int `json:"totalNamespaces"`
		Deleting int `json:"deletingCount"`
	} `json:"summary"`
}

func (s *Server) handleNSDeletion2522(w http.ResponseWriter, r *http.Request) {
	result := NSDeletionResult2522{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if ns.DeletionTimestamp != nil {
			result.Summary.Deleting++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
