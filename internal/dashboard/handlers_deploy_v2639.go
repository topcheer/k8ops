package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.39 Deployment: STSUpdateStrategy, DSUpdateStrategy, DeploymentProgressDeadline

type STSUpdateStrategy2639Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     STSUpdateStrategy2639Summary `json:"summary"`
	Items       []STSUpdateStrategy2639Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type STSUpdateStrategy2639Summary struct {
	TotalSTS   int `json:"totalSTS"`
	RollingUpd int `json:"rollingUpdate"`
	OnDelete   int `json:"onDelete"`
}

type STSUpdateStrategy2639Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
}

func (s *Server) handleSTSUpdateStrategy2639(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateStrategy2639Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range stss.Items {
			result.Summary.TotalSTS++
			strategy := string(sts.Spec.UpdateStrategy.Type)
			if strategy == "RollingUpdate" {
				result.Summary.RollingUpd++
			} else {
				result.Summary.OnDelete++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, STSUpdateStrategy2639Item{
					Name: sts.Name, Namespace: sts.Namespace, Strategy: strategy,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdateStrategy2639Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     DSUpdateStrategy2639Summary `json:"summary"`
	Items       []DSUpdateStrategy2639Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type DSUpdateStrategy2639Summary struct {
	TotalDS    int `json:"totalDS"`
	RollingUpd int `json:"rollingUpdate"`
	OnDelete   int `json:"onDelete"`
}

type DSUpdateStrategy2639Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
}

func (s *Server) handleDSUpdateStrategy2639(w http.ResponseWriter, r *http.Request) {
	result := DSUpdateStrategy2639Result{ScannedAt: time.Now()}
	dss, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			result.Summary.TotalDS++
			strategy := string(ds.Spec.UpdateStrategy.Type)
			if strategy == "RollingUpdate" {
				result.Summary.RollingUpd++
			} else {
				result.Summary.OnDelete++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DSUpdateStrategy2639Item{
					Name: ds.Name, Namespace: ds.Namespace, Strategy: strategy,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployProgressDeadline2639Result struct {
	ScannedAt   time.Time                         `json:"scannedAt"`
	Summary     DeployProgressDeadline2639Summary `json:"summary"`
	Items       []DeployProgressDeadline2639Item  `json:"items"`
	HealthScore int                               `json:"healthScore"`
	Grade       string                            `json:"grade"`
}

type DeployProgressDeadline2639Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithDeadline     int `json:"withDeadline"`
	NoDeadline       int `json:"noDeadline"`
}

type DeployProgressDeadline2639Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Deadline  int32  `json:"deadline"`
}

func (s *Server) handleDeployProgressDeadline2639(w http.ResponseWriter, r *http.Request) {
	result := DeployProgressDeadline2639Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			if dep.Spec.ProgressDeadlineSeconds != nil {
				result.Summary.WithDeadline++
				if len(result.Items) < 50 {
					result.Items = append(result.Items, DeployProgressDeadline2639Item{
						Name: dep.Name, Namespace: dep.Namespace, Deadline: *dep.Spec.ProgressDeadlineSeconds,
					})
				}
			} else {
				result.Summary.NoDeadline++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
