package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.84 Operations: Node Taint Summary, Pod Condition Distribution, Container Image Pull Count
type NodeTaintResult2484 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		TotalTaints int            `json:"totalTaints"`
		ByEffect    map[string]int `json:"byEffect"`
	} `json:"summary"`
}

func (s *Server) handleNodeTaint2484(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintResult2484{ScannedAt: time.Now()}
	result.Summary.ByEffect = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, taint := range node.Spec.Taints {
			result.Summary.TotalTaints++
			result.Summary.ByEffect[string(taint.Effect)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodConditionDistResult2484 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByCond    map[string]int `json:"byCondition"`
	} `json:"summary"`
}

func (s *Server) handlePodConditionDist2484(w http.ResponseWriter, r *http.Request) {
	result := PodConditionDistResult2484{ScannedAt: time.Now()}
	result.Summary.ByCond = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cond := range pod.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				result.Summary.ByCond[string(cond.Type)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImagePullCountResult2484 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPull        int `json:"withImagePullSecrets"`
	} `json:"summary"`
}

func (s *Server) handleImagePullCount2484(w http.ResponseWriter, r *http.Request) {
	result := ImagePullCountResult2484{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if len(pod.Spec.ImagePullSecrets) > 0 {
			for range pod.Spec.Containers {
				result.Summary.TotalContainers++
				result.Summary.WithPull++
			}
		} else {
			result.Summary.TotalContainers += len(pod.Spec.Containers)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
