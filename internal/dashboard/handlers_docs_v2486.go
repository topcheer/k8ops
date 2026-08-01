package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.86 Documentation: Node KubeProxyVersion, Pod Spec NodeName Distribution, Service ClusterIP Summary
type NodeKubeProxyResult2486 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubeProxy2486(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeProxyResult2486{ScannedAt: time.Now()}
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

type PodNodeNameDistResult2486 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		UniqueNodes int `json:"uniqueNodes"`
	} `json:"summary"`
}

func (s *Server) handlePodNodeNameDist2486(w http.ResponseWriter, r *http.Request) {
	result := PodNodeNameDistResult2486{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodes := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.NodeName != "" {
			nodes[pod.Spec.NodeName] = true
		}
	}
	result.Summary.UniqueNodes = len(nodes)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcClusterIPResult2486 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int            `json:"totalServices"`
		ByIPFamily map[string]int `json:"byIPFamily"`
	} `json:"summary"`
}

func (s *Server) handleSvcClusterIP2486(w http.ResponseWriter, r *http.Request) {
	result := SvcClusterIPResult2486{ScannedAt: time.Now()}
	result.Summary.ByIPFamily = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		for _, ip := range svc.Spec.ClusterIPs {
			if ip != "" && ip != "None" {
				if len(ip) > 0 && ip[0] == ':' {
					result.Summary.ByIPFamily["IPv6"]++
				} else {
					result.Summary.ByIPFamily["IPv4"]++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
