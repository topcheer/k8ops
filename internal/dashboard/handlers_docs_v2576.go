package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.76 Documentation: Node OSImage Dist, Pod Spec HostAliases Summary, Namespace Label Count
type NodeOSImageDistResult2576 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	}
}

func (s *Server) handleNodeOSImageDist2576(w http.ResponseWriter, r *http.Request) {
	result := NodeOSImageDistResult2576{ScannedAt: time.Now()}
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

type HostAliasesSummaryResult2576 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		TotalAliases int `json:"totalHostAliases"`
	}
}

func (s *Server) handleHostAliasesSummary2576(w http.ResponseWriter, r *http.Request) {
	result := HostAliasesSummaryResult2576{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalAliases += len(pod.Spec.HostAliases)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelCountResult2576 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS     int `json:"totalNamespaces"`
		TotalLabels int `json:"totalLabels"`
	}
}

func (s *Server) handleNSLabelCount2576(w http.ResponseWriter, r *http.Request) {
	result := NSLabelCountResult2576{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.TotalLabels += len(ns.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
