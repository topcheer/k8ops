package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.59 Security: PodCapabilitiesAudit, RoleBindingKind, PDBAllowedDisruptions

type PodCapabilities2659Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     PodCapabilities2659Summary `json:"summary"`
	Items       []PodCapabilities2659Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type PodCapabilities2659Summary struct {
	TotalPods   int `json:"totalPods"`
	WithCapAdd  int `json:"withCapAdd"`
	WithCapDrop int `json:"withCapDrop"`
}

type PodCapabilities2659Item struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	CapAdd    []string `json:"capAdd"`
	CapDrop   []string `json:"capDrop"`
}

func (s *Server) handlePodCapabilities2659(w http.ResponseWriter, r *http.Request) {
	result := PodCapabilities2659Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			var capAdd, capDrop []string
			for _, c := range pod.Spec.Containers {
				if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
					for _, cap := range c.SecurityContext.Capabilities.Add {
						capAdd = append(capAdd, string(cap))
					}
					for _, cap := range c.SecurityContext.Capabilities.Drop {
						capDrop = append(capDrop, string(cap))
					}
				}
				break
			}
			if len(capAdd) > 0 {
				result.Summary.WithCapAdd++
			}
			if len(capDrop) > 0 {
				result.Summary.WithCapDrop++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodCapabilities2659Item{
					Name: pod.Name, Namespace: pod.Namespace, CapAdd: capAdd, CapDrop: capDrop,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleBindingKind2659Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     RoleBindingKind2659Summary `json:"summary"`
	Items       []RoleBindingKind2659Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type RoleBindingKind2659Summary struct {
	TotalRBs  int `json:"totalRBs"`
	UserKind  int `json:"userKind"`
	SAKind    int `json:"saKind"`
	GroupKind int `json:"groupKind"`
}

type RoleBindingKind2659Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	SubjectKind string `json:"subjectKind"`
}

func (s *Server) handleRoleBindingKind2659(w http.ResponseWriter, r *http.Request) {
	result := RoleBindingKind2659Result{ScannedAt: time.Now()}
	rbs, err := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, rb := range rbs.Items {
			result.Summary.TotalRBs++
			for _, sub := range rb.Subjects {
				switch sub.Kind {
				case "User":
					result.Summary.UserKind++
				case "ServiceAccount":
					result.Summary.SAKind++
				case "Group":
					result.Summary.GroupKind++
				}
				if len(result.Items) < 50 {
					result.Items = append(result.Items, RoleBindingKind2659Item{
						Name: rb.Name, Namespace: rb.Namespace, SubjectKind: sub.Kind,
					})
				}
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PDBAllowedDisrupt2659Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     PDBAllowedDisrupt2659Summary `json:"summary"`
	Items       []PDBAllowedDisrupt2659Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type PDBAllowedDisrupt2659Summary struct {
	TotalPDBs   int `json:"totalPDBs"`
	ZeroDisrupt int `json:"zeroDisruption"`
	HasDisrupt  int `json:"hasDisruption"`
}

type PDBAllowedDisrupt2659Item struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	AllowedDisruptions int32  `json:"allowedDisruptions"`
}

func (s *Server) handlePDBAllowedDisrupt2659(w http.ResponseWriter, r *http.Request) {
	result := PDBAllowedDisrupt2659Result{ScannedAt: time.Now()}
	pdbs, err := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pdb := range pdbs.Items {
			result.Summary.TotalPDBs++
			allowed := pdb.Status.DisruptionsAllowed
			if allowed == 0 {
				result.Summary.ZeroDisrupt++
			} else {
				result.Summary.HasDisrupt++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PDBAllowedDisrupt2659Item{
					Name: pdb.Name, Namespace: pdb.Namespace, AllowedDisruptions: allowed,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
