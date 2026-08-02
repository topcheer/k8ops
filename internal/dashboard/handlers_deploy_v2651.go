package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.51 Deployment: STSServiceName, DSTemplateHash, DeployStrategyType

type STSServiceName2651Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     STSServiceName2651Summary `json:"summary"`
	Items       []STSServiceName2651Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type STSServiceName2651Summary struct {
	TotalSTS       int `json:"totalSTS"`
	WithSvcName    int `json:"withSvcName"`
	WithoutSvcName int `json:"withoutSvcName"`
}

type STSServiceName2651Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	ServiceName string `json:"serviceName"`
}

func (s *Server) handleSTSServiceName2651(w http.ResponseWriter, r *http.Request) {
	result := STSServiceName2651Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range stss.Items {
			result.Summary.TotalSTS++
			if sts.Spec.ServiceName != "" {
				result.Summary.WithSvcName++
			} else {
				result.Summary.WithoutSvcName++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, STSServiceName2651Item{
					Name: sts.Name, Namespace: sts.Namespace, ServiceName: sts.Spec.ServiceName,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSTemplateHash2651Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     DSTemplateHash2651Summary `json:"summary"`
	Items       []DSTemplateHash2651Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type DSTemplateHash2651Summary struct {
	TotalDS  int `json:"totalDS"`
	WithHash int `json:"withHash"`
}

type DSTemplateHash2651Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HashLabel string `json:"hashLabel"`
}

func (s *Server) handleDSTemplateHash2651(w http.ResponseWriter, r *http.Request) {
	result := DSTemplateHash2651Result{ScannedAt: time.Now()}
	dss, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			result.Summary.TotalDS++
			hash := ds.Labels["pod-template-hash"]
			if hash != "" {
				result.Summary.WithHash++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DSTemplateHash2651Item{
					Name: ds.Name, Namespace: ds.Namespace, HashLabel: hash,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployStrategyType2651Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     DeployStrategyType2651Summary `json:"summary"`
	Items       []DeployStrategyType2651Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type DeployStrategyType2651Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	RollingUpdate    int `json:"rollingUpdate"`
	Recreate         int `json:"recreate"`
}

type DeployStrategyType2651Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
}

func (s *Server) handleDeployStrategyType2651(w http.ResponseWriter, r *http.Request) {
	result := DeployStrategyType2651Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			st := string(dep.Spec.Strategy.Type)
			if st == "RollingUpdate" {
				result.Summary.RollingUpdate++
			} else {
				result.Summary.Recreate++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployStrategyType2651Item{
					Name: dep.Name, Namespace: dep.Namespace, Strategy: st,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
