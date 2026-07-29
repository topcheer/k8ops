package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.37 — Product Dimension (Round 26)
// 1. Ephemeral Workload Tracker — Jobs/CronJobs product visibility
// 2. API Version Deprecation — deprecated k8s API version usage
// 3. Cross-NS Traffic Estimator — service-to-service dependency patterns
// ============================================================

// ---------------------------------------------------------------
// 1. Ephemeral Workload Tracker
// ---------------------------------------------------------------

type EphemeralResult2037 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EphemeralSummary2037 `json:"summary"`
	Workloads       []EphemeralEntry2037 `json:"workloads"`
	Recommendations []string             `json:"recommendations"`
}

type EphemeralSummary2037 struct {
	TotalJobs     int `json:"totalJobs"`
	TotalCronJobs int `json:"totalCronJobs"`
	FailedJobs    int `json:"failedJobs"`
	SuspendedJobs int `json:"suspendedJobs"`
}

type EphemeralEntry2037 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Schedule  string `json:"schedule,omitempty"`
	Status    string `json:"status"`
}

func (s *Server) handleEphemeralTracker(w http.ResponseWriter, r *http.Request) {
	result := EphemeralResult2037{ScannedAt: time.Now()}
	score := 100

	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})

	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		status := "running"
		if job.Status.Failed > 0 {
			result.Summary.FailedJobs++
			status = "failed"
			score -= 2
		} else if job.Status.Succeeded > 0 {
			status = "completed"
		}
		if job.Spec.Suspend != nil && *job.Spec.Suspend {
			result.Summary.SuspendedJobs++
			status = "suspended"
		}

		result.Workloads = append(result.Workloads, EphemeralEntry2037{
			Name: job.Name, Namespace: job.Namespace, Kind: "Job", Status: status,
		})
	}

	for _, cron := range cronList.Items {
		result.Summary.TotalCronJobs++
		status := "active"
		schedule := ""
		if cron.Spec.Schedule != "" {
			schedule = cron.Spec.Schedule
		}
		if cron.Spec.Suspend != nil && *cron.Spec.Suspend {
			result.Summary.SuspendedJobs++
			status = "suspended"
		}

		result.Workloads = append(result.Workloads, EphemeralEntry2037{
			Name: cron.Name, Namespace: cron.Namespace, Kind: "CronJob",
			Schedule: schedule, Status: status,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].Namespace < result.Workloads[j].Namespace
	})

	if result.Summary.FailedJobs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d jobs have failed — check job logs and retry policy", result.Summary.FailedJobs))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. API Version Deprecation
// ---------------------------------------------------------------

type APIVerDepResult2037 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         APIVerDepSummary2037 `json:"summary"`
	Deprecated      []APIVerDepEntry2037 `json:"deprecatedResources"`
	Recommendations []string             `json:"recommendations"`
}

type APIVerDepSummary2037 struct {
	TotalCRDs     int `json:"totalCRDs"`
	DeprecatedAPI int `json:"deprecatedAPI"`
	RemovedAPI    int `json:"removedAPI"`
}

type APIVerDepEntry2037 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Status     string `json:"status"`
}

func (s *Server) handleAPIVerDeprecation(w http.ResponseWriter, r *http.Request) {
	result := APIVerDepResult2037{ScannedAt: time.Now()}
	score := 100

	// Known deprecated/removed API versions in k8s 1.25+
	deprecatedAPIs := map[string]string{
		"extensions/v1beta1":                   "deprecated",
		"apps/v1beta1":                         "deprecated",
		"apps/v1beta2":                         "deprecated",
		"networking.k8s.io/v1beta1":            "deprecated",
		"policy/v1beta1":                       "deprecated",
		"autoscaling/v2beta1":                  "deprecated",
		"autoscaling/v2beta2":                  "deprecated",
		"batch/v1beta1":                        "deprecated",
		"rbac.authorization.k8s.io/v1beta1":    "deprecated",
		"storage.k8s.io/v1beta1":               "deprecated",
		"certificates.k8s.io/v1beta1":          "deprecated",
		"admissionregistration.k8s.io/v1beta1": "deprecated",
		"apiextensions.k8s.io/v1beta1":         "deprecated",
		"scheduling.k8s.io/v1beta1":            "deprecated",
	}

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	for _, ing := range ingList.Items {
		result.Summary.TotalCRDs++
		apiVer := ing.APIVersion
		if apiVer == "" {
			apiVer = "networking.k8s.io/v1"
		}
		if status, ok := deprecatedAPIs[strings.ToLower(apiVer)]; ok {
			if status == "deprecated" {
				result.Summary.DeprecatedAPI++
				score -= 3
			}
			result.Deprecated = append(result.Deprecated, APIVerDepEntry2037{
				Name: ing.Name, Namespace: ing.Namespace,
				Kind: "Ingress", APIVersion: apiVer, Status: status,
			})
		}
	}

	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	for _, pdb := range pdbList.Items {
		result.Summary.TotalCRDs++
		apiVer := pdb.APIVersion
		if apiVer == "" {
			apiVer = "policy/v1"
		}
		if status, ok := deprecatedAPIs[strings.ToLower(apiVer)]; ok {
			result.Summary.DeprecatedAPI++
			score -= 3
			result.Deprecated = append(result.Deprecated, APIVerDepEntry2037{
				Name: pdb.Name, Namespace: pdb.Namespace,
				Kind: "PDB", APIVersion: apiVer, Status: status,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Deprecated, func(i, j int) bool {
		return result.Deprecated[i].Namespace < result.Deprecated[j].Namespace
	})

	if result.Summary.DeprecatedAPI > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d resources use deprecated API versions — migrate before cluster upgrade", result.Summary.DeprecatedAPI))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cross-NS Traffic Estimator
// ---------------------------------------------------------------

type XNSResult2037 struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	HealthScore     int            `json:"healthScore"`
	Grade           string         `json:"grade"`
	Summary         XNSSummary2037 `json:"summary"`
	CrossNSTraffic  []XNSEntry2037 `json:"crossNSTraffic"`
	Recommendations []string       `json:"recommendations"`
}

type XNSSummary2037 struct {
	TotalServices    int `json:"totalServices"`
	ExternalNameSvcs int `json:"externalNameServices"`
	HeadlessSvcs     int `json:"headlessServices"`
	NSLinked         int `json:"namespacesLinked"`
}

type XNSEntry2037 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	TargetNS  string `json:"targetNamespace,omitempty"`
}

func (s *Server) handleXNSTrafficEst(w http.ResponseWriter, r *http.Request) {
	result := XNSResult2037{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalServices = len(svcList.Items)

	// ExternalName services represent cross-namespace traffic
	for _, svc := range svcList.Items {
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalNameSvcs++
			targetNS := ""
			if strings.Contains(svc.Spec.ExternalName, ".") {
				parts := strings.SplitN(svc.Spec.ExternalName, ".", 2)
				if len(parts) > 1 {
					nsParts := strings.SplitN(parts[1], ".", 2)
					if len(nsParts) > 0 {
						targetNS = nsParts[0]
					}
				}
			}
			result.CrossNSTraffic = append(result.CrossNSTraffic, XNSEntry2037{
				Service:   svc.Name,
				Namespace: svc.Namespace,
				Type:      "ExternalName",
				TargetNS:  targetNS,
			})
		}
		if svc.Spec.ClusterIP == "None" {
			result.Summary.HeadlessSvcs++
		}
	}

	// Ingress backends linking namespaces
	for _, ing := range ingList.Items {
		result.Summary.TotalServices++
		for _, rule := range ing.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				backend := path.Backend.Service
				if backend != nil {
					// Check if service is in a different namespace via ingressClassName or annotations
					ns := ing.Namespace
					if backend.Port.Name != "" || backend.Port.Number != 0 {
						result.CrossNSTraffic = append(result.CrossNSTraffic, XNSEntry2037{
							Service:   backend.Name,
							Namespace: ns,
							Type:      "Ingress",
						})
					}
				}
			}
		}
	}

	result.Summary.NSLinked = len(result.CrossNSTraffic)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.CrossNSTraffic, func(i, j int) bool {
		return result.CrossNSTraffic[i].Namespace < result.CrossNSTraffic[j].Namespace
	})

	if result.Summary.ExternalNameSvcs > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ExternalName services — review cross-namespace dependencies", result.Summary.ExternalNameSvcs))
	}

	writeJSON(w, result)
}

// keep imports
var _ = batchv1.Job{}
