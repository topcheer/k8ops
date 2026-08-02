package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.44 Product: PodHostNetworkAudit, DeploymentMaxSurge, ServiceSessionAffinity

type PodHostNetwork2644Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     PodHostNetwork2644Summary `json:"summary"`
	Items       []PodHostNetwork2644Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type PodHostNetwork2644Summary struct {
	TotalPods   int `json:"totalPods"`
	HostNetwork int `json:"hostNetwork"`
	PodNetwork  int `json:"podNetwork"`
}

type PodHostNetwork2644Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	HostNetwork bool   `json:"hostNetwork"`
}

func (s *Server) handlePodHostNetwork2644(w http.ResponseWriter, r *http.Request) {
	result := PodHostNetwork2644Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			if pod.Spec.HostNetwork {
				result.Summary.HostNetwork++
			} else {
				result.Summary.PodNetwork++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodHostNetwork2644Item{
					Name: pod.Name, Namespace: pod.Namespace, HostNetwork: pod.Spec.HostNetwork,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployMaxSurge2644Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     DeployMaxSurge2644Summary `json:"summary"`
	Items       []DeployMaxSurge2644Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type DeployMaxSurge2644Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithMaxSurge     int `json:"withMaxSurge"`
}

type DeployMaxSurge2644Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	MaxSurge  string `json:"maxSurge"`
}

func (s *Server) handleDeployMaxSurge2644(w http.ResponseWriter, r *http.Request) {
	result := DeployMaxSurge2644Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			surge := "25%"
			if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
				surge = dep.Spec.Strategy.RollingUpdate.MaxSurge.String()
				result.Summary.WithMaxSurge++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployMaxSurge2644Item{
					Name: dep.Name, Namespace: dep.Namespace, MaxSurge: surge,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcSessionAffinity2644Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     SvcSessionAffinity2644Summary `json:"summary"`
	Items       []SvcSessionAffinity2644Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type SvcSessionAffinity2644Summary struct {
	TotalServices int `json:"totalServices"`
	ClientIPAffin int `json:"clientIPAffinity"`
	NoneAffinity  int `json:"noneAffinity"`
}

type SvcSessionAffinity2644Item struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	SessionAffinity string `json:"sessionAffinity"`
}

func (s *Server) handleSvcSessionAffinity2644(w http.ResponseWriter, r *http.Request) {
	result := SvcSessionAffinity2644Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			affinity := string(svc.Spec.SessionAffinity)
			if affinity == "ClientIP" {
				result.Summary.ClientIPAffin++
			} else {
				result.Summary.NoneAffinity++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcSessionAffinity2644Item{
					Name: svc.Name, Namespace: svc.Namespace, SessionAffinity: affinity,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
