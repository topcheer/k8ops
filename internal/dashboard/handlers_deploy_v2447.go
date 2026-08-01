package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.47 Deployment: Deployment Partition Count, STS PodManagementPolicy, DS UpdateStrategy Type
type DepPartitionResult2447 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDep      int `json:"totalDeployments"`
		WithPartition int `json:"withPartition"`
	} `json:"summary"`
}

func (s *Server) handleDepPartition2447(w http.ResponseWriter, r *http.Request) {
	result := DepPartitionResult2447{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDep++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			result.Summary.WithPartition++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPodMgmtResult2447 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByPolicy map[string]int `json:"byPodManagementPolicy"`
	} `json:"summary"`
}

func (s *Server) handleSTSPodMgmt2447(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtResult2447{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByPolicy[string(sts.Spec.PodManagementPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdateStrategyResult2447 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int            `json:"totalDS"`
		ByType  map[string]int `json:"byUpdateStrategyType"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdateStrategy2447(w http.ResponseWriter, r *http.Request) {
	result := DSUpdateStrategyResult2447{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.ByType[string(ds.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
