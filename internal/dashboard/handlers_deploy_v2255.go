package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.55 Deployment: Deployment Available Cond, STS Replicas Status, RS Full Status
type DepAvailCondResult2255 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Available    int `json:"available"`
		NotAvailable int `json:"notAvailable"`
	} `json:"summary"`
}

func (s *Server) handleDepAvailCond2255(w http.ResponseWriter, r *http.Request) {
	result := DepAvailCondResult2255{ScannedAt: time.Now()}
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		avail := false
		for _, cond := range dep.Status.Conditions {
			if string(cond.Type) == "Available" && cond.Status == "True" {
				avail = true
			}
		}
		if avail {
			result.Summary.Available++
		} else {
			result.Summary.NotAvailable++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSRepStatusResult2255 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int   `json:"totalSTS"`
		TotalReplicas int32 `json:"totalReplicas"`
		TotalReady    int32 `json:"totalReady"`
	} `json:"summary"`
}

func (s *Server) handleSTSRepStatus2255(w http.ResponseWriter, r *http.Request) {
	result := STSRepStatusResult2255{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReplicas += *sts.Spec.Replicas
		}
		result.Summary.TotalReady += sts.Status.ReadyReplicas
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RSFullStatusResult2255 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		FullyReady int `json:"fullyReady"`
	} `json:"summary"`
}

func (s *Server) handleRSFullStatus2255(w http.ResponseWriter, r *http.Request) {
	result := RSFullStatusResult2255{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas == nil || *rs.Spec.Replicas == 0 {
			continue
		}
		result.Summary.TotalRS++
		if rs.Status.ReadyReplicas >= *rs.Spec.Replicas {
			result.Summary.FullyReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
