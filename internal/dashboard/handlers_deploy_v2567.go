package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.67 Deployment: RS Annotations Count, STS ServiceName Detail, DS NodeSelector Summary
type RSAnnotationsResult2567 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int `json:"totalRS"`
		TotalAnnots int `json:"totalAnnotations"`
	}
}

func (s *Server) handleRSAnnotations2567(w http.ResponseWriter, r *http.Request) {
	result := RSAnnotationsResult2567{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalAnnots += len(rs.Annotations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSServiceNameResult2567 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		WithSvcName int `json:"withServiceName"`
	}
}

func (s *Server) handleSTSServiceName2567(w http.ResponseWriter, r *http.Request) {
	result := STSServiceNameResult2567{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName != "" {
			result.Summary.WithSvcName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNodeSelectorResult2567 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int            `json:"totalDS"`
		BySelector map[string]int `json:"byNodeSelectorKey"`
	}
}

func (s *Server) handleDSNodeSelector2567(w http.ResponseWriter, r *http.Request) {
	result := DSNodeSelectorResult2567{ScannedAt: time.Now()}
	result.Summary.BySelector = make(map[string]int)
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		for k := range ds.Spec.Template.Spec.NodeSelector {
			result.Summary.BySelector[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
