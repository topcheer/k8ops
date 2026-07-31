package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.07 Scalability: Namespace CPU Limit vs Request Ratio, Node Storage Commit, Cluster Pod Churn Rate
type NSCPURatioResult2307 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS      int `json:"totalNS"`
		WellBalanced int `json:"wellBalanced"`
	} `json:"summary"`
}

func (s *Server) handleNSCPURatio2307(w http.ResponseWriter, r *http.Request) {
	result := NSCPURatioResult2307{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsReq := make(map[string]float64)
	nsLimit := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsReq[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
			nsLimit[pod.Namespace] += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsReq)
	for ns := range nsReq {
		req := nsReq[ns]
		lim := nsLimit[ns]
		if req > 0 && lim > 0 {
			ratio := lim / req
			if ratio >= 1 && ratio <= 4 {
				result.Summary.WellBalanced++
			}
		}
	}
	score := 100
	if result.Summary.TotalNS > 0 {
		score = result.Summary.WellBalanced * 100 / result.Summary.TotalNS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeStorageCommitResult2307 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
		CommitPct    int     `json:"commitPct"`
	} `json:"summary"`
}

func (s *Server) handleNodeStorageCommit2307(w http.ResponseWriter, r *http.Request) {
	result := NodeStorageCommitResult2307{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Storage().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocGB += node.Status.Allocatable.Storage().AsApproximateFloat64() / 1e9
	}
	if result.Summary.TotalCapGB > 0 {
		result.Summary.CommitPct = int((1 - result.Summary.TotalAllocGB/result.Summary.TotalCapGB) * 100)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodChurnResult2307 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		Created1h  int `json:"createdLast1h"`
		Created24h int `json:"createdLast24h"`
	} `json:"summary"`
}

func (s *Server) handlePodChurn2307(w http.ResponseWriter, r *http.Request) {
	result := PodChurnResult2307{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		age := now.Sub(pod.CreationTimestamp.Time)
		if age < time.Hour {
			result.Summary.Created1h++
		}
		if age < 24*time.Hour {
			result.Summary.Created24h++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		churnPct := result.Summary.Created1h * 100 / result.Summary.TotalPods
		if churnPct > 50 {
			score = 60
		} else if churnPct > 20 {
			score = 80
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
