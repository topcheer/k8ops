package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.45 Deployment: STSPodManagementPolicy, DSMaxUnavailable, DeployMinReady

type STSPodMgmtPolicy2645Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     STSPodMgmtPolicy2645Summary `json:"summary"`
	Items       []STSPodMgmtPolicy2645Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type STSPodMgmtPolicy2645Summary struct {
	TotalSTS     int `json:"totalSTS"`
	OrderedReady int `json:"orderedReady"`
	Parallel     int `json:"parallel"`
}

type STSPodMgmtPolicy2645Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	PodMgmtPolicy string `json:"podMgmtPolicy"`
}

func (s *Server) handleSTSPodMgmtPolicy2645(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtPolicy2645Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range stss.Items {
			result.Summary.TotalSTS++
			policy := string(sts.Spec.PodManagementPolicy)
			if policy == "Parallel" {
				result.Summary.Parallel++
			} else {
				result.Summary.OrderedReady++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, STSPodMgmtPolicy2645Item{
					Name: sts.Name, Namespace: sts.Namespace, PodMgmtPolicy: policy,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSMaxUnavailable2645Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     DSMaxUnavailable2645Summary `json:"summary"`
	Items       []DSMaxUnavailable2645Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type DSMaxUnavailable2645Summary struct {
	TotalDS        int `json:"totalDS"`
	WithMaxUnavail int `json:"withMaxUnavailable"`
}

type DSMaxUnavailable2645Item struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	MaxUnavailable string `json:"maxUnavailable"`
}

func (s *Server) handleDSMaxUnavailable2645(w http.ResponseWriter, r *http.Request) {
	result := DSMaxUnavailable2645Result{ScannedAt: time.Now()}
	dss, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			result.Summary.TotalDS++
			mu := "1"
			if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
				mu = ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.String()
				result.Summary.WithMaxUnavail++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DSMaxUnavailable2645Item{
					Name: ds.Name, Namespace: ds.Namespace, MaxUnavailable: mu,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployMinReady2645Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     DeployMinReady2645Summary `json:"summary"`
	Items       []DeployMinReady2645Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type DeployMinReady2645Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	DefaultMinReady  int `json:"defaultMinReady"`
	CustomMinReady   int `json:"customMinReady"`
}

type DeployMinReady2645Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	MinReady  int32  `json:"minReady"`
}

func (s *Server) handleDeployMinReady2645(w http.ResponseWriter, r *http.Request) {
	result := DeployMinReady2645Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			mr := int32(0)
			if dep.Spec.MinReadySeconds > 0 {
				mr = dep.Spec.MinReadySeconds
				result.Summary.CustomMinReady++
			} else {
				result.Summary.DefaultMinReady++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployMinReady2645Item{
					Name: dep.Name, Namespace: dep.Namespace, MinReady: mr,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
