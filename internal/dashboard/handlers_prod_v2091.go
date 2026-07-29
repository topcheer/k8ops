package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.91 — Product Dimension (Round 35)
// 1. Workload Label Compliance — recommended labels coverage
// 2. Service Port Collision Detector — duplicate port bindings
// 3. Pod Restart Budget — total restarts per namespace
// ============================================================

type LabelCompResult2091 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         LabelCompSummary2091 `json:"summary"`
	MissingLabels   []LabelCompEntry2091 `json:"missingLabels"`
	Recommendations []string             `json:"recommendations"`
}

type LabelCompSummary2091 struct {
	TotalDeploys  int `json:"totalDeployments"`
	WithAppLabel  int `json:"withAppLabel"`
	WithVerLabel  int `json:"withVersionLabel"`
	MissingLabels int `json:"missingLabels"`
}

type LabelCompEntry2091 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Missing   string `json:"missing"`
}

func (s *Server) handleLabelComp2091(w http.ResponseWriter, r *http.Request) {
	result := LabelCompResult2091{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		labels := dep.Spec.Template.Labels
		missing := []string{}
		if labels["app"] == "" && labels["app.kubernetes.io/name"] == "" {
			missing = append(missing, "app")
		} else {
			result.Summary.WithAppLabel++
		}
		if labels["version"] == "" && labels["app.kubernetes.io/version"] == "" {
			missing = append(missing, "version")
		} else {
			result.Summary.WithVerLabel++
		}
		if len(missing) > 0 {
			result.Summary.MissingLabels++
			result.MissingLabels = append(result.MissingLabels, LabelCompEntry2091{
				Name: dep.Name, Namespace: dep.Namespace, Missing: fmt.Sprintf("%v", missing),
			})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.MissingLabels, func(i, j int) bool { return result.MissingLabels[i].Namespace < result.MissingLabels[j].Namespace })
	writeJSON(w, result)
}

// 2. Service Port Collision Detector
type PortCollResult2091 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PortCollSummary2091 `json:"summary"`
	Collisions      []PortCollEntry2091 `json:"collisions"`
	Recommendations []string            `json:"recommendations"`
}

type PortCollSummary2091 struct {
	TotalServices int `json:"totalServices"`
	Collisions    int `json:"collisions"`
}

type PortCollEntry2091 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Port      int32  `json:"port"`
}

func (s *Server) handlePortColl2091(w http.ResponseWriter, r *http.Request) {
	result := PortCollResult2091{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	portUsers := make(map[int32][]string)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			key := svc.Namespace + "/" + svc.Name
			portUsers[p.Port] = append(portUsers[p.Port], key)
		}
	}
	for port, users := range portUsers {
		if len(users) > 1 {
			result.Summary.Collisions++
			result.Collisions = append(result.Collisions, PortCollEntry2091{Service: users[0], Port: port})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Restart Budget
type RestartBudgetResult2091 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         RestartBudgetSummary2091 `json:"summary"`
	TopNS           []RestartBudgetEntry2091 `json:"topNamespaces"`
	Recommendations []string                 `json:"recommendations"`
}

type RestartBudgetSummary2091 struct {
	TotalPods     int   `json:"totalPods"`
	TotalRestarts int32 `json:"totalRestarts"`
}

type RestartBudgetEntry2091 struct {
	Namespace string `json:"namespace"`
	Restarts  int32  `json:"restarts"`
}

func (s *Server) handleRestartBudget2091(w http.ResponseWriter, r *http.Request) {
	result := RestartBudgetResult2091{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsRestarts := make(map[string]int32)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalRestarts += cs.RestartCount
			nsRestarts[pod.Namespace] += cs.RestartCount
		}
	}
	for ns, restarts := range nsRestarts {
		result.TopNS = append(result.TopNS, RestartBudgetEntry2091{Namespace: ns, Restarts: restarts})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Restarts > result.TopNS[j].Restarts })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
