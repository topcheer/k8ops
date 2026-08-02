package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.36 Documentation: Node OS Name, Pod Spec OSEnabled Detail, Namespace Label Key Count
type NodeOSName2636Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOS       map[string]int `json:"byOperatingSystem"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSName2636(w http.ResponseWriter, r *http.Request) {
	result := NodeOSName2636Result{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		os := node.Labels[corev1.LabelOSStable]
		if os == "" {
			os = "<unknown>"
		}
		result.Summary.ByOS[os]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodOSEnabled2636Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithOS    int `json:"withOSName"`
	} `json:"summary"`
}

func (s *Server) handlePodOSEnabled2636(w http.ResponseWriter, r *http.Request) {
	result := PodOSEnabled2636Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil && pod.Spec.OS.Name != "" {
			result.Summary.WithOS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelKeyCount2636Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByKey   map[string]int `json:"byLabelKey"`
	} `json:"summary"`
}

func (s *Server) handleNSLabelKeyCount2636(w http.ResponseWriter, r *http.Request) {
	result := NSLabelKeyCount2636Result{ScannedAt: time.Now()}
	result.Summary.ByKey = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		for k := range ns.Labels {
			result.Summary.ByKey[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
