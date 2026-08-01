package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.24 Documentation: Node SystemUUID Dist, Pod QOSClass Dist, Namespace Type Label Dist
type NodeSysUUID2624Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		UniqueUUIDs int `json:"uniqueSystemUUIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeSysUUID2624(w http.ResponseWriter, r *http.Request) {
	result := NodeSysUUID2624Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		uuid := node.Status.NodeInfo.SystemUUID
		if uuid != "" && !seen[uuid] {
			seen[uuid] = true
			result.Summary.UniqueUUIDs++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodQOSClass2624Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByQOS     map[string]int `json:"byQOSClass"`
	} `json:"summary"`
}

func (s *Server) handlePodQOSClass2624(w http.ResponseWriter, r *http.Request) {
	result := PodQOSClass2624Result{ScannedAt: time.Now()}
	result.Summary.ByQOS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByQOS[string(pod.Status.QOSClass)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSTypeLabel2624Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByType  map[string]int `json:"byTypeLabel"`
	} `json:"summary"`
}

func (s *Server) handleNSTypeLabel2624(w http.ResponseWriter, r *http.Request) {
	result := NSTypeLabel2624Result{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		t := ns.Labels["type"]
		if t == "" {
			t = "<none>"
		}
		result.Summary.ByType[t]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
