package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.14 Documentation: Node KernelBootID, Pod ImagePullSecret Count, ConfigMap Namespace Count
type KernelBootResult2414 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		UniqueBoots int `json:"uniqueKernelBootIDs"`
	} `json:"summary"`
}

func (s *Server) handleKernelBoot2414(w http.ResponseWriter, r *http.Request) {
	result := KernelBootResult2414{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		seen[node.Status.NodeInfo.BootID] = true
	}
	result.Summary.UniqueBoots = len(seen)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImgPullSecretResult2414 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithIPS   int `json:"withImagePullSecrets"`
	} `json:"summary"`
}

func (s *Server) handleImgPullSecret2414(w http.ResponseWriter, r *http.Request) {
	result := ImgPullSecretResult2414{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ImagePullSecrets) > 0 {
			result.Summary.WithIPS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMNSCountResult2414 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs int            `json:"totalConfigMaps"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleCMNSCount2414(w http.ResponseWriter, r *http.Request) {
	result := CMNSCountResult2414{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.ByNS[cm.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
