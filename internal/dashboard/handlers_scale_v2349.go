package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.49 Scalability: Top Node by Container Count, Cluster HPA Coverage, Namespace Replica Distribution
type TopNodeContainerResult2349 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node      string `json:"node"`
		CtnrCount int    `json:"containerCount"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeContainer2349(w http.ResponseWriter, r *http.Request) {
	result := TopNodeContainerResult2349{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeCtnrs := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for range pod.Spec.Containers {
			nodeCtnrs[pod.Spec.NodeName]++
		}
	}
	result.Summary.TotalNodes = len(nodeCtnrs)
	for node, count := range nodeCtnrs {
		result.TopNodes = append(result.TopNodes, struct {
			Node      string `json:"node"`
			CtnrCount int    `json:"containerCount"`
		}{node, count})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].CtnrCount > result.TopNodes[j].CtnrCount })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HPACoverageResult2349 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithHPA      int `json:"withHPA"`
	} `json:"summary"`
}

func (s *Server) handleHPACoverage2349(w http.ResponseWriter, r *http.Request) {
	result := HPACoverageResult2349{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	hpaSet := make(map[string]bool)
	for _, hpa := range hpaList.Items {
		if hpa.Spec.ScaleTargetRef.Kind == "Deployment" {
			hpaSet[hpa.Namespace+"/"+hpa.Spec.ScaleTargetRef.Name] = true
		}
	}
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if hpaSet[dep.Namespace+"/"+dep.Name] {
			result.Summary.WithHPA++
		}
	}
	score := 100
	if result.Summary.TotalDeploys > 0 {
		score = result.Summary.WithHPA * 100 / result.Summary.TotalDeploys
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NSReplicaDistResult2349 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS       int `json:"totalNS"`
		TotalReplicas int `json:"totalReplicas"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		Replicas  int32  `json:"replicas"`
	} `json:"topNS"`
}

func (s *Server) handleNSReplicaDist2349(w http.ResponseWriter, r *http.Request) {
	result := NSReplicaDistResult2349{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsReps := make(map[string]int32)
	for _, dep := range depList.Items {
		nsReps[dep.Namespace] += dep.Status.Replicas
		result.Summary.TotalReplicas += int(dep.Status.Replicas)
	}
	result.Summary.TotalNS = len(nsReps)
	for ns, reps := range nsReps {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			Replicas  int32  `json:"replicas"`
		}{ns, reps})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Replicas > result.TopNS[j].Replicas })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
