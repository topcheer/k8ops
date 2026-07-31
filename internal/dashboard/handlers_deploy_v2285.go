package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.85 Deployment: DS NodeSelector Census, Deployment Paused Status, STS Service Name Link
type DSNodeSelectorResult2285 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS          int `json:"totalDS"`
		WithNodeSelector int `json:"withNodeSelector"`
	} `json:"summary"`
}

func (s *Server) handleDSNodeSelector2285(w http.ResponseWriter, r *http.Request) {
	result := DSNodeSelectorResult2285{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			result.Summary.WithNodeSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DepPausedResult2285 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeployments int `json:"totalDeployments"`
		Paused           int `json:"paused"`
	} `json:"summary"`
}

func (s *Server) handleDepPaused2285(w http.ResponseWriter, r *http.Request) {
	result := DepPausedResult2285{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++
		if dep.Spec.Paused {
			result.Summary.Paused++
		}
	}
	score := 100
	if result.Summary.Paused > 0 {
		score = 70
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSSvcLinkResult2285 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS        int `json:"totalSTS"`
		WithServiceName int `json:"withServiceName"`
	} `json:"summary"`
}

func (s *Server) handleSTSSvcLink2285(w http.ResponseWriter, r *http.Request) {
	result := STSSvcLinkResult2285{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName != "" {
			result.Summary.WithServiceName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
