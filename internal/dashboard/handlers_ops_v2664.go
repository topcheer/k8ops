package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.64 Operations: PodContainerCount, NodeDiskPressure, NamespaceStatusPhase

type PodContainerCount2664Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     PodContainerCount2664Summary `json:"summary"`
	Items       []PodContainerCount2664Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type PodContainerCount2664Summary struct {
	TotalPods       int `json:"totalPods"`
	SingleContainer int `json:"singleContainer"`
	MultiContainer  int `json:"multiContainer"`
}

type PodContainerCount2664Item struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	ContainerCount int    `json:"containerCount"`
}

func (s *Server) handlePodContainerCount2664(w http.ResponseWriter, r *http.Request) {
	result := PodContainerCount2664Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			cnt := len(pod.Spec.Containers)
			if cnt > 1 {
				result.Summary.MultiContainer++
			} else {
				result.Summary.SingleContainer++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodContainerCount2664Item{
					Name: pod.Name, Namespace: pod.Namespace, ContainerCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeDiskPressure2664Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     NodeDiskPressure2664Summary `json:"summary"`
	Items       []NodeDiskPressure2664Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type NodeDiskPressure2664Summary struct {
	TotalNodes    int `json:"totalNodes"`
	PressureOK    int `json:"pressureOk"`
	UnderPressure int `json:"underPressure"`
}

type NodeDiskPressure2664Item struct {
	Name         string `json:"name"`
	DiskPressure string `json:"diskPressure"`
}

func (s *Server) handleNodeDiskPressure2664(w http.ResponseWriter, r *http.Request) {
	result := NodeDiskPressure2664Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			status := "False"
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeDiskPressure {
					status = string(cond.Status)
					break
				}
			}
			if status == "False" {
				result.Summary.PressureOK++
			} else {
				result.Summary.UnderPressure++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeDiskPressure2664Item{
					Name: node.Name, DiskPressure: status,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSStatusPhase2664Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     NSStatusPhase2664Summary `json:"summary"`
	Items       []NSStatusPhase2664Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type NSStatusPhase2664Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	Active          int `json:"active"`
	Terminating     int `json:"terminating"`
}

type NSStatusPhase2664Item struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

func (s *Server) handleNSStatusPhase2664(w http.ResponseWriter, r *http.Request) {
	result := NSStatusPhase2664Result{ScannedAt: time.Now()}
	nss, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ns := range nss.Items {
			result.Summary.TotalNamespaces++
			ph := string(ns.Status.Phase)
			if ns.Status.Phase == corev1.NamespaceActive {
				result.Summary.Active++
			} else {
				result.Summary.Terminating++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NSStatusPhase2664Item{
					Name: ns.Name, Phase: ph,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
