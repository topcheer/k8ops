package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.02 Operations: Node Address Count, Pod QOS Guaranteed Ratio, Container Last State Summary
type NodeAddressResult2502 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalAddrs  int `json:"totalAddresses"`
		ExternalIPs int `json:"externalIPs"`
	} `json:"summary"`
}

func (s *Server) handleNodeAddress2502(w http.ResponseWriter, r *http.Request) {
	result := NodeAddressResult2502{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, addr := range node.Status.Addresses {
			result.Summary.TotalAddrs++
			if addr.Type == corev1.NodeExternalIP {
				result.Summary.ExternalIPs++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type QOSGuaranteedResult2502 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		Guaranteed int `json:"guaranteedPods"`
	} `json:"summary"`
}

func (s *Server) handleQOSGuaranteed2502(w http.ResponseWriter, r *http.Request) {
	result := QOSGuaranteedResult2502{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Status.QOSClass == corev1.PodQOSGuaranteed {
			result.Summary.Guaranteed++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = result.Summary.Guaranteed * 100 / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type LastStateResult2502 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByReason        map[string]int `json:"byLastTerminationReason"`
	} `json:"summary"`
}

func (s *Server) handleLastState2502(w http.ResponseWriter, r *http.Request) {
	result := LastStateResult2502{ScannedAt: time.Now()}
	result.Summary.ByReason = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.LastTerminationState.Terminated != nil {
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "" {
					reason = "<unknown>"
				}
				result.Summary.ByReason[reason]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
