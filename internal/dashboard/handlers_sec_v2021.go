package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.21 — Security Dimension (Round 23)
// 1. Container CapEffective Audit — effective capabilities after drop/add
// 2. Secret Type Coverage — secret type distribution & compliance
// 3. Pod ServiceAccount Mapping — SA-to-pod binding audit
// ============================================================

// ---------------------------------------------------------------
// 1. Container CapEffective Audit
// ---------------------------------------------------------------

type CapEffResult2021 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         CapEffSummary2021 `json:"summary"`
	Caps            []CapEffEntry2021 `json:"effectiveCaps"`
	Recommendations []string          `json:"recommendations"`
}

type CapEffSummary2021 struct {
	TotalContainers int      `json:"totalContainers"`
	WithCapDrop     int      `json:"withCapDrop"`
	WithCapAdd      int      `json:"withCapAdd"`
	WithDropAll     int      `json:"withDropAll"`
	HighRiskCaps    []string `json:"highRiskCapsFound"`
}

type CapEffEntry2021 struct {
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Added     []string `json:"addedCaps"`
	Dropped   []string `json:"droppedCaps"`
}

var highRiskCaps2021 = map[string]bool{
	"CAP_SYS_ADMIN": true, "CAP_SYS_PTRACE": true, "CAP_NET_ADMIN": true,
	"CAP_DAC_OVERRIDE": true, "CAP_SETUID": true, "CAP_SETGID": true,
	"CAP_SETPCAP": true, "CAP_SYS_MODULE": true, "CAP_NET_RAW": true,
}

func (s *Server) handleCapEffAudit(w http.ResponseWriter, r *http.Request) {
	result := CapEffResult2021{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	highRiskFound := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasDrop := false
			hasAdd := false
			var dropped, added []string

			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				caps := c.SecurityContext.Capabilities
				if len(caps.Drop) > 0 {
					hasDrop = true
					for _, d := range caps.Drop {
						capName := string(d)
						dropped = append(dropped, capName)
						if capName == "ALL" {
							result.Summary.WithDropAll++
						}
					}
				}
				if len(caps.Add) > 0 {
					hasAdd = true
					for _, a := range caps.Add {
						capName := string(a)
						added = append(added, capName)
						// Check high-risk
						upperCap := "CAP_" + strings.ToUpper(capName)
						if highRiskCaps2021[upperCap] {
							highRiskFound[capName] = true
							score -= 3
						}
					}
				}
			}

			if hasDrop {
				result.Summary.WithCapDrop++
			}
			if hasAdd {
				result.Summary.WithCapAdd++
			}

			if hasDrop || hasAdd {
				result.Caps = append(result.Caps, CapEffEntry2021{
					Pod: pod.Name, Container: c.Name,
					Added: added, Dropped: dropped,
				})
			}
		}
	}

	for capName := range highRiskFound {
		result.Summary.HighRiskCaps = append(result.Summary.HighRiskCaps, capName)
	}
	sort.Strings(result.Summary.HighRiskCaps)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d drop, %d add, %d dropALL, high-risk: %v", result.Summary.TotalContainers, result.Summary.WithCapDrop, result.Summary.WithCapAdd, result.Summary.WithDropAll, result.Summary.HighRiskCaps))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Secret Type Coverage
// ---------------------------------------------------------------

type SecTypeResult2021 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SecTypeSummary2021 `json:"summary"`
	PerType         []SecTypeEntry2021 `json:"perType"`
	Recommendations []string           `json:"recommendations"`
}

type SecTypeSummary2021 struct {
	TotalSecrets   int `json:"totalSecrets"`
	Dockerconfig   int `json:"dockerConfigCount"`
	TLS            int `json:"tlsCount"`
	Opaque         int `json:"opaqueCount"`
	ServiceAccount int `json:"serviceAccountCount"`
}

type SecTypeEntry2021 struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (s *Server) handleSecTypeCov(w http.ResponseWriter, r *http.Request) {
	result := SecTypeResult2021{ScannedAt: time.Now()}
	score := 100

	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	typeMap := make(map[string]int)
	for _, sec := range secretList.Items {
		result.Summary.TotalSecrets++

		stype := string(sec.Type)
		if stype == "" {
			stype = "Opaque"
		}
		typeMap[stype]++

		switch sec.Type {
		case corev1.SecretTypeDockerConfigJson:
			result.Summary.Dockerconfig++
		case corev1.SecretTypeTLS:
			result.Summary.TLS++
		case corev1.SecretTypeServiceAccountToken:
			result.Summary.ServiceAccount++
		case corev1.SecretTypeOpaque:
			result.Summary.Opaque++
		}
	}

	for t, c := range typeMap {
		result.PerType = append(result.PerType, SecTypeEntry2021{Type: t, Count: c})
	}
	sort.Slice(result.PerType, func(i, j int) bool {
		return result.PerType[i].Count > result.PerType[j].Count
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d secrets: %d opaque, %d TLS, %d dockerconfig, %d SA", result.Summary.TotalSecrets, result.Summary.Opaque, result.Summary.TLS, result.Summary.Dockerconfig, result.Summary.ServiceAccount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod ServiceAccount Mapping
// ---------------------------------------------------------------

type SAPodMapResult2021 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SAPodMapSummary2021 `json:"summary"`
	PerSA           []SAPodMapEntry2021 `json:"perServiceAccount"`
	Recommendations []string            `json:"recommendations"`
}

type SAPodMapSummary2021 struct {
	TotalPods    int `json:"totalPods"`
	UsingDefault int `json:"usingDefaultSA"`
	UsingCustom  int `json:"usingCustomSA"`
	UniqueSAs    int `json:"uniqueServiceAccounts"`
}

type SAPodMapEntry2021 struct {
	SAName    string `json:"serviceAccountName"`
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleSAPodMap(w http.ResponseWriter, r *http.Request) {
	result := SAPodMapResult2021{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	saStats := make(map[string]*SAPodMapEntry2021)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}

		if saName == "default" {
			result.Summary.UsingDefault++
		} else {
			result.Summary.UsingCustom++
		}

		key := pod.Namespace + "/" + saName
		entry, ok := saStats[key]
		if !ok {
			entry = &SAPodMapEntry2021{SAName: saName, Namespace: pod.Namespace}
			saStats[key] = entry
		}
		entry.PodCount++
	}

	result.Summary.UniqueSAs = len(saStats)
	for _, e := range saStats {
		result.PerSA = append(result.PerSA, *e)
	}
	sort.Slice(result.PerSA, func(i, j int) bool {
		return result.PerSA[i].PodCount > result.PerSA[j].PodCount
	})
	if len(result.PerSA) > 15 {
		result.PerSA = result.PerSA[:15]
	}

	if result.Summary.UsingDefault > result.Summary.TotalPods/2 {
		score -= 2
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d default SA, %d custom, %d unique SAs", result.Summary.TotalPods, result.Summary.UsingDefault, result.Summary.UsingCustom, result.Summary.UniqueSAs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
