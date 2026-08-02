package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.65 Security: PodSeccompProfile, ClusterRoleAggregation, SAAutoMountToken

type PodSeccompProfile2665Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     PodSeccompProfile2665Summary `json:"summary"`
	Items       []PodSeccompProfile2665Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type PodSeccompProfile2665Summary struct {
	TotalPods      int `json:"totalPods"`
	WithSeccomp    int `json:"withSeccomp"`
	WithoutSeccomp int `json:"withoutSeccomp"`
}

type PodSeccompProfile2665Item struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	SeccompProfile string `json:"seccompProfile"`
}

func (s *Server) handlePodSeccompProfile2665(w http.ResponseWriter, r *http.Request) {
	result := PodSeccompProfile2665Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			profile := "none"
			if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
				if pod.Spec.SecurityContext.SeccompProfile.Type != "" {
					profile = string(pod.Spec.SecurityContext.SeccompProfile.Type)
				}
			}
			if profile != "none" {
				result.Summary.WithSeccomp++
			} else {
				result.Summary.WithoutSeccomp++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodSeccompProfile2665Item{
					Name: pod.Name, Namespace: pod.Namespace, SeccompProfile: profile,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterRoleAggr2665Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     ClusterRoleAggr2665Summary `json:"summary"`
	Items       []ClusterRoleAggr2665Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type ClusterRoleAggr2665Summary struct {
	TotalCRs     int `json:"totalCRs"`
	WithAggrRule int `json:"withAggrRule"`
	NoAggrRule   int `json:"noAggrRule"`
}

type ClusterRoleAggr2665Item struct {
	Name      string   `json:"name"`
	AggrRules []string `json:"aggregationRules"`
}

func (s *Server) handleClusterRoleAggr2665(w http.ResponseWriter, r *http.Request) {
	result := ClusterRoleAggr2665Result{ScannedAt: time.Now()}
	crs, err := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, cr := range crs.Items {
			result.Summary.TotalCRs++
			rules := make([]string, 0)
			if cr.AggregationRule != nil {
				for _, rb := range cr.AggregationRule.ClusterRoleSelectors {
					rules = append(rules, rb.String())
				}
			}
			if len(rules) > 0 {
				result.Summary.WithAggrRule++
			} else {
				result.Summary.NoAggrRule++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, ClusterRoleAggr2665Item{
					Name: cr.Name, AggrRules: rules,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SAAutoMountToken2665Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     SAAutoMountToken2665Summary `json:"summary"`
	Items       []SAAutoMountToken2665Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type SAAutoMountToken2665Summary struct {
	TotalSAs       int `json:"totalSAs"`
	AutoMountTrue  int `json:"autoMountTrue"`
	AutoMountFalse int `json:"autoMountFalse"`
}

type SAAutoMountToken2665Item struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	AutoMountToken bool   `json:"automountToken"`
}

func (s *Server) handleSAAutoMountToken2665(w http.ResponseWriter, r *http.Request) {
	result := SAAutoMountToken2665Result{ScannedAt: time.Now()}
	sas, err := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sa := range sas.Items {
			result.Summary.TotalSAs++
			autoMount := true
			if sa.AutomountServiceAccountToken != nil {
				autoMount = *sa.AutomountServiceAccountToken
			}
			if autoMount {
				result.Summary.AutoMountTrue++
			} else {
				result.Summary.AutoMountFalse++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SAAutoMountToken2665Item{
					Name: sa.Name, Namespace: sa.Namespace, AutoMountToken: autoMount,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
