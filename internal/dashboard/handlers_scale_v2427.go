package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.27 Scalability: Top Namespace by Restart, Node Allocatable Ephemeral GB, Cluster ConfigMap Keys
type TopNSRestartResult2427 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		Restarts  int    `json:"restartCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSRestart2427(w http.ResponseWriter, r *http.Request) {
	result := TopNSRestartResult2427{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsRestarts := make(map[string]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			nsRestarts[pod.Namespace] += int(cs.RestartCount)
		}
	}
	result.Summary.TotalNS = len(nsRestarts)
	for ns, restarts := range nsRestarts {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			Restarts  int    `json:"restartCount"`
		}{ns, restarts})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Restarts > result.TopNS[j].Restarts })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeEphemeralGBResult2427 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalAllocatableGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeEphemeralGB2427(w http.ResponseWriter, r *http.Request) {
	result := NodeEphemeralGBResult2427{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMKeysResult2427 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		TotalKeys int `json:"totalDataKeys"`
	} `json:"summary"`
}

func (s *Server) handleCMKeys2427(w http.ResponseWriter, r *http.Request) {
	result := CMKeysResult2427{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.TotalKeys += len(cm.Data)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
