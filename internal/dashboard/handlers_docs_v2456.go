package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.56 Documentation: Node KernelVersion Distribution, Pod DNSPolicy Distribution, Service Port Summary
type NodeKernelResult2456 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKernel2456(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelResult2456{ScannedAt: time.Now()}
	result.Summary.ByKernel = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KernelVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByKernel[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DNSPolicyDistResult2456 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byDNSPolicy"`
	} `json:"summary"`
}

func (s *Server) handleDNSPolicyDist2456(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyDistResult2456{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.DNSPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ServicePortSummaryResult2456 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int `json:"totalServices"`
		TotalPorts int `json:"totalPorts"`
	} `json:"summary"`
}

func (s *Server) handleServicePortSummary2456(w http.ResponseWriter, r *http.Request) {
	result := ServicePortSummaryResult2456{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.TotalPorts += len(svc.Spec.Ports)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
