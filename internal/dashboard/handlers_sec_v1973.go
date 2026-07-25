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
// v19.73 — Security Dimension (Round 15)
// 1. Container UID/GID Audit — user/group ID compliance
// 2. Default SA Binding Audit — pods using default ServiceAccount
// 3. Pod Security Posture V2 — composite security scorecard
// ============================================================

// ---------------------------------------------------------------
// 1. Container UID/GID Audit
// ---------------------------------------------------------------

type UIDGIDResult1973 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         UIDGIDSummary1973 `json:"summary"`
	Issues          []UIDGIDEntry1973 `json:"issues"`
	Recommendations []string          `json:"recommendations"`
}

type UIDGIDSummary1973 struct {
	TotalContainers int `json:"totalContainers"`
	WithUID         int `json:"withExplicitUID"`
	RootUID         int `json:"runningAsUID0"`
	WithGID         int `json:"withExplicitGID"`
	RootGID         int `json:"runningAsGID0"`
	WithFSGroup     int `json:"withFSGroup"`
}

type UIDGIDEntry1973 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	UID       int64  `json:"uid"`
	GID       int64  `json:"gid"`
	Issue     string `json:"issue"`
}

func (s *Server) handleContainerUIDGID(w http.ResponseWriter, r *http.Request) {
	result := UIDGIDResult1973{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Pod-level defaults
		podUID := int64(-1)
		podGID := int64(-1)
		podFSGroup := int64(-1)
		if pod.Spec.SecurityContext != nil {
			if pod.Spec.SecurityContext.RunAsUser != nil {
				podUID = *pod.Spec.SecurityContext.RunAsUser
			}
			if pod.Spec.SecurityContext.RunAsGroup != nil {
				podGID = *pod.Spec.SecurityContext.RunAsGroup
			}
			if pod.Spec.SecurityContext.FSGroup != nil {
				podFSGroup = *pod.Spec.SecurityContext.FSGroup
			}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			uid := podUID
			gid := podGID
			if c.SecurityContext != nil {
				if c.SecurityContext.RunAsUser != nil {
					uid = *c.SecurityContext.RunAsUser
					result.Summary.WithUID++
				}
				if c.SecurityContext.RunAsGroup != nil {
					gid = *c.SecurityContext.RunAsGroup
					result.Summary.WithGID++
				}
			}

			if uid == 0 {
				result.Summary.RootUID++
				result.Issues = append(result.Issues, UIDGIDEntry1973{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					UID: 0, Issue: "Running as UID 0 (root)",
				})
				score -= 3
			}
			if gid == 0 {
				result.Summary.RootGID++
			}
			if podFSGroup > 0 {
				result.Summary.WithFSGroup++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d root UID, %d root GID, %d with fsGroup", result.Summary.TotalContainers, result.Summary.RootUID, result.Summary.RootGID, result.Summary.WithFSGroup))
	if result.Summary.RootUID > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers as UID 0 — specify non-root user", result.Summary.RootUID))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Default SA Binding Audit
// ---------------------------------------------------------------

type DefaultSAResult1973 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DefaultSASummary1973 `json:"summary"`
	Violations      []DefaultSAEntry1973 `json:"violations"`
	Recommendations []string             `json:"recommendations"`
}

type DefaultSASummary1973 struct {
	TotalPods      int `json:"totalPods"`
	UsingDefaultSA int `json:"usingDefaultSA"`
	UsingCustomSA  int `json:"usingCustomSA"`
	NoSA           int `json:"withoutSA"`
	WithAutomount  int `json:"withAutomountToken"`
}

type DefaultSAEntry1973 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	SAName    string `json:"serviceAccount"`
	Automount bool   `json:"automountToken"`
}

func (s *Server) handleDefaultSABinding(w http.ResponseWriter, r *http.Request) {
	result := DefaultSAResult1973{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		saName := pod.Spec.ServiceAccountName
		if saName == "" || saName == "default" {
			result.Summary.UsingDefaultSA++

			// Check automount
			automount := true
			if pod.Spec.AutomountServiceAccountToken != nil {
				automount = *pod.Spec.AutomountServiceAccountToken
			}
			if automount {
				result.Summary.WithAutomount++
			}

			result.Violations = append(result.Violations, DefaultSAEntry1973{
				Pod: pod.Name, Namespace: pod.Namespace,
				SAName: saName, Automount: automount,
			})
			if automount {
				score -= 2
			}
		} else {
			result.Summary.UsingCustomSA++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d using default SA, %d custom, %d with automount token", result.Summary.TotalPods, result.Summary.UsingDefaultSA, result.Summary.UsingCustomSA, result.Summary.WithAutomount))
	if result.Summary.UsingDefaultSA > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods using default ServiceAccount — create dedicated SA", result.Summary.UsingDefaultSA))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Security Posture V2
// ---------------------------------------------------------------

type SecPostureV2Result1973 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         SecPostureV2Summary1973 `json:"summary"`
	Checks          []SecPostureCheck1973   `json:"checks"`
	WorstNS         []SecPostureNSEntry1973 `json:"worstNamespaces"`
	Recommendations []string                `json:"recommendations"`
}

type SecPostureV2Summary1973 struct {
	TotalPods      int     `json:"totalPods"`
	PostureScore   float64 `json:"postureScore"`
	PrivilegedPods int     `json:"privilegedPods"`
	HostNetPods    int     `json:"hostNetworkPods"`
	RootPods       int     `json:"rootPods"`
	WritableFS     int     `json:"writableRootFS"`
}

type SecPostureCheck1973 struct {
	Check  string  `json:"check"`
	Passed int     `json:"passed"`
	Failed int     `json:"failed"`
	Score  float64 `json:"score"`
}

type SecPostureNSEntry1973 struct {
	Namespace string  `json:"namespace"`
	PodCount  int     `json:"podCount"`
	Issues    int     `json:"issues"`
	Score     float64 `json:"score"`
}

func (s *Server) handlePodSecurityPostureV2(w http.ResponseWriter, r *http.Request) {
	result := SecPostureV2Result1973{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	checks := map[string]*SecPostureCheck1973{
		"non-privileged":  {Check: "non-privileged"},
		"non-root":        {Check: "non-root"},
		"readonly-rootfs": {Check: "readonly-rootfs"},
		"no-host-network": {Check: "no-host-network"},
		"no-host-pid":     {Check: "no-host-pid"},
		"non-default-sa":  {Check: "non-default-sa"},
	}
	nsStats := make(map[string]*SecPostureNSEntry1973)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &SecPostureNSEntry1973{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.PodCount++

		// Check privileged
		isPriv := false
		isRoot := true // assume root unless proven otherwise
		readonlyFS := false

		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					isPriv = true
					result.Summary.PrivilegedPods++
				}
				if c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
					isRoot = false
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser != 0 {
					isRoot = false
				}
				if c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
					readonlyFS = true
				}
			}
		}

		// Pod-level checks
		if pod.Spec.SecurityContext != nil {
			if pod.Spec.SecurityContext.RunAsNonRoot != nil && *pod.Spec.SecurityContext.RunAsNonRoot {
				isRoot = false
			}
			if pod.Spec.SecurityContext.RunAsUser != nil && *pod.Spec.SecurityContext.RunAsUser != 0 {
				isRoot = false
			}
		}

		hostNet := pod.Spec.HostNetwork
		hostPID := pod.Spec.HostPID

		// Update checks
		if isPriv {
			checks["non-privileged"].Failed++
			ns.Issues++
		} else {
			checks["non-privileged"].Passed++
		}
		if isRoot {
			checks["non-root"].Failed++
			result.Summary.RootPods++
			ns.Issues++
		} else {
			checks["non-root"].Passed++
		}
		if !readonlyFS {
			checks["readonly-rootfs"].Failed++
			result.Summary.WritableFS++
		} else {
			checks["readonly-rootfs"].Passed++
		}
		if hostNet {
			checks["no-host-network"].Failed++
			result.Summary.HostNetPods++
			ns.Issues++
		} else {
			checks["no-host-network"].Passed++
		}
		if hostPID {
			checks["no-host-pid"].Failed++
			ns.Issues++
		} else {
			checks["no-host-pid"].Passed++
		}
		if pod.Spec.ServiceAccountName == "" || pod.Spec.ServiceAccountName == "default" {
			checks["non-default-sa"].Failed++
		} else {
			checks["non-default-sa"].Passed++
		}
	}

	// Calculate check scores
	var totalCheckScore float64
	checkCount := 0
	for _, chk := range checks {
		total := chk.Passed + chk.Failed
		if total > 0 {
			chk.Score = float64(chk.Passed) / float64(total) * 100
		} else {
			chk.Score = 100
		}
		totalCheckScore += chk.Score
		checkCount++
		result.Checks = append(result.Checks, *chk)
	}
	if checkCount > 0 {
		result.Summary.PostureScore = totalCheckScore / float64(checkCount)
	}

	sort.Slice(result.Checks, func(i, j int) bool {
		return result.Checks[i].Score < result.Checks[j].Score
	})

	// Worst namespaces
	for _, ns := range nsStats {
		if ns.PodCount > 0 {
			ns.Score = (1 - float64(ns.Issues)/float64(ns.PodCount)) * 100
		} else {
			ns.Score = 100
		}
		result.WorstNS = append(result.WorstNS, *ns)
	}
	sort.Slice(result.WorstNS, func(i, j int) bool {
		return result.WorstNS[i].Score < result.WorstNS[j].Score
	})
	if len(result.WorstNS) > 10 {
		result.WorstNS = result.WorstNS[:10]
	}

	// Score based on posture
	score = int(result.Summary.PostureScore)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Posture score: %.0f/100 across %d pods (%d privileged, %d root, %d hostNet)", result.Summary.PostureScore, result.Summary.TotalPods, result.Summary.PrivilegedPods, result.Summary.RootPods, result.Summary.HostNetPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
