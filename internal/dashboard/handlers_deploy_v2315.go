package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.15 Deployment: Deployment Available Ratio, STS Generation Sync, DS Number Available
type DepAvailRatioResult2315 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int   `json:"totalDeployments"`
		TotalReps    int32 `json:"totalReplicas"`
		TotalAvail   int32 `json:"totalAvailable"`
		TotalUnavail int32 `json:"totalUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleDepAvailRatio2315(w http.ResponseWriter, r *http.Request) {
	result := DepAvailRatioResult2315{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.TotalReps += dep.Status.Replicas
		result.Summary.TotalAvail += dep.Status.AvailableReplicas
		result.Summary.TotalUnavail += dep.Status.UnavailableReplicas
	}
	score := 100
	if result.Summary.TotalReps > 0 {
		score = int(result.Summary.TotalAvail * 100 / result.Summary.TotalReps)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSGenSyncResult2315 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		InSync   int `json:"generationInSync"`
	} `json:"summary"`
}

func (s *Server) handleSTSGenSync2315(w http.ResponseWriter, r *http.Request) {
	result := STSGenSyncResult2315{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Generation == sts.Status.ObservedGeneration {
			result.Summary.InSync++
		}
	}
	score := 100
	if result.Summary.TotalSTS > 0 {
		score = result.Summary.InSync * 100 / result.Summary.TotalSTS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DSNumAvailResult2315 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		DesiredNum   int `json:"totalDesiredNumber"`
		AvailableNum int `json:"totalNumberAvailable"`
	} `json:"summary"`
}

func (s *Server) handleDSNumAvail2315(w http.ResponseWriter, r *http.Request) {
	result := DSNumAvailResult2315{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.DesiredNum += int(ds.Status.DesiredNumberScheduled)
		result.Summary.AvailableNum += int(ds.Status.NumberAvailable)
	}
	score := 100
	if result.Summary.DesiredNum > 0 {
		score = result.Summary.AvailableNum * 100 / result.Summary.DesiredNum
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
