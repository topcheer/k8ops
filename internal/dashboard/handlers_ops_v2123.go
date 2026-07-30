package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.23 — Operations Dimension (Round 40)
// 1. Pod QoS Guaranteed Ratio
// 2. Node KubeProxy Version Map
// 3. Container Volume Device Path Audit
// ============================================================

type QoSGuarResult2123 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         QoSGuarSummary2123 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type QoSGuarSummary2123 struct {
	TotalPods  int `json:"totalPods"`
	Guaranteed int `json:"guaranteed"`
}

func (s *Server) handleQoSGuar2123(w http.ResponseWriter, r *http.Request) {
	result := QoSGuarResult2123{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		guaranteed := true
		for _, c := range pod.Spec.Containers {
			reqC := c.Resources.Requests.Cpu()
			limC := c.Resources.Limits.Cpu()
			reqM := c.Resources.Requests.Memory()
			limM := c.Resources.Limits.Memory()
			if reqC.IsZero() || limC.IsZero() || reqM.IsZero() || limM.IsZero() {
				guaranteed = false
			}
			if reqC.Cmp(*limC) != 0 || reqM.Cmp(*limM) != 0 {
				guaranteed = false
			}
		}
		if guaranteed {
			result.Summary.Guaranteed++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. KubeProxy Version Map
type KPVerResult2123 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         KPVerSummary2123 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type KPVerSummary2123 struct {
	TotalNodes int            `json:"totalNodes"`
	Versions   map[string]int `json:"kubeProxyVersions"`
}

func (s *Server) handleKPVer2123(w http.ResponseWriter, r *http.Request) {
	result := KPVerResult2123{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	versions := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		versions[node.Status.NodeInfo.KubeProxyVersion]++
	}
	result.Summary.Versions = versions
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Volume Device Path
type VolDevResult2123 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         VolDevSummary2123 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type VolDevSummary2123 struct {
	TotalPods      int `json:"totalPods"`
	WithDevicePath int `json:"withDevicePath"`
}

func (s *Server) handleVolDev2123(w http.ResponseWriter, r *http.Request) {
	result := VolDevResult2123{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil && vol.HostPath.Path != "" {
				result.Summary.WithDevicePath++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
