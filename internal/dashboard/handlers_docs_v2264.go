package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.64 Documentation: Pod Tolerations Catalog, Node OS Image Census, PV Reclaim Policy Inventory
type TolerationCatalogResult2264 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int            `json:"totalPods"`
		WithTolerations int            `json:"withTolerations"`
		ByOperator      map[string]int `json:"byOperator"`
	} `json:"summary"`
}

func (s *Server) handleTolerationCatalog2264(w http.ResponseWriter, r *http.Request) {
	result := TolerationCatalogResult2264{ScannedAt: time.Now()}
	result.Summary.ByOperator = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.Tolerations) > 0 {
			result.Summary.WithTolerations++
			for _, tol := range pod.Spec.Tolerations {
				result.Summary.ByOperator[string(tol.Operator)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeOSImageResult2264 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSImage2264(w http.ResponseWriter, r *http.Request) {
	result := NodeOSImageResult2264{ScannedAt: time.Now()}
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

type PVReclaimResult2264 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs        int            `json:"totalPVs"`
		ByReclaimPolicy map[string]int `json:"byReclaimPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePVReclaim2264(w http.ResponseWriter, r *http.Request) {
	result := PVReclaimResult2264{ScannedAt: time.Now()}
	result.Summary.ByReclaimPolicy = make(map[string]int)
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		result.Summary.ByReclaimPolicy[string(pv.Spec.PersistentVolumeReclaimPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
