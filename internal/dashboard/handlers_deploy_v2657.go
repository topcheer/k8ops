package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.57 Deployment: STSPartitionOrd, DSNumberAvailable, DeployReplicaStatus

type STSPartitionOrd2657Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     STSPartitionOrd2657Summary `json:"summary"`
	Items       []STSPartitionOrd2657Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type STSPartitionOrd2657Summary struct {
	TotalSTS      int `json:"totalSTS"`
	WithPartition int `json:"withPartition"`
	NoPartition   int `json:"noPartition"`
}

type STSPartitionOrd2657Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Partition int32  `json:"partition"`
}

func (s *Server) handleSTSPartitionOrd2657(w http.ResponseWriter, r *http.Request) {
	result := STSPartitionOrd2657Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range stss.Items {
			result.Summary.TotalSTS++
			partition := int32(0)
			if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
				partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
				result.Summary.WithPartition++
			} else {
				result.Summary.NoPartition++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, STSPartitionOrd2657Item{
					Name: sts.Name, Namespace: sts.Namespace, Partition: partition,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNumberAvailable2657Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     DSNumberAvailable2657Summary `json:"summary"`
	Items       []DSNumberAvailable2657Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type DSNumberAvailable2657Summary struct {
	TotalDS         int `json:"totalDS"`
	AllAvailable    int `json:"allAvailable"`
	SomeUnavailable int `json:"someUnavailable"`
}

type DSNumberAvailable2657Item struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	NumberAvailable  int32  `json:"numberAvailable"`
	DesiredScheduled int32  `json:"desiredScheduled"`
}

func (s *Server) handleDSNumberAvailable2657(w http.ResponseWriter, r *http.Request) {
	result := DSNumberAvailable2657Result{ScannedAt: time.Now()}
	dss, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			result.Summary.TotalDS++
			if ds.Status.NumberAvailable >= ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
				result.Summary.AllAvailable++
			} else {
				result.Summary.SomeUnavailable++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DSNumberAvailable2657Item{
					Name: ds.Name, Namespace: ds.Namespace,
					NumberAvailable: ds.Status.NumberAvailable, DesiredScheduled: ds.Status.DesiredNumberScheduled,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployReplicaStatus2657Result struct {
	ScannedAt   time.Time                      `json:"scannedAt"`
	Summary     DeployReplicaStatus2657Summary `json:"summary"`
	Items       []DeployReplicaStatus2657Item  `json:"items"`
	HealthScore int                            `json:"healthScore"`
	Grade       string                         `json:"grade"`
}

type DeployReplicaStatus2657Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	FullyReady       int `json:"fullyReady"`
	NotReady         int `json:"notReady"`
}

type DeployReplicaStatus2657Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Replicas      int32  `json:"replicas"`
}

func (s *Server) handleDeployReplicaStatus2657(w http.ResponseWriter, r *http.Request) {
	result := DeployReplicaStatus2657Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			ready := dep.Status.ReadyReplicas
			replicas := dep.Status.Replicas
			if replicas == 0 || ready >= replicas {
				result.Summary.FullyReady++
			} else {
				result.Summary.NotReady++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployReplicaStatus2657Item{
					Name: dep.Name, Namespace: dep.Namespace, ReadyReplicas: ready, Replicas: replicas,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
