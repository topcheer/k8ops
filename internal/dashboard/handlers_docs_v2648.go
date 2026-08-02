package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.48 Documentation: PodImageCount, NodePodCapacity, SvcPortCount

type PodImageCount2648Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     PodImageCount2648Summary `json:"summary"`
	Items       []PodImageCount2648Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type PodImageCount2648Summary struct {
	TotalPods   int `json:"totalPods"`
	MultiImage  int `json:"multiImage"`
	SingleImage int `json:"singleImage"`
}

type PodImageCount2648Item struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	ImageCount int    `json:"imageCount"`
}

func (s *Server) handlePodImageCount2648(w http.ResponseWriter, r *http.Request) {
	result := PodImageCount2648Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			imgCount := len(pod.Spec.Containers) + len(pod.Spec.InitContainers)
			if imgCount > 1 {
				result.Summary.MultiImage++
			} else {
				result.Summary.SingleImage++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodImageCount2648Item{
					Name: pod.Name, Namespace: pod.Namespace, ImageCount: imgCount,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodCapacity2648Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     NodePodCapacity2648Summary `json:"summary"`
	Items       []NodePodCapacity2648Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type NodePodCapacity2648Summary struct {
	TotalNodes int `json:"totalNodes"`
	AvgPodCap  int `json:"avgPodCapacity"`
}

type NodePodCapacity2648Item struct {
	Name        string `json:"name"`
	PodCapacity int64  `json:"podCapacity"`
}

func (s *Server) handleNodePodCapacity2648(w http.ResponseWriter, r *http.Request) {
	result := NodePodCapacity2648Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil && len(nodes.Items) > 0 {
		var totalCap int64
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			cap := node.Status.Allocatable.Pods().Value()
			totalCap += cap
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodePodCapacity2648Item{
					Name: node.Name, PodCapacity: cap,
				})
			}
		}
		result.Summary.AvgPodCap = int(totalCap) / len(nodes.Items)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcPortCount2648Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     SvcPortCount2648Summary `json:"summary"`
	Items       []SvcPortCount2648Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type SvcPortCount2648Summary struct {
	TotalServices int `json:"totalServices"`
	MultiPort     int `json:"multiPort"`
	SinglePort    int `json:"singlePort"`
}

type SvcPortCount2648Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	PortCount int    `json:"portCount"`
}

func (s *Server) handleSvcPortCount2648(w http.ResponseWriter, r *http.Request) {
	result := SvcPortCount2648Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			portCnt := len(svc.Spec.Ports)
			if portCnt > 1 {
				result.Summary.MultiPort++
			} else {
				result.Summary.SinglePort++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcPortCount2648Item{
					Name: svc.Name, Namespace: svc.Namespace, PortCount: portCnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
