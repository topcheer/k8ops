package dashboard

import (
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.46 Operations: PodHostPID, NodeContainerRuntime, EventCountByType

type PodHostPID2646Result struct {
	ScannedAt   time.Time             `json:"scannedAt"`
	Summary     PodHostPID2646Summary `json:"summary"`
	Items       []PodHostPID2646Item  `json:"items"`
	HealthScore int                   `json:"healthScore"`
	Grade       string                `json:"grade"`
}

type PodHostPID2646Summary struct {
	TotalPods   int `json:"totalPods"`
	HostPIDPods int `json:"hostPidPods"`
	NormalPods  int `json:"normalPods"`
}

type PodHostPID2646Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HostPID   bool   `json:"hostPid"`
}

func (s *Server) handlePodHostPID2646(w http.ResponseWriter, r *http.Request) {
	result := PodHostPID2646Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			if pod.Spec.HostPID {
				result.Summary.HostPIDPods++
			} else {
				result.Summary.NormalPods++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodHostPID2646Item{
					Name: pod.Name, Namespace: pod.Namespace, HostPID: pod.Spec.HostPID,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeContainerRuntime2646Result struct {
	ScannedAt   time.Time                       `json:"scannedAt"`
	Summary     NodeContainerRuntime2646Summary `json:"summary"`
	Items       []NodeContainerRuntime2646Item  `json:"items"`
	HealthScore int                             `json:"healthScore"`
	Grade       string                          `json:"grade"`
}

type NodeContainerRuntime2646Summary struct {
	TotalNodes  int `json:"totalNodes"`
	DockerNodes int `json:"dockerNodes"`
	ContdNodes  int `json:"containerdNodes"`
	CRIONodes   int `json:"crioNodes"`
}

type NodeContainerRuntime2646Item struct {
	Name             string `json:"name"`
	ContainerRuntime string `json:"containerRuntime"`
}

func (s *Server) handleNodeContainerRuntime2646(w http.ResponseWriter, r *http.Request) {
	result := NodeContainerRuntime2646Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			cr := node.Status.NodeInfo.ContainerRuntimeVersion
			switch {
			case strings.Contains(cr, "docker"):
				result.Summary.DockerNodes++
			case strings.Contains(cr, "containerd"):
				result.Summary.ContdNodes++
			case strings.Contains(cr, "cri-o"):
				result.Summary.CRIONodes++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeContainerRuntime2646Item{
					Name: node.Name, ContainerRuntime: cr,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventCountByType2646Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     EventCountByType2646Summary `json:"summary"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type EventCountByType2646Summary struct {
	TotalEvents int `json:"totalEvents"`
	Normal      int `json:"normal"`
	Warning     int `json:"warning"`
}

func (s *Server) handleEventCountByType2646(w http.ResponseWriter, r *http.Request) {
	result := EventCountByType2646Result{ScannedAt: time.Now()}
	events, err := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{Limit: 500})
	if err == nil {
		for _, ev := range events.Items {
			result.Summary.TotalEvents++
			if ev.Type == "Warning" {
				result.Summary.Warning++
			} else {
				result.Summary.Normal++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
