package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.67 Deployment: HPA Target Utilization Audit, PDB Min Available Census, Job Completion Status
type HPATargetUtilResult2267 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalHPA   int `json:"totalHPA"`
		WithCPU    int `json:"withCPUTarget"`
		WithMemory int `json:"withMemTarget"`
	} `json:"summary"`
	Items []struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		TargetKind string `json:"targetKind"`
	} `json:"items"`
}

func (s *Server) handleHPATargetUtil2267(w http.ResponseWriter, r *http.Request) {
	result := HPATargetUtilResult2267{ScannedAt: time.Now()}
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPA++
		for _, metric := range hpa.Spec.Metrics {
			if metric.Type == "Resource" && metric.Resource != nil {
				if metric.Resource.Name == "cpu" {
					result.Summary.WithCPU++
				}
				if metric.Resource.Name == "memory" {
					result.Summary.WithMemory++
				}
			}
		}
		result.Items = append(result.Items, struct {
			Name       string `json:"name"`
			Namespace  string `json:"namespace"`
			TargetKind string `json:"targetKind"`
		}{hpa.Name, hpa.Namespace, string(hpa.Spec.ScaleTargetRef.Kind)})
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PDBMinAvailResult2267 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPDB       int `json:"totalPDB"`
		WithMinAvail   int `json:"withMinAvailable"`
		WithMaxUnavail int `json:"withMaxUnavailable"`
	} `json:"summary"`
}

func (s *Server) handlePDBMinAvail2267(w http.ResponseWriter, r *http.Request) {
	result := PDBMinAvailResult2267{ScannedAt: time.Now()}
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	for _, pdb := range pdbList.Items {
		result.Summary.TotalPDB++
		if pdb.Spec.MinAvailable != nil {
			result.Summary.WithMinAvail++
		}
		if pdb.Spec.MaxUnavailable != nil {
			result.Summary.WithMaxUnavail++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobCompletionResult2267 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int `json:"totalJobs"`
		Completed int `json:"completed"`
		Running   int `json:"running"`
		Failed    int `json:"failed"`
	} `json:"summary"`
}

func (s *Server) handleJobCompletion2267(w http.ResponseWriter, r *http.Request) {
	result := JobCompletionResult2267{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		for _, cond := range job.Status.Conditions {
			if string(cond.Type) == "Complete" && cond.Status == "True" {
				result.Summary.Completed++
			}
			if string(cond.Type) == "Failed" && cond.Status == "True" {
				result.Summary.Failed++
			}
		}
		if job.Status.Active > 0 {
			result.Summary.Running++
		}
	}
	score := 100
	if result.Summary.TotalJobs > 0 && result.Summary.Failed > 0 {
		score = 100 - (result.Summary.Failed*100)/result.Summary.TotalJobs
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
