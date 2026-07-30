package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.80 — Documentation Dimension (Round 49)
// 1. Node OS Image Distribution
// 2. Service ExternalIP Tracker
// 3. Pod Init Container Count Catalog
// ============================================================

type OSDistResult2180 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOSDist2180(w http.ResponseWriter, r *http.Request) {
	result := OSDistResult2180{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByOSImage = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOSImage[node.Status.NodeInfo.OSImage]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. ExternalIP Tracker
type ExternalIPResult2180 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithExtIP     int `json:"withExternalIP"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleExternalIP2180(w http.ResponseWriter, r *http.Request) {
	result := ExternalIPResult2180{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" || ing.Hostname != "" {
				result.Summary.WithExtIP++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Init Container Count
type InitCtnrCountResult2180 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithInit      int `json:"withInitContainers"`
		MaxInitPerPod int `json:"maxInitPerPod"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleInitCtnrCount2180(w http.ResponseWriter, r *http.Request) {
	result := InitCtnrCountResult2180{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	maxInit := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		cnt := len(pod.Spec.InitContainers)
		if cnt > 0 {
			result.Summary.WithInit++
		}
		if cnt > maxInit {
			maxInit = cnt
		}
	}
	result.Summary.MaxInitPerPod = maxInit
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
