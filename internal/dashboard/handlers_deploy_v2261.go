package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.61 Deployment: Deployment Strategy Census, STS Update Strategy, DS Update Strategy
type DepStrategyResult2261 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeployments int            `json:"totalDeployments"`
		ByStrategy       map[string]int `json:"byStrategy"`
	} `json:"summary"`
}

func (s *Server) handleDepStrategy2261(w http.ResponseWriter, r *http.Request) {
	result := DepStrategyResult2261{ScannedAt: time.Now()}
	result.Summary.ByStrategy = make(map[string]int)
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++
		result.Summary.ByStrategy[string(dep.Spec.Strategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSUpdateStrategyResult2261 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int            `json:"totalSTS"`
		ByStrategy    map[string]int `json:"byStrategy"`
		WithPartition int            `json:"withPartition"`
	} `json:"summary"`
}

func (s *Server) handleSTSUpdateStrategy2261(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateStrategyResult2261{ScannedAt: time.Now()}
	result.Summary.ByStrategy = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByStrategy[string(sts.Spec.UpdateStrategy.Type)]++
		if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *sts.Spec.UpdateStrategy.RollingUpdate.Partition > 0 {
			result.Summary.WithPartition++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdateStrategyResult2261 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int            `json:"totalDS"`
		ByStrategy map[string]int `json:"byStrategy"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdateStrategy2261(w http.ResponseWriter, r *http.Request) {
	result := DSUpdateStrategyResult2261{ScannedAt: time.Now()}
	result.Summary.ByStrategy = make(map[string]int)
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.ByStrategy[string(ds.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
