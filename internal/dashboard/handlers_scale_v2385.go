package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.85 Scalability: Top Image by Deployment, Node CPU Limit Commit, Cluster Service Total
type TopImgDeployResult2385 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int `json:"totalImages"`
	} `json:"summary"`
	TopImages []struct {
		Image       string `json:"image"`
		DeployCount int    `json:"deployCount"`
	} `json:"topImages"`
}

func (s *Server) handleTopImgDeploy2385(w http.ResponseWriter, r *http.Request) {
	result := TopImgDeployResult2385{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	imgCount := make(map[string]int)
	for _, dep := range depList.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			imgCount[c.Image]++
		}
	}
	result.Summary.TotalImages = len(imgCount)
	for img, count := range imgCount {
		result.TopImages = append(result.TopImages, struct {
			Image       string `json:"image"`
			DeployCount int    `json:"deployCount"`
		}{img, count})
	}
	sort.Slice(result.TopImages, func(i, j int) bool { return result.TopImages[i].DeployCount > result.TopImages[j].DeployCount })
	if len(result.TopImages) > 10 {
		result.TopImages = result.TopImages[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPULimitResult2385 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int     `json:"totalNodes"`
		TotalLimitCPU float64 `json:"totalLimitCPU"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPULimit2385(w http.ResponseWriter, r *http.Request) {
	result := NodeCPULimitResult2385{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for range nodeList.Items {
		result.Summary.TotalNodes++
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLimitCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcTotalResult2385 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByNS          map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleSvcTotal2385(w http.ResponseWriter, r *http.Request) {
	result := SvcTotalResult2385{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByNS[svc.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
