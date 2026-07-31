package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.48 Documentation: Node Zone Label, Pod Resource Request Summary, Secret Namespace Count
type NodeZoneLabelResult2348 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByZone     map[string]int `json:"byZoneLabel"`
	} `json:"summary"`
}

func (s *Server) handleNodeZoneLabel2348(w http.ResponseWriter, r *http.Request) {
	result := NodeZoneLabelResult2348{ScannedAt: time.Now()}
	result.Summary.ByZone = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		zone := node.Labels[corev1.LabelFailureDomainBetaZone]
		if zone == "" {
			zone = node.Labels[corev1.LabelTopologyZone]
		}
		if zone == "" {
			zone = "<unknown>"
		}
		result.Summary.ByZone[zone]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodResReqResult2348 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int     `json:"totalPods"`
		TotalReqCPU   float64 `json:"totalRequestedCPU"`
		TotalReqMemGB float64 `json:"totalRequestedMemGB"`
	} `json:"summary"`
}

func (s *Server) handlePodResReq2348(w http.ResponseWriter, r *http.Request) {
	result := PodResReqResult2348{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalReqMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretNSCountResult2348 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByNamespace  map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleSecretNSCount2348(w http.ResponseWriter, r *http.Request) {
	result := SecretNSCountResult2348{ScannedAt: time.Now()}
	result.Summary.ByNamespace = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByNamespace[secret.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
