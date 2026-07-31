package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.27 Deployment: STS Current Revision, DS Updated Number, Job Failing Rate
type STSRevisionResult2327 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS        int   `json:"totalSTS"`
		TotalRevisions  int32 `json:"totalCurrentRevision"`
		TotalUpdatedRev int32 `json:"totalUpdateRevision"`
	} `json:"summary"`
}

func (s *Server) handleSTSRevision2327(w http.ResponseWriter, r *http.Request) {
	result := STSRevisionResult2327{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalRevisions += sts.Status.Replicas
		result.Summary.TotalUpdatedRev += sts.Status.UpdatedReplicas
	}
	score := 100
	if result.Summary.TotalRevisions > 0 {
		score = int(result.Summary.TotalUpdatedRev * 100 / result.Summary.TotalRevisions)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DSUpdatedNumResult2327 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		TotalDesired int `json:"totalDesired"`
		TotalUpdated int `json:"totalUpdatedScheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdatedNum2327(w http.ResponseWriter, r *http.Request) {
	result := DSUpdatedNumResult2327{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalDesired += int(ds.Status.DesiredNumberScheduled)
		result.Summary.TotalUpdated += int(ds.Status.UpdatedNumberScheduled)
	}
	score := 100
	if result.Summary.TotalDesired > 0 {
		score = result.Summary.TotalUpdated * 100 / result.Summary.TotalDesired
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type JobFailRateResult2327 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int `json:"totalJobs"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		FailPct   int `json:"failPct"`
	} `json:"summary"`
}

func (s *Server) handleJobFailRate2327(w http.ResponseWriter, r *http.Request) {
	result := JobFailRateResult2327{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		for _, cond := range job.Status.Conditions {
			if string(cond.Type) == "Complete" && cond.Status == "True" {
				result.Summary.Succeeded++
			}
			if string(cond.Type) == "Failed" && cond.Status == "True" {
				result.Summary.Failed++
			}
		}
	}
	if result.Summary.TotalJobs > 0 {
		result.Summary.FailPct = result.Summary.Failed * 100 / result.Summary.TotalJobs
	}
	score := 100 - result.Summary.FailPct
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
