package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.41 Security: PodPrivilegeEscalation, SAImagePullSecret, PVAccessModes

type PodPrivEscalation2641Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     PodPrivEscalation2641Summary `json:"summary"`
	Items       []PodPrivEscalation2641Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type PodPrivEscalation2641Summary struct {
	TotalPods     int `json:"totalPods"`
	AllowEscalate int `json:"allowEscalate"`
	DenyEscalate  int `json:"denyEscalate"`
}

type PodPrivEscalation2641Item struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	AllowPrivEscalation bool   `json:"allowPrivilegeEscalation"`
}

func (s *Server) handlePodPrivEscalation2641(w http.ResponseWriter, r *http.Request) {
	result := PodPrivEscalation2641Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			for _, c := range pod.Spec.Containers {
				allow := true
				if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil {
					allow = *c.SecurityContext.AllowPrivilegeEscalation
				}
				if allow {
					result.Summary.AllowEscalate++
				} else {
					result.Summary.DenyEscalate++
				}
				if len(result.Items) < 50 {
					result.Items = append(result.Items, PodPrivEscalation2641Item{
						Name: pod.Name, Namespace: pod.Namespace, AllowPrivEscalation: allow,
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

type SAImagePullSecret2641Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     SAImagePullSecret2641Summary `json:"summary"`
	Items       []SAImagePullSecret2641Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type SAImagePullSecret2641Summary struct {
	TotalSAs       int `json:"totalSAs"`
	WithPullSecret int `json:"withPullSecret"`
	WithoutSecret  int `json:"withoutSecret"`
}

type SAImagePullSecret2641Item struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Secrets   []string `json:"secrets"`
}

func (s *Server) handleSAImagePullSecret2641(w http.ResponseWriter, r *http.Request) {
	result := SAImagePullSecret2641Result{ScannedAt: time.Now()}
	sas, err := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sa := range sas.Items {
			result.Summary.TotalSAs++
			if len(sa.ImagePullSecrets) > 0 {
				result.Summary.WithPullSecret++
			} else {
				result.Summary.WithoutSecret++
			}
			if len(result.Items) < 50 {
				names := make([]string, 0, len(sa.ImagePullSecrets))
				for _, ips := range sa.ImagePullSecrets {
					names = append(names, ips.Name)
				}
				result.Items = append(result.Items, SAImagePullSecret2641Item{
					Name: sa.Name, Namespace: sa.Namespace, Secrets: names,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVAccessModes2641Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     PVAccessModes2641Summary `json:"summary"`
	Items       []PVAccessModes2641Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type PVAccessModes2641Summary struct {
	TotalPVs int `json:"totalPVs"`
	RWO      int `json:"rwo"`
	ROX      int `json:"rox"`
	RWX      int `json:"rwx"`
}

type PVAccessModes2641Item struct {
	Name        string   `json:"name"`
	AccessModes []string `json:"accessModes"`
}

func (s *Server) handlePVAccessModes2641(w http.ResponseWriter, r *http.Request) {
	result := PVAccessModes2641Result{ScannedAt: time.Now()}
	pvs, err := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pv := range pvs.Items {
			result.Summary.TotalPVs++
			modes := make([]string, 0, len(pv.Spec.AccessModes))
			for _, am := range pv.Spec.AccessModes {
				modes = append(modes, string(am))
				switch am {
				case corev1.ReadWriteOnce:
					result.Summary.RWO++
				case corev1.ReadOnlyMany:
					result.Summary.ROX++
				case corev1.ReadWriteMany:
					result.Summary.RWX++
				}
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PVAccessModes2641Item{
					Name: pv.Name, AccessModes: modes,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
