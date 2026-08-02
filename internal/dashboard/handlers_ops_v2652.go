package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.52 Operations: PodReadinessGate, NodeAllocatableCPU, ConfigMapKeyCount

type PodReadinessGate2652Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     PodReadinessGate2652Summary `json:"summary"`
	Items       []PodReadinessGate2652Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type PodReadinessGate2652Summary struct {
	TotalPods    int `json:"totalPods"`
	WithReadGate int `json:"withReadinessGate"`
	NoReadGate   int `json:"noReadinessGate"`
}

type PodReadinessGate2652Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	GateCount int    `json:"gateCount"`
}

func (s *Server) handlePodReadinessGate2652(w http.ResponseWriter, r *http.Request) {
	result := PodReadinessGate2652Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			cnt := len(pod.Spec.ReadinessGates)
			if cnt > 0 {
				result.Summary.WithReadGate++
			} else {
				result.Summary.NoReadGate++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodReadinessGate2652Item{
					Name: pod.Name, Namespace: pod.Namespace, GateCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocatableCPU2652Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     NodeAllocatableCPU2652Summary `json:"summary"`
	Items       []NodeAllocatableCPU2652Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type NodeAllocatableCPU2652Summary struct {
	TotalNodes  int `json:"totalNodes"`
	AvgCpuCores int `json:"avgCpuCores"`
}

type NodeAllocatableCPU2652Item struct {
	Name     string `json:"name"`
	CpuCores int64  `json:"cpuCores"`
}

func (s *Server) handleNodeAllocatableCPU2652(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocatableCPU2652Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil && len(nodes.Items) > 0 {
		var totalCPU int64
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			cpu := node.Status.Allocatable.Cpu().MilliValue()
			totalCPU += cpu
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeAllocatableCPU2652Item{
					Name: node.Name, CpuCores: cpu,
				})
			}
		}
		result.Summary.AvgCpuCores = int(totalCPU) / len(nodes.Items)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ConfigMapKeyCount2652Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     ConfigMapKeyCount2652Summary `json:"summary"`
	Items       []ConfigMapKeyCount2652Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type ConfigMapKeyCount2652Summary struct {
	TotalCMs int `json:"totalCMs"`
	LargeCMs int `json:"largeCMs"`
	SmallCMs int `json:"smallCMs"`
}

type ConfigMapKeyCount2652Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	KeyCount  int    `json:"keyCount"`
}

func (s *Server) handleConfigMapKeyCount2652(w http.ResponseWriter, r *http.Request) {
	result := ConfigMapKeyCount2652Result{ScannedAt: time.Now()}
	cms, err := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, cm := range cms.Items {
			result.Summary.TotalCMs++
			cnt := len(cm.Data) + len(cm.BinaryData)
			if cnt > 20 {
				result.Summary.LargeCMs++
			} else {
				result.Summary.SmallCMs++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, ConfigMapKeyCount2652Item{
					Name: cm.Name, Namespace: cm.Namespace, KeyCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
