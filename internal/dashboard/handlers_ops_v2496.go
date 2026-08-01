package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.96 Operations: Node Kubelet Version Drift, Pod Container Status Summary, Namespace ResourceQuota Count
type NodeKubeletDriftResult2496 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int            `json:"totalNodes"`
		ByVersion      map[string]int `json:"byKubeletVersion"`
		UniqueVersions int            `json:"uniqueVersions"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubeletDrift2496(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeletDriftResult2496{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KubeletVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByVersion[kv]++
	}
	result.Summary.UniqueVersions = len(result.Summary.ByVersion)
	score := 100
	if result.Summary.UniqueVersions > 1 {
		score = 100 - (result.Summary.UniqueVersions-1)*10
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CtnrStatusSummaryResult2496 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Running         int `json:"running"`
		Waiting         int `json:"waiting"`
		Terminated      int `json:"terminated"`
	} `json:"summary"`
}

func (s *Server) handleCtnrStatusSummary2496(w http.ResponseWriter, r *http.Request) {
	result := CtnrStatusSummaryResult2496{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Running != nil {
				result.Summary.Running++
			} else if cs.State.Waiting != nil {
				result.Summary.Waiting++
			} else if cs.State.Terminated != nil {
				result.Summary.Terminated++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSResourceQuotaResult2496 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalQuotas int            `json:"totalResourceQuotas"`
		ByNS        map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNSResourceQuota2496(w http.ResponseWriter, r *http.Request) {
	result := NSResourceQuotaResult2496{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	rqList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	for _, rq := range rqList.Items {
		result.Summary.TotalQuotas++
		result.Summary.ByNS[rq.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
