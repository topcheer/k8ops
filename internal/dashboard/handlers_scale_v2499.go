package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.99 Scalability: Top Namespace by Secret, Node CPU Limit Total, Cluster HPA Total
type TopNSBySecretResult2499 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace   string `json:"namespace"`
		SecretCount int    `json:"secretCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySecret2499(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySecretResult2499{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	nsSecrets := make(map[string]int)
	for _, secret := range secretList.Items {
		nsSecrets[secret.Namespace]++
	}
	result.Summary.TotalNS = len(nsSecrets)
	for ns, count := range nsSecrets {
		result.TopNS = append(result.TopNS, struct {
			Namespace   string `json:"namespace"`
			SecretCount int    `json:"secretCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SecretCount > result.TopNS[j].SecretCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPULimitTotalResult2499 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int     `json:"totalNodes"`
		TotalCPULim float64 `json:"totalCPULimitCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPULimitTotal2499(w http.ResponseWriter, r *http.Request) {
	result := NodeCPULimitTotalResult2499{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	var totalCPU float64
	nodesSeen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nodesSeen[pod.Spec.NodeName] = true
		for _, c := range pod.Spec.Containers {
			totalCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNodes = len(nodesSeen)
	result.Summary.TotalCPULim = totalCPU
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HPATotalResult2499 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalHPA int            `json:"totalHPA"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleHPATotal2499(w http.ResponseWriter, r *http.Request) {
	result := HPATotalResult2499{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPA++
		result.Summary.ByNS[hpa.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
