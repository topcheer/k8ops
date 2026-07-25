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
// v19.80 — Documentation Dimension (Round 16)
// 1. Secret Inventory — all secrets with type & age classification
// 2. Service Account Inventory — SA catalog with binding status
// 3. Event Type Catalog — event reason distribution & source mapping
// ============================================================

// ---------------------------------------------------------------
// 1. Secret Inventory
// ---------------------------------------------------------------

type SecretInvResult1980 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SecretInvSummary1980 `json:"summary"`
	Secrets         []SecretInvEntry1980 `json:"secrets"`
	OldSecrets      []SecretInvEntry1980 `json:"oldSecrets"`
	Recommendations []string             `json:"recommendations"`
}

type SecretInvSummary1980 struct {
	TotalSecrets  int     `json:"totalSecrets"`
	DockerConfig  int     `json:"dockerConfigSecrets"`
	TLSCerts      int     `json:"tlsCertSecrets"`
	OpaqueSecrets int     `json:"opaqueSecrets"`
	AvgAgeDays    float64 `json:"avgAgeDays"`
	OldSecrets    int     `json:"oldSecrets90d"`
}

type SecretInvEntry1980 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Type      string  `json:"type"`
	KeyCount  int     `json:"keyCount"`
	AgeDays   float64 `json:"ageDays"`
}

func (s *Server) handleSecretInventory(w http.ResponseWriter, r *http.Request) {
	result := SecretInvResult1980{ScannedAt: time.Now()}
	score := 100

	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	var totalAge float64
	for _, sec := range secretList.Items {
		result.Summary.TotalSecrets++

		keyCount := len(sec.Data)
		ageDays := 0.0
		if !sec.CreationTimestamp.IsZero() {
			ageDays = time.Since(sec.CreationTimestamp.Time).Hours() / 24
			totalAge += ageDays
		}

		entry := SecretInvEntry1980{
			Name: sec.Name, Namespace: sec.Namespace,
			Type: string(sec.Type), KeyCount: keyCount, AgeDays: ageDays,
		}

		switch sec.Type {
		case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
			result.Summary.DockerConfig++
		case corev1.SecretTypeTLS:
			result.Summary.TLSCerts++
		case corev1.SecretTypeOpaque:
			result.Summary.OpaqueSecrets++
		}

		if ageDays > 90 {
			result.Summary.OldSecrets++
			result.OldSecrets = append(result.OldSecrets, entry)
		}

		result.Secrets = append(result.Secrets, entry)
	}

	if result.Summary.TotalSecrets > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(result.Summary.TotalSecrets)
	}

	sort.Slice(result.Secrets, func(i, j int) bool {
		return result.Secrets[i].AgeDays > result.Secrets[j].AgeDays
	})
	if len(result.Secrets) > 30 {
		result.Secrets = result.Secrets[:30]
	}

	if result.Summary.OldSecrets > 10 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d secrets: %d opaque, %d TLS, %d docker, avg %.0fd old", result.Summary.TotalSecrets, result.Summary.OpaqueSecrets, result.Summary.TLSCerts, result.Summary.DockerConfig, result.Summary.AvgAgeDays))
	if result.Summary.OldSecrets > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d secrets older than 90 days — review for rotation", result.Summary.OldSecrets))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Account Inventory
// ---------------------------------------------------------------

type SAInvResult1980 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         SAInvSummary1980 `json:"summary"`
	SAs             []SAInvEntry1980 `json:"serviceAccounts"`
	UnboundSAs      []SAInvEntry1980 `json:"unboundSAs"`
	Recommendations []string         `json:"recommendations"`
}

type SAInvSummary1980 struct {
	TotalSAs      int `json:"totalServiceAccounts"`
	WithBindings  int `json:"withRoleBindings"`
	Unbound       int `json:"unboundSAs"`
	DefaultSAs    int `json:"defaultSAs"`
	WithImagePull int `json:"withImagePullSecrets"`
}

type SAInvEntry1980 struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	HasSecret    bool   `json:"hasImagePullSecret"`
	AutoMount    bool   `json:"automountToken"`
	BindingCount int    `json:"bindingCount"`
}

func (s *Server) handleSAInventory(w http.ResponseWriter, r *http.Request) {
	result := SAInvResult1980{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	// Count bindings per SA
	saBindings := make(map[string]int)
	for _, rb := range rbList.Items {
		for _, sub := range rb.Subjects {
			if sub.Kind == "ServiceAccount" {
				saBindings[sub.Namespace+"/"+sub.Name]++
			}
		}
	}
	for _, crb := range crbList.Items {
		for _, sub := range crb.Subjects {
			if sub.Kind == "ServiceAccount" {
				saBindings[sub.Namespace+"/"+sub.Name]++
			}
		}
	}

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++

		key := sa.Namespace + "/" + sa.Name
		bindCount := saBindings[key]

		entry := SAInvEntry1980{
			Name: sa.Name, Namespace: sa.Namespace,
			HasSecret:    len(sa.ImagePullSecrets) > 0,
			AutoMount:    sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken,
			BindingCount: bindCount,
		}

		if sa.Name == "default" {
			result.Summary.DefaultSAs++
		}
		if bindCount > 0 {
			result.Summary.WithBindings++
		} else {
			result.Summary.Unbound++
			result.UnboundSAs = append(result.UnboundSAs, entry)
		}
		if entry.HasSecret {
			result.Summary.WithImagePull++
		}

		result.SAs = append(result.SAs, entry)
	}

	if result.Summary.Unbound > 20 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs: %d bound, %d unbound, %d with pull secrets", result.Summary.TotalSAs, result.Summary.WithBindings, result.Summary.Unbound, result.Summary.WithImagePull))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event Type Catalog
// ---------------------------------------------------------------

type EventTypeResult1980 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EventTypeSummary1980 `json:"summary"`
	ByReason        []EventTypeEntry1980 `json:"byReason"`
	BySource        []EventTypeEntry1980 `json:"bySource"`
	Recommendations []string             `json:"recommendations"`
}

type EventTypeSummary1980 struct {
	TotalEvents   int `json:"totalEvents"`
	UniqueReasons int `json:"uniqueReasons"`
	UniqueSources int `json:"uniqueSources"`
	WarningCount  int `json:"warningEvents"`
	NormalCount   int `json:"normalEvents"`
}

type EventTypeEntry1980 struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Server) handleEventTypeCatalog(w http.ResponseWriter, r *http.Request) {
	result := EventTypeResult1980{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	reasonMap := make(map[string]int)
	sourceMap := make(map[string]int)

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		if evt.Type == "Warning" {
			result.Summary.WarningCount++
		} else {
			result.Summary.NormalCount++
		}

		reasonMap[evt.Reason]++
		sourceMap[evt.Source.Component]++
	}

	result.Summary.UniqueReasons = len(reasonMap)
	result.Summary.UniqueSources = len(sourceMap)

	for reason, count := range reasonMap {
		result.ByReason = append(result.ByReason, EventTypeEntry1980{Name: reason, Count: count})
	}
	sort.Slice(result.ByReason, func(i, j int) bool {
		return result.ByReason[i].Count > result.ByReason[j].Count
	})
	if len(result.ByReason) > 20 {
		result.ByReason = result.ByReason[:20]
	}

	for source, count := range sourceMap {
		result.BySource = append(result.BySource, EventTypeEntry1980{Name: source, Count: count})
	}
	sort.Slice(result.BySource, func(i, j int) bool {
		return result.BySource[i].Count > result.BySource[j].Count
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events: %d unique reasons, %d sources (%d warning, %d normal)", result.Summary.TotalEvents, result.Summary.UniqueReasons, result.Summary.UniqueSources, result.Summary.WarningCount, result.Summary.NormalCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
