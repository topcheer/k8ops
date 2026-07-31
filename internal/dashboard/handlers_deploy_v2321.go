package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.21 Deployment: Deployment Collision Check, STS Collision Check, RS Replica Status Distribution
type DepCollisionResult2321 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Collisions   int `json:"withLabelCollisions"`
	} `json:"summary"`
}

func (s *Server) handleDepCollision2321(w http.ResponseWriter, r *http.Request) {
	result := DepCollisionResult2321{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	labelMap := make(map[string]int)
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		labelKey := dep.Namespace + "/" + dep.Spec.Selector.String()
		labelMap[labelKey]++
	}
	for _, count := range labelMap {
		if count > 1 {
			result.Summary.Collisions += count
		}
	}
	score := 100
	if result.Summary.TotalDeploys > 0 && result.Summary.Collisions > 0 {
		score = 100 - (result.Summary.Collisions*50)/result.Summary.TotalDeploys
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSCollisionResult2321 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		Collisions int `json:"withSelectorCollisions"`
	} `json:"summary"`
}

func (s *Server) handleSTSCollision2321(w http.ResponseWriter, r *http.Request) {
	result := STSCollisionResult2321{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	selectorMap := make(map[string]int)
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		selectorKey := sts.Namespace + "/" + sts.Spec.Selector.String()
		selectorMap[selectorKey]++
	}
	for _, count := range selectorMap {
		if count > 1 {
			result.Summary.Collisions += count
		}
	}
	score := 100
	if result.Summary.TotalSTS > 0 && result.Summary.Collisions > 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RSReplicaStatusResult2321 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS       int   `json:"totalRS"`
		TotalReplicas int32 `json:"totalReplicas"`
		TotalReady    int32 `json:"totalReady"`
		TotalAvail    int32 `json:"totalAvailable"`
	} `json:"summary"`
}

func (s *Server) handleRSReplicaStatus2321(w http.ResponseWriter, r *http.Request) {
	result := RSReplicaStatusResult2321{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 && len(rs.OwnerReferences) > 0 {
			continue
		}
		result.Summary.TotalRS++
		result.Summary.TotalReplicas += rs.Status.Replicas
		result.Summary.TotalReady += rs.Status.ReadyReplicas
		result.Summary.TotalAvail += rs.Status.AvailableReplicas
	}
	score := 100
	if result.Summary.TotalReplicas > 0 {
		score = int(result.Summary.TotalReady * 100 / result.Summary.TotalReplicas)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
