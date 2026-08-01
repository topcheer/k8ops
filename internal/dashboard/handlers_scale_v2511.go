package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.11 Scalability: Top Namespace by ReplicaSet, Node Memory Limit Total, Cluster Lease Count
type TopNSByRSResult2511 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		RSCount   int    `json:"rsCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByRS2511(w http.ResponseWriter, r *http.Request) {
	result := TopNSByRSResult2511{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	nsRS := make(map[string]int)
	for _, rs := range rsList.Items {
		if *rs.Spec.Replicas == 0 {
			continue
		}
		nsRS[rs.Namespace]++
	}
	result.Summary.TotalNS = len(nsRS)
	for ns, count := range nsRS {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			RSCount   int    `json:"rsCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].RSCount > result.TopNS[j].RSCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemLimitTotalResult2511 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int     `json:"totalPods"`
		TotalLimit float64 `json:"totalMemLimitGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemLimitTotal2511(w http.ResponseWriter, r *http.Request) {
	result := NodeMemLimitTotalResult2511{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLimit += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LeaseCountResult2511 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLeases int            `json:"totalLeases"`
		ByNS        map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleLeaseCount2511(w http.ResponseWriter, r *http.Request) {
	result := LeaseCountResult2511{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	leaseList, _ := s.clientset.CoordinationV1().Leases("").List(r.Context(), metav1.ListOptions{})
	for _, lease := range leaseList.Items {
		result.Summary.TotalLeases++
		result.Summary.ByNS[lease.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
