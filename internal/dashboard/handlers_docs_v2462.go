package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.62 Documentation: Node ContainerRuntime Distribution, Pod SchedulerName Summary, Ingress TLS Summary
type NodeRuntimeResult2462 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byContainerRuntimeVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeRuntime2462(w http.ResponseWriter, r *http.Request) {
	result := NodeRuntimeResult2462{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		rt := node.Status.NodeInfo.ContainerRuntimeVersion
		if rt == "" {
			rt = "<unknown>"
		}
		result.Summary.ByRuntime[rt]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SchedulerNameResult2462 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByScheduler map[string]int `json:"bySchedulerName"`
	} `json:"summary"`
}

func (s *Server) handleSchedulerName2462(w http.ResponseWriter, r *http.Request) {
	result := SchedulerNameResult2462{ScannedAt: time.Now()}
	result.Summary.ByScheduler = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sn := pod.Spec.SchedulerName
		if sn == "" {
			sn = "<default>"
		}
		result.Summary.ByScheduler[sn]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IngressTLSSummaryResult2462 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalIngress int `json:"totalIngress"`
		WithTLS      int `json:"withTLS"`
	} `json:"summary"`
}

func (s *Server) handleIngressTLSSummary2462(w http.ResponseWriter, r *http.Request) {
	result := IngressTLSSummaryResult2462{ScannedAt: time.Now()}
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	for _, ing := range ingList.Items {
		result.Summary.TotalIngress++
		if len(ing.Spec.TLS) > 0 {
			result.Summary.WithTLS++
		}
	}
	score := 100
	if result.Summary.TotalIngress > 0 {
		score = result.Summary.WithTLS * 100 / result.Summary.TotalIngress
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
