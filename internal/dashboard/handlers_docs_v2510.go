package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.10 Documentation: Node KubeletVersion vs KubeProxyVersion, Pod Spec ActiveDeadlineSeconds, Namespace Spec Finalizer List
type NodeVerCompareResult2510 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		Mismatched int `json:"versionMismatch"`
	} `json:"summary"`
}

func (s *Server) handleNodeVerCompare2510(w http.ResponseWriter, r *http.Request) {
	result := NodeVerCompareResult2510{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Status.NodeInfo.KubeletVersion != node.Status.NodeInfo.KubeProxyVersion {
			result.Summary.Mismatched++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Mismatched*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ActiveDeadlineResult2510 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithDeadline int `json:"withActiveDeadlineSeconds"`
	} `json:"summary"`
}

func (s *Server) handleActiveDeadline2510(w http.ResponseWriter, r *http.Request) {
	result := ActiveDeadlineResult2510{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ActiveDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSFinalizerListResult2510 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS     int            `json:"totalNamespaces"`
		ByFinalizer map[string]int `json:"byFinalizer"`
	} `json:"summary"`
}

func (s *Server) handleNSFinalizerList2510(w http.ResponseWriter, r *http.Request) {
	result := NSFinalizerListResult2510{ScannedAt: time.Now()}
	result.Summary.ByFinalizer = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		for _, f := range ns.Spec.Finalizers {
			result.Summary.ByFinalizer[string(f)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
