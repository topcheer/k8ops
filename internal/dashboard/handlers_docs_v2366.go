package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.66 Documentation: Pod Restart Policy Distribution, Node OS Image, Service Port Target
type RestartPolResult2366 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byRestartPolicy"`
	} `json:"summary"`
}

func (s *Server) handleRestartPol2366(w http.ResponseWriter, r *http.Request) {
	result := RestartPolResult2366{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.RestartPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeOSImageResult2366 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSImage2366(w http.ResponseWriter, r *http.Request) {
	result := NodeOSImageResult2366{ScannedAt: time.Now()}
	result.Summary.ByOSImage = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOSImage[node.Status.NodeInfo.OSImage]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcPortTargetResult2366 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices    int `json:"totalServices"`
		TotalTargetPorts int `json:"totalTargetPorts"`
	} `json:"summary"`
}

func (s *Server) handleSvcPortTarget2366(w http.ResponseWriter, r *http.Request) {
	result := SvcPortTargetResult2366{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			if p.TargetPort.IntVal != 0 || p.TargetPort.StrVal != "" {
				result.Summary.TotalTargetPorts++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
