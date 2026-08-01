package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v26.19 Scalability: Top Namespace by Secret v3, Node CPU Allocatable Min Max, Cluster NetworkPolicy Ingress Count
type TopNSBySecret3Result2619 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace   string `json:"namespace"`
		SecretCount int    `json:"secretCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySecret3Result2619(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySecret3Result2619{ScannedAt: time.Now()}
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

type NodeCPUAllocMinMax2619Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		MinAlloc   float64 `json:"minCPUAlloc"`
		MaxAlloc   float64 `json:"maxCPUAlloc"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUAllocMinMax2619(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocMinMax2619Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		if result.Summary.MinAlloc == 0 || alloc < result.Summary.MinAlloc {
			result.Summary.MinAlloc = alloc
		}
		if alloc > result.Summary.MaxAlloc {
			result.Summary.MaxAlloc = alloc
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NetPolicyIngress2619Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP     int `json:"totalNetworkPolicies"`
		WithIngress int `json:"withIngressRules"`
	} `json:"summary"`
}

func (s *Server) handleNetPolicyIngress2619(w http.ResponseWriter, r *http.Request) {
	result := NetPolicyIngress2619Result{ScannedAt: time.Now()}
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		if len(np.Spec.Ingress) > 0 {
			result.Summary.WithIngress++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
