package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.18 Documentation: Node Unschedulable Count, Pod Spec HostAliases IP Dist, Namespace Creation Date
type NodeUnschedulable2618Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		Unschedulable int `json:"unschedulable"`
	} `json:"summary"`
}

func (s *Server) handleNodeUnschedulable2618(w http.ResponseWriter, r *http.Request) {
	result := NodeUnschedulable2618Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Spec.Unschedulable {
			result.Summary.Unschedulable++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Unschedulable*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type HostAliasesIP2618Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByIP      map[string]int `json:"byHostAliasIP"`
	} `json:"summary"`
}

func (s *Server) handleHostAliasesIP2618(w http.ResponseWriter, r *http.Request) {
	result := HostAliasesIP2618Result{ScannedAt: time.Now()}
	result.Summary.ByIP = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, ha := range pod.Spec.HostAliases {
			result.Summary.ByIP[ha.IP]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSCreationDate2618Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int   `json:"totalNamespaces"`
		OldestNS int64 `json:"oldestAgeDays"`
	} `json:"summary"`
}

func (s *Server) handleNSCreationDate2618(w http.ResponseWriter, r *http.Request) {
	result := NSCreationDate2618Result{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var oldest time.Duration
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		age := now.Sub(ns.CreationTimestamp.Time)
		if age > oldest {
			oldest = age
		}
	}
	result.Summary.OldestNS = int64(oldest.Hours() / 24)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
