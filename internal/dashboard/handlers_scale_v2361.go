package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.61 Scalability: Namespace PVC Total, Node Container Runtime Version, Cluster Ingress Total
type NSPVCTotalResult2361 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByNS      map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNSPVCTotal2361(w http.ResponseWriter, r *http.Request) {
	result := NSPVCTotalResult2361{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.Summary.ByNS[pvc.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCtnrRuntimeResult2361 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byContainerRuntime"`
	} `json:"summary"`
}

func (s *Server) handleNodeCtnrRuntime2361(w http.ResponseWriter, r *http.Request) {
	result := NodeCtnrRuntimeResult2361{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByRuntime[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IngressTotalResult2361 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalIngress int            `json:"totalIngress"`
		ByNS         map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleIngressTotal2361(w http.ResponseWriter, r *http.Request) {
	result := IngressTotalResult2361{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	for _, ing := range ingList.Items {
		result.Summary.TotalIngress++
		result.Summary.ByNS[ing.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
