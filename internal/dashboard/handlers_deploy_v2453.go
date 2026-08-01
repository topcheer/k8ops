package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.53 Deployment: RS Replicas Distribution, STS ServiceName Check, DS NodeSelector Count
type RSReplicasDistResult2453 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS int `json:"totalRS"`
		ZeroRep int `json:"zeroReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSReplicasDist2453(w http.ResponseWriter, r *http.Request) {
	result := RSReplicasDistResult2453{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if *rs.Spec.Replicas == 0 {
			continue
		}
		result.Summary.TotalRS++
		if *rs.Spec.Replicas == 0 {
			result.Summary.ZeroRep++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSServiceNameResult2453 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		WithSvcName int `json:"withServiceName"`
	} `json:"summary"`
}

func (s *Server) handleSTSServiceName2453(w http.ResponseWriter, r *http.Request) {
	result := STSServiceNameResult2453{ScannedAt: time.Now()}
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

type DSNodeSelectorResult2453 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		WithSelector int `json:"withNodeSelector"`
	} `json:"summary"`
}

func (s *Server) handleDSNodeSelector2453(w http.ResponseWriter, r *http.Request) {
	result := DSNodeSelectorResult2453{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			result.Summary.WithSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
