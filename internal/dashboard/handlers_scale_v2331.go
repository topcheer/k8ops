package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"strings"
	"time"
)

// v23.31 Scalability: Top Image Registry Distribution, Node Pod Headroom, Namespace Service Density
type ImgRegistryResult2331 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByRegistry  map[string]int `json:"byRegistry"`
	} `json:"summary"`
}

func (s *Server) handleImgRegistry2331(w http.ResponseWriter, r *http.Request) {
	result := ImgRegistryResult2331{ScannedAt: time.Now()}
	result.Summary.ByRegistry = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seen[c.Image] {
				continue
			}
			seen[c.Image] = true
			result.Summary.TotalImages++
			reg := "docker.io"
			if idx := strings.Index(c.Image, "/"); idx > 0 {
				prefix := c.Image[:idx]
				if strings.Contains(prefix, ".") || strings.Contains(prefix, ":") {
					reg = prefix
				}
			}
			result.Summary.ByRegistry[reg]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeHeadroomResult2331 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		TotalCapPods int `json:"totalPodCapacity"`
		TotalPods    int `json:"totalRunningPods"`
		HeadroomPods int `json:"headroomPods"`
	} `json:"summary"`
}

func (s *Server) handleNodeHeadroom2331(w http.ResponseWriter, r *http.Request) {
	result := NodeHeadroomResult2331{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapPods += int(node.Status.Allocatable.Pods().Value())
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	result.Summary.HeadroomPods = result.Summary.TotalCapPods - result.Summary.TotalPods
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSSvcDensityResult2331 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS       int `json:"totalNS"`
		TotalServices int `json:"totalServices"`
		AvgPerNS      int `json:"avgPerNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		SvcCount  int    `json:"svcCount"`
	} `json:"topNS"`
}

func (s *Server) handleNSSvcDensity2331(w http.ResponseWriter, r *http.Request) {
	result := NSSvcDensityResult2331{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsSvc := make(map[string]int)
	for _, svc := range svcList.Items {
		nsSvc[svc.Namespace]++
		result.Summary.TotalServices++
	}
	result.Summary.TotalNS = len(nsSvc)
	if result.Summary.TotalNS > 0 {
		result.Summary.AvgPerNS = result.Summary.TotalServices / result.Summary.TotalNS
	}
	for ns, count := range nsSvc {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			SvcCount  int    `json:"svcCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SvcCount > result.TopNS[j].SvcCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
