package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.26 Documentation: Node Role Label, Pod Spec HostAliases, PVC Phase Distribution
type NodeRoleResult2426 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRole     map[string]int `json:"byNodeRole"`
	} `json:"summary"`
}

func (s *Server) handleNodeRole2426(w http.ResponseWriter, r *http.Request) {
	result := NodeRoleResult2426{ScannedAt: time.Now()}
	result.Summary.ByRole = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}
		result.Summary.ByRole[role]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HostAliasesResult2426 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int `json:"totalPods"`
		WithHostAliases int `json:"withHostAliases"`
	} `json:"summary"`
}

func (s *Server) handleHostAliases2426(w http.ResponseWriter, r *http.Request) {
	result := HostAliasesResult2426{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.HostAliases) > 0 {
			result.Summary.WithHostAliases++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCPhaseResult2426 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePVCPhase2426(w http.ResponseWriter, r *http.Request) {
	result := PVCPhaseResult2426{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.Summary.ByPhase[string(pvc.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
