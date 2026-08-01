package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.25 Scalability: Top Namespace by EPS v2, Node CPU Request Total, Cluster HPA MaxReplicas
type TopNSByEPS2Result2625 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int `json:"totalNamespaces"`
		TotalEPS int `json:"totalEndpointSlices"`
	}
}

func (s *Server) handleTopNSByEPS2Result2625(w http.ResponseWriter, r *http.Request) {
	result := TopNSByEPS2Result2625{ScannedAt: time.Now()}
	sliceList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	nsEPS := make(map[string]int)
	for _, slice := range sliceList.Items {
		nsEPS[slice.Namespace]++
	}
	result.Summary.TotalNS = len(nsEPS)
	for _, count := range nsEPS {
		result.Summary.TotalEPS += count
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUReqTotal2625Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int     `json:"totalPods"`
		TotalReq  float64 `json:"totalCPUReq"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUReqTotal2625(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUReqTotal2625Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HPAMaxReplicas2625Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalHPA    int `json:"totalHPA"`
		TotalMaxRep int `json:"totalMaxReplicas"`
	} `json:"summary"`
}

func (s *Server) handleHPAMaxReplicas2625(w http.ResponseWriter, r *http.Request) {
	result := HPAMaxReplicas2625Result{ScannedAt: time.Now()}
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPA++
		result.Summary.TotalMaxRep += int(hpa.Spec.MaxReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
