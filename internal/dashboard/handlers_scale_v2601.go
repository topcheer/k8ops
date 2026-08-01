package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v26.01 Scalability: Top Namespace by CronJob, Node Storage Capacity vs Allocatable Ratio, Cluster HPA MinReplicas
type TopNSByCron2Result2601 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		CronCount int    `json:"cronJobCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByCron2Result2601(w http.ResponseWriter, r *http.Request) {
	result := TopNSByCron2Result2601{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	nsCrons := make(map[string]int)
	for _, cron := range cronList.Items {
		nsCrons[cron.Namespace]++
	}
	result.Summary.TotalNS = len(nsCrons)
	for ns, count := range nsCrons {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			CronCount int    `json:"cronJobCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CronCount > result.TopNS[j].CronCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeStorCapVsAllocRatioResult2601 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgRatio   float64 `json:"avgStorCapVsAllocRatio"`
	}
}

func (s *Server) handleNodeStorCapVsAllocRatio2601(w http.ResponseWriter, r *http.Request) {
	result := NodeStorCapVsAllocRatioResult2601{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Capacity.StorageEphemeral().AsApproximateFloat64()
		alloc := node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64()
		if cap > 0 {
			totalRatio += alloc / cap * 100
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgRatio = totalRatio / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HPAMinReplicasResult2601 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalHPA    int `json:"totalHPA"`
		TotalMinRep int `json:"totalMinReplicas"`
	}
}

func (s *Server) handleHPAMinReplicas2601(w http.ResponseWriter, r *http.Request) {
	result := HPAMinReplicasResult2601{ScannedAt: time.Now()}
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPA++
		if hpa.Spec.MinReplicas != nil {
			result.Summary.TotalMinRep += int(*hpa.Spec.MinReplicas)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
