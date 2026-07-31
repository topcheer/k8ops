package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.37 Scalability: Top Namespace by Secret Count, Node CPU Headroom, Cluster Endpoint Health Ratio
type TopNSSecretResult2337 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace   string `json:"namespace"`
		SecretCount int    `json:"secretCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSSecret2337(w http.ResponseWriter, r *http.Request) {
	result := TopNSSecretResult2337{ScannedAt: time.Now()}
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

type NodeCPUHeadroomResult2337 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int     `json:"totalNodes"`
		TotalAllocCPU float64 `json:"totalAllocatableCPU"`
		TotalReqCPU   float64 `json:"totalRequestedCPU"`
		HeadroomCPU   float64 `json:"headroomCPU"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUHeadroom2337(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUHeadroomResult2337{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.HeadroomCPU = result.Summary.TotalAllocCPU - result.Summary.TotalReqCPU
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPRatioResult2337 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithEndpoints int `json:"withEndpoints"`
		WithoutEPs    int `json:"withoutEndpoints"`
	} `json:"summary"`
}

func (s *Server) handleEPRatio2337(w http.ResponseWriter, r *http.Request) {
	result := EPRatioResult2337{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	epSet := make(map[string]bool)
	for _, ep := range epList.Items {
		total := 0
		for _, sub := range ep.Subsets {
			total += len(sub.Addresses)
		}
		if total > 0 {
			epSet[ep.Namespace+"/"+ep.Name] = true
		}
	}
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if epSet[svc.Namespace+"/"+svc.Name] {
			result.Summary.WithEndpoints++
		} else {
			result.Summary.WithoutEPs++
		}
	}
	score := 100
	if result.Summary.TotalServices > 0 {
		score = result.Summary.WithEndpoints * 100 / result.Summary.TotalServices
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
