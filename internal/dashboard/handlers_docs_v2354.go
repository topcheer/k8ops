package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.54 Documentation: Node Instance Type Label, Pod Env Var From ConfigMap, PVC Storage Class Name
type InstanceTypeResult2354 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByInstance map[string]int `json:"byInstanceType"`
	} `json:"summary"`
}

func (s *Server) handleInstanceType2354(w http.ResponseWriter, r *http.Request) {
	result := InstanceTypeResult2354{ScannedAt: time.Now()}
	result.Summary.ByInstance = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		it := node.Labels[corev1.LabelInstanceType]
		if it == "" {
			it = "<unknown>"
		}
		result.Summary.ByInstance[it]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvFromCMResult2354 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEnvFromCM   int `json:"withEnvFromConfigMap"`
	} `json:"summary"`
}

func (s *Server) handleEnvFromCM2354(w http.ResponseWriter, r *http.Request) {
	result := EnvFromCMResult2354{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					result.Summary.WithEnvFromCM++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCSCNameResult2354 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		BySCName  map[string]int `json:"byStorageClassName"`
	} `json:"summary"`
}

func (s *Server) handlePVCSCName2354(w http.ResponseWriter, r *http.Request) {
	result := PVCSCNameResult2354{ScannedAt: time.Now()}
	result.Summary.BySCName = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sc := "<default>"
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		result.Summary.BySCName[sc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
