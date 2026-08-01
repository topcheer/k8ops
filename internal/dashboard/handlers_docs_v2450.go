package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.50 Documentation: Node OS Image Distribution, Pod RestartPolicy Distribution, Service Type Distribution
type NodeOSImageResult2450 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSImage2450(w http.ResponseWriter, r *http.Request) {
	result := NodeOSImageResult2450{ScannedAt: time.Now()}
	result.Summary.ByOSImage = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		img := node.Status.NodeInfo.OSImage
		if img == "" {
			img = "<unknown>"
		}
		result.Summary.ByOSImage[img]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RestartPolicyDistResult2450 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byRestartPolicy"`
	} `json:"summary"`
}

func (s *Server) handleRestartPolicyDist2450(w http.ResponseWriter, r *http.Request) {
	result := RestartPolicyDistResult2450{ScannedAt: time.Now()}
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

type ServiceTypeDistResult2450 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByType    map[string]int `json:"byServiceType"`
	} `json:"summary"`
}

func (s *Server) handleServiceTypeDist2450(w http.ResponseWriter, r *http.Request) {
	result := ServiceTypeDistResult2450{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByType[string(svc.Spec.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
