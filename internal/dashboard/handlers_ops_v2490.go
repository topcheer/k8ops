package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.90 Operations: Node Unschedulable Count, Pod ImagePullBackOff Count, Container VolumeMount Summary
type NodeUnschedulableResult2490 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		Unschedulable int `json:"unschedulable"`
	} `json:"summary"`
}

func (s *Server) handleNodeUnschedulable2490(w http.ResponseWriter, r *http.Request) {
	result := NodeUnschedulableResult2490{ScannedAt: time.Now()}
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

type ImagePullBackOffResult2490 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		PullBackOff int `json:"imagePullBackOff"`
	} `json:"summary"`
}

func (s *Server) handleImagePullBackOff2490(w http.ResponseWriter, r *http.Request) {
	result := ImagePullBackOffResult2490{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && (cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "ErrImagePull") {
				result.Summary.PullBackOff++
			}
		}
	}
	score := 100
	if result.Summary.PullBackOff > 0 {
		score = 100 - result.Summary.PullBackOff*10
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type VolumeMountResult2490 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalMounts     int `json:"totalVolumeMounts"`
	} `json:"summary"`
}

func (s *Server) handleVolumeMount2490(w http.ResponseWriter, r *http.Request) {
	result := VolumeMountResult2490{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMounts += len(c.VolumeMounts)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
