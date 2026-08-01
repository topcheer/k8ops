package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.77 Scalability: Top Namespace by Deployment v2, Node Memory Request Total, Cluster Service Total by Type
type TopNSByDeploy2Result2577 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		DepCount  int    `json:"deployCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByDeploy2Result2577(w http.ResponseWriter, r *http.Request) {
	result := TopNSByDeploy2Result2577{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsDeps := make(map[string]int)
	for _, dep := range depList.Items {
		nsDeps[dep.Namespace]++
	}
	result.Summary.TotalNS = len(nsDeps)
	for ns, count := range nsDeps {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			DepCount  int    `json:"deployCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].DepCount > result.TopNS[j].DepCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemReqTotalResult2577 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int     `json:"totalPods"`
		TotalReq  float64 `json:"totalMemReqGB"`
	}
}

func (s *Server) handleNodeMemReqTotal2577(w http.ResponseWriter, r *http.Request) {
	result := NodeMemReqTotalResult2577{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReq += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ServiceTotalByTypeResult2577 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByType    map[string]int `json:"byType"`
	}
}

func (s *Server) handleServiceTotalByType2577(w http.ResponseWriter, r *http.Request) {
	result := ServiceTotalByTypeResult2577{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByType[string(svc.Spec.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
