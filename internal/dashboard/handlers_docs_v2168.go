package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.68 — Documentation Dimension (Round 47)
// 1. Node Architecture Summary
// 2. ConfigMap Data Key Count
// 3. Pod Volume Type Distribution
// ============================================================

type NodeArchResult2168 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeArch2168(w http.ResponseWriter, r *http.Request) {
	result := NodeArchResult2168{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByArch = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByArch[node.Status.NodeInfo.Architecture]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. ConfigMap Data Key Count
type CMKeyCountResult2168 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		TotalKeys int `json:"totalDataKeys"`
		MaxKeys   int `json:"maxKeysPerCM"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMKeyCount2168(w http.ResponseWriter, r *http.Request) {
	result := CMKeyCountResult2168{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		cnt := len(cm.Data) + len(cm.BinaryData)
		result.Summary.TotalKeys += cnt
		if cnt > result.Summary.MaxKeys {
			result.Summary.MaxKeys = cnt
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Volume Type Distribution
type VolTypeResult2168 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByVolType map[string]int `json:"byVolumeType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleVolType2168(w http.ResponseWriter, r *http.Request) {
	result := VolTypeResult2168{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByVolType = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			switch {
			case vol.ConfigMap != nil:
				result.Summary.ByVolType["configMap"]++
			case vol.Secret != nil:
				result.Summary.ByVolType["secret"]++
			case vol.EmptyDir != nil:
				result.Summary.ByVolType["emptyDir"]++
			case vol.PersistentVolumeClaim != nil:
				result.Summary.ByVolType["pvc"]++
			case vol.HostPath != nil:
				result.Summary.ByVolType["hostPath"]++
			case vol.Projected != nil:
				result.Summary.ByVolType["projected"]++
			case vol.DownwardAPI != nil:
				result.Summary.ByVolType["downwardAPI"]++
			default:
				result.Summary.ByVolType["other"]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
