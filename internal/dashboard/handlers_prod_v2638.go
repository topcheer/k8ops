package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.38 Product: PodRestartPolicyAudit, DeploymentRevisionHistoryLimit, ServiceExternalTrafficPolicy

type PodRestartPolicy2638Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     PodRestartPolicy2638Summary `json:"summary"`
	Items       []PodRestartPolicy2638Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type PodRestartPolicy2638Summary struct {
	TotalPods    int `json:"totalPods"`
	AlwaysPolicy int `json:"alwaysPolicy"`
	OnFailure    int `json:"onFailure"`
	NeverPolicy  int `json:"neverPolicy"`
}

type PodRestartPolicy2638Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	RestartPolicy string `json:"restartPolicy"`
}

func (s *Server) handlePodRestartPolicy2638(w http.ResponseWriter, r *http.Request) {
	result := PodRestartPolicy2638Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			switch pod.Spec.RestartPolicy {
			case corev1.RestartPolicyAlways:
				result.Summary.AlwaysPolicy++
			case corev1.RestartPolicyOnFailure:
				result.Summary.OnFailure++
			case corev1.RestartPolicyNever:
				result.Summary.NeverPolicy++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodRestartPolicy2638Item{
					Name: pod.Name, Namespace: pod.Namespace, RestartPolicy: string(pod.Spec.RestartPolicy),
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployRevHistLimit2638Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     DeployRevHistLimit2638Summary `json:"summary"`
	Items       []DeployRevHistLimit2638Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type DeployRevHistLimit2638Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithLimit        int `json:"withLimit"`
	Unlimited        int `json:"unlimited"`
}

type DeployRevHistLimit2638Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Limit     int32  `json:"limit"`
}

func (s *Server) handleDeployRevHistLimit2638(w http.ResponseWriter, r *http.Request) {
	result := DeployRevHistLimit2638Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			limit := int32(10)
			if dep.Spec.RevisionHistoryLimit != nil {
				limit = *dep.Spec.RevisionHistoryLimit
				result.Summary.WithLimit++
			} else {
				result.Summary.Unlimited++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployRevHistLimit2638Item{
					Name: dep.Name, Namespace: dep.Namespace, Limit: limit,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcExternalTrafficPolicy2638Result struct {
	ScannedAt   time.Time                           `json:"scannedAt"`
	Summary     SvcExternalTrafficPolicy2638Summary `json:"summary"`
	Items       []SvcExternalTrafficPolicy2638Item  `json:"items"`
	HealthScore int                                 `json:"healthScore"`
	Grade       string                              `json:"grade"`
}

type SvcExternalTrafficPolicy2638Summary struct {
	TotalServices int `json:"totalServices"`
	ClusterPolicy int `json:"clusterPolicy"`
	LocalPolicy   int `json:"localPolicy"`
}

type SvcExternalTrafficPolicy2638Item struct {
	Name                  string `json:"name"`
	Namespace             string `json:"namespace"`
	ExternalTrafficPolicy string `json:"externalTrafficPolicy"`
}

func (s *Server) handleSvcExternalTrafficPolicy2638(w http.ResponseWriter, r *http.Request) {
	result := SvcExternalTrafficPolicy2638Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			switch svc.Spec.ExternalTrafficPolicy {
			case corev1.ServiceExternalTrafficPolicyTypeCluster:
				result.Summary.ClusterPolicy++
			case corev1.ServiceExternalTrafficPolicyTypeLocal:
				result.Summary.LocalPolicy++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcExternalTrafficPolicy2638Item{
					Name: svc.Name, Namespace: svc.Namespace, ExternalTrafficPolicy: string(svc.Spec.ExternalTrafficPolicy),
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
