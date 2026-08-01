package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.88 Documentation: Node BootID Dist, Pod Spec Tolerations Summary, Namespace UID vs CreationTime
type NodeBootIDResult2588 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		UniqueBoots int `json:"uniqueBootIDs"`
	}
}

func (s *Server) handleNodeBootID2588(w http.ResponseWriter, r *http.Request) {
	result := NodeBootIDResult2588{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		bid := node.Status.NodeInfo.BootID
		if bid != "" && !seen[bid] {
			seen[bid] = true
			result.Summary.UniqueBoots++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TolerationsSummaryResult2588 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByOperator map[string]int `json:"byTolerationOperator"`
	}
}

func (s *Server) handleTolerationsSummary2588(w http.ResponseWriter, r *http.Request) {
	result := TolerationsSummaryResult2588{ScannedAt: time.Now()}
	result.Summary.ByOperator = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, t := range pod.Spec.Tolerations {
			result.Summary.ByOperator[string(t.Operator)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSUIDVsCreationResult2588 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
		WithUID int `json:"withUID"`
	}
}

func (s *Server) handleNSUIDVsCreation2588(w http.ResponseWriter, r *http.Request) {
	result := NSUIDVsCreationResult2588{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if ns.UID != "" {
			result.Summary.WithUID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
