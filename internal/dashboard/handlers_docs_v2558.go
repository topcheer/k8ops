package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.58 Documentation: Node KernelVersion Dist, Pod Spec HostPID HostIPC, Namespace Creation Time
type NodeKernelDistResult2558 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	}
}

func (s *Server) handleNodeKernelDist2558(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelDistResult2558{ScannedAt: time.Now()}
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

type HostPIDIPCResult2558 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostPID   int `json:"hostPID"`
		HostIPC   int `json:"hostIPC"`
	}
}

func (s *Server) handleHostPIDIPC2558(w http.ResponseWriter, r *http.Request) {
	result := HostPIDIPCResult2558{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostPID {
			result.Summary.HostPID++
		}
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSCreationTimeResult2558 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int     `json:"totalNamespaces"`
		AvgAgeDays float64 `json:"avgAgeDays"`
	}
}

func (s *Server) handleNSCreationTime2558(w http.ResponseWriter, r *http.Request) {
	result := NSCreationTimeResult2558{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var totalAge float64
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		totalAge += now.Sub(ns.CreationTimestamp.Time).Hours()
	}
	if result.Summary.TotalNS > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(result.Summary.TotalNS) / 24
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
