package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.96 Documentation: Node Taint Count, Pod NodeSelector Key, Endpoint Address by Node
type TaintCountResult2396 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalTaints int `json:"totalTaints"`
	} `json:"summary"`
}

func (s *Server) handleTaintCount2396(w http.ResponseWriter, r *http.Request) {
	result := TaintCountResult2396{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalTaints += len(node.Spec.Taints)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeSelectorKeyResult2396 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int            `json:"totalPods"`
		BySelectorKey map[string]int `json:"byNodeSelectorKey"`
	} `json:"summary"`
}

func (s *Server) handleNodeSelectorKey2396(w http.ResponseWriter, r *http.Request) {
	result := NodeSelectorKeyResult2396{ScannedAt: time.Now()}
	result.Summary.BySelectorKey = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for k := range pod.Spec.NodeSelector {
			result.Summary.BySelectorKey[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPAddrByNodeResult2396 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEndpoints int            `json:"totalEndpoints"`
		ByNode         map[string]int `json:"byNode"`
	} `json:"summary"`
}

func (s *Server) handleEPAddrByNode2396(w http.ResponseWriter, r *http.Request) {
	result := EPAddrByNodeResult2396{ScannedAt: time.Now()}
	result.Summary.ByNode = make(map[string]int)
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	for _, ep := range epList.Items {
		for _, sub := range ep.Subsets {
			for _, addr := range sub.Addresses {
				result.Summary.TotalEndpoints++
				if addr.NodeName != nil {
					result.Summary.ByNode[*addr.NodeName]++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
