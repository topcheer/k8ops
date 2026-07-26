package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.92 — Documentation Dimension (Round 18)
// 1. Priority Class Catalog — all priority classes with values & defaults
// 2. Role Binding Catalog — RBAC binding inventory (who-can-do-what)
// 3. Endpoint Slice Catalog — endpoint slice discovery & routing info
// ============================================================

// ---------------------------------------------------------------
// 1. Priority Class Catalog
// ---------------------------------------------------------------

type PCClassResult1992 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PCClassSummary1992 `json:"summary"`
	Classes         []PCClassEntry1992 `json:"priorityClasses"`
	Recommendations []string           `json:"recommendations"`
}

type PCClassSummary1992 struct {
	TotalClasses   int  `json:"totalPriorityClasses"`
	HasDefault     bool `json:"hasGlobalDefault"`
	SystemCritical int  `json:"systemCriticalClasses"`
	UserClasses    int  `json:"userDefinedClasses"`
}

type PCClassEntry1992 struct {
	Name            string `json:"name"`
	Value           int32  `json:"value"`
	IsGlobalDefault bool   `json:"isGlobalDefault"`
	IsSystem        bool   `json:"isSystemCritical"`
	Description     string `json:"description"`
}

func (s *Server) handlePriorityClassCatalog(w http.ResponseWriter, r *http.Request) {
	result := PCClassResult1992{ScannedAt: time.Now()}
	score := 100

	pcList, _ := s.clientset.SchedulingV1().PriorityClasses().List(r.Context(), metav1.ListOptions{})

	for _, pc := range pcList.Items {
		result.Summary.TotalClasses++

		entry := PCClassEntry1992{
			Name: pc.Name, Value: pc.Value,
			IsGlobalDefault: pc.GlobalDefault,
			Description:     pc.Description,
		}

		if pc.GlobalDefault {
			result.Summary.HasDefault = true
		}
		if pc.Value >= 1000000 {
			entry.IsSystem = true
			result.Summary.SystemCritical++
		} else {
			result.Summary.UserClasses++
		}

		result.Classes = append(result.Classes, entry)
	}

	sort.Slice(result.Classes, func(i, j int) bool {
		return result.Classes[i].Value > result.Classes[j].Value
	})

	if !result.Summary.HasDefault && result.Summary.TotalClasses > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d priority classes (%d system, %d user), default: %v", result.Summary.TotalClasses, result.Summary.SystemCritical, result.Summary.UserClasses, result.Summary.HasDefault))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Role Binding Catalog
// ---------------------------------------------------------------

type RBListResult1992 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         RBListSummary1992 `json:"summary"`
	Bindings        []RBListEntry1992 `json:"bindings"`
	Recommendations []string          `json:"recommendations"`
}

type RBListSummary1992 struct {
	TotalRoleBindings    int `json:"totalRoleBindings"`
	TotalClusterBindings int `json:"totalClusterRoleBindings"`
	ToUser               int `json:"bindingsToUser"`
	ToSA                 int `json:"bindingsToServiceAccount"`
	ToGroup              int `json:"bindingsToGroup"`
}

type RBListEntry1992 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RoleRef   string `json:"roleRef"`
	Scope     string `json:"scope"`
	Subjects  int    `json:"subjectCount"`
}

func (s *Server) handleRoleBindingCatalog(w http.ResponseWriter, r *http.Request) {
	result := RBListResult1992{ScannedAt: time.Now()}
	score := 100

	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	for _, rb := range rbList.Items {
		result.Summary.TotalRoleBindings++

		entry := RBListEntry1992{
			Name: rb.Name, Namespace: rb.Namespace,
			RoleRef: rb.RoleRef.Kind + "/" + rb.RoleRef.Name,
			Scope:   "namespace", Subjects: len(rb.Subjects),
		}

		for _, sub := range rb.Subjects {
			switch sub.Kind {
			case "User":
				result.Summary.ToUser++
			case "ServiceAccount":
				result.Summary.ToSA++
			case "Group":
				result.Summary.ToGroup++
			}
		}

		result.Bindings = append(result.Bindings, entry)
	}

	for _, crb := range crbList.Items {
		result.Summary.TotalClusterBindings++

		entry := RBListEntry1992{
			Name: crb.Name, Namespace: "cluster",
			RoleRef: crb.RoleRef.Kind + "/" + crb.RoleRef.Name,
			Scope:   "cluster", Subjects: len(crb.Subjects),
		}

		for _, sub := range crb.Subjects {
			switch sub.Kind {
			case "User":
				result.Summary.ToUser++
			case "ServiceAccount":
				result.Summary.ToSA++
			case "Group":
				result.Summary.ToGroup++
			}
		}

		result.Bindings = append(result.Bindings, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d role bindings (%d cluster): %d to SA, %d to User, %d to Group", result.Summary.TotalRoleBindings+result.Summary.TotalClusterBindings, result.Summary.TotalClusterBindings, result.Summary.ToSA, result.Summary.ToUser, result.Summary.ToGroup))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Endpoint Slice Catalog
// ---------------------------------------------------------------

type EPSliceResult1992 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EPSliceSummary1992 `json:"summary"`
	Slices          []EPSliceEntry1992 `json:"slices"`
	Recommendations []string           `json:"recommendations"`
}

type EPSliceSummary1992 struct {
	TotalSlices    int `json:"totalEndpointSlices"`
	TotalEndpoints int `json:"totalEndpoints"`
	ReadyEndpoints int `json:"readyEndpoints"`
	NotReady       int `json:"notReadyEndpoints"`
}

type EPSliceEntry1992 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Service   string `json:"serviceRef"`
	Addresses int    `json:"addressCount"`
	Ports     int    `json:"portCount"`
}

func (s *Server) handleEndpointSliceCatalog(w http.ResponseWriter, r *http.Request) {
	result := EPSliceResult1992{ScannedAt: time.Now()}
	score := 100

	epList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})

	for _, ep := range epList.Items {
		result.Summary.TotalSlices++

		svcName := ""
		if ep.Labels != nil {
			svcName = ep.Labels["kubernetes.io/service-name"]
		}

		entry := EPSliceEntry1992{
			Name: ep.Name, Namespace: ep.Namespace,
			Service: svcName, Ports: len(ep.Ports),
		}

		totalAddrs := 0
		for _, endpoint := range ep.Endpoints {
			totalAddrs += len(endpoint.Addresses)
			result.Summary.TotalEndpoints++
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				result.Summary.ReadyEndpoints++
			} else {
				result.Summary.NotReady++
				score -= 1
			}
		}
		entry.Addresses = totalAddrs

		result.Slices = append(result.Slices, entry)
	}

	_ = discoveryv1.EndpointSlice{} // suppress unused import

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d endpoint slices, %d endpoints (%d ready, %d not ready)", result.Summary.TotalSlices, result.Summary.TotalEndpoints, result.Summary.ReadyEndpoints, result.Summary.NotReady))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
