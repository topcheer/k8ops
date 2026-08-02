package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.56 Product: PodTerminationGrace, DeploymentConditions, ServiceIPFamily

type PodTermGrace2656Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     PodTermGrace2656Summary `json:"summary"`
	Items       []PodTermGrace2656Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type PodTermGrace2656Summary struct {
	TotalPods  int `json:"totalPods"`
	DefaultSec int `json:"defaultSec"`
	CustomSec  int `json:"customSec"`
}

type PodTermGrace2656Item struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	TerminationGraceS int64  `json:"terminationGraceSeconds"`
}

func (s *Server) handlePodTermGrace2656(w http.ResponseWriter, r *http.Request) {
	result := PodTermGrace2656Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			grace := int64(30)
			if pod.Spec.TerminationGracePeriodSeconds != nil {
				grace = *pod.Spec.TerminationGracePeriodSeconds
				result.Summary.CustomSec++
			} else {
				result.Summary.DefaultSec++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodTermGrace2656Item{
					Name: pod.Name, Namespace: pod.Namespace, TerminationGraceS: grace,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployConditions2656Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     DeployConditions2656Summary `json:"summary"`
	Items       []DeployConditions2656Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type DeployConditions2656Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	Available        int `json:"available"`
	Progressing      int `json:"progressing"`
	ReplicaFailure   int `json:"replicaFailure"`
}

type DeployConditions2656Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Status    string `json:"status"`
}

func (s *Server) handleDeployConditions2656(w http.ResponseWriter, r *http.Request) {
	result := DeployConditions2656Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			for _, cond := range dep.Status.Conditions {
				if len(result.Items) < 50 {
					result.Items = append(result.Items, DeployConditions2656Item{
						Name: dep.Name, Namespace: dep.Namespace, Type: string(cond.Type), Status: string(cond.Status),
					})
				}
				switch cond.Type {
				case "Available":
					if cond.Status == "True" {
						result.Summary.Available++
					}
				case "Progressing":
					result.Summary.Progressing++
				case "ReplicaFailure":
					result.Summary.ReplicaFailure++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcIPFamily2656Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     SvcIPFamily2656Summary `json:"summary"`
	Items       []SvcIPFamily2656Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type SvcIPFamily2656Summary struct {
	TotalServices int `json:"totalServices"`
	IPv4Only      int `json:"ipv4Only"`
	IPv6Only      int `json:"ipv6Only"`
	DualStack     int `json:"dualStack"`
}

type SvcIPFamily2656Item struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	IPFamilies []string `json:"ipFamilies"`
}

func (s *Server) handleSvcIPFamily2656(w http.ResponseWriter, r *http.Request) {
	result := SvcIPFamily2656Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			families := make([]string, 0, len(svc.Spec.IPFamilies))
			for _, f := range svc.Spec.IPFamilies {
				families = append(families, string(f))
			}
			if len(families) == 1 {
				if families[0] == string(corev1.IPv4Protocol) {
					result.Summary.IPv4Only++
				} else {
					result.Summary.IPv6Only++
				}
			} else if len(families) > 1 {
				result.Summary.DualStack++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcIPFamily2656Item{
					Name: svc.Name, Namespace: svc.Namespace, IPFamilies: families,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
