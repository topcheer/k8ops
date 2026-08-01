package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.44 Operations: Node Status Addresses Detail, Pod Spec HostAliases Count, Container VolumeMount ReadOnly
type NodeAddrDetailResult2544 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByType     map[string]int `json:"byAddressType"`
	} `json:"summary"`
}

func (s *Server) handleNodeAddrDetail2544(w http.ResponseWriter, r *http.Request) {
	result := NodeAddrDetailResult2544{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, addr := range node.Status.Addresses {
			result.Summary.ByType[string(addr.Type)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodHostAliasesCountResult2544 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithAliases int `json:"withHostAliases"`
	} `json:"summary"`
}

func (s *Server) handlePodHostAliasesCount2544(w http.ResponseWriter, r *http.Request) {
	result := PodHostAliasesCountResult2544{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.HostAliases) > 0 {
			result.Summary.WithAliases++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ReadOnlyMountResult2544 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalMounts int `json:"totalMounts"`
		ReadOnly    int `json:"readOnlyMounts"`
	} `json:"summary"`
}

func (s *Server) handleReadOnlyMount2544(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyMountResult2544{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, vm := range c.VolumeMounts {
				result.Summary.TotalMounts++
				if vm.ReadOnly {
					result.Summary.ReadOnly++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
