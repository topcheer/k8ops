package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.19 — Deployment Dimension (Round 23)
// 1. Startup Probe Audit — startup probe configuration compliance
// 2. Container Command Hash — command/args reproducibility tracking
// 3. Deployment Strategy Type — rollout strategy distribution
// ============================================================

// ---------------------------------------------------------------
// 1. Startup Probe Audit
// ---------------------------------------------------------------

type StartupProbeResult2019 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         StartupProbeSummary2019 `json:"summary"`
	Without         []StartupProbeEntry2019 `json:"withoutStartupProbe"`
	Recommendations []string                `json:"recommendations"`
}

type StartupProbeSummary2019 struct {
	TotalContainers  int `json:"totalContainers"`
	WithStartupProbe int `json:"withStartupProbe"`
	WithLiveness     int `json:"withLivenessProbe"`
	WithoutAny       int `json:"withoutAnyProbe"`
}

type StartupProbeEntry2019 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleStartupProbeAudit(w http.ResponseWriter, r *http.Request) {
	result := StartupProbeResult2019{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasStartup := c.StartupProbe != nil
			hasLiveness := c.LivenessProbe != nil

			if hasStartup {
				result.Summary.WithStartupProbe++
			} else if hasLiveness {
				result.Summary.WithLiveness++
			} else {
				result.Summary.WithoutAny++
				result.Without = append(result.Without, StartupProbeEntry2019{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with startup, %d with liveness only, %d without", result.Summary.TotalContainers, result.Summary.WithStartupProbe, result.Summary.WithLiveness, result.Summary.WithoutAny))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Command Hash
// ---------------------------------------------------------------

type CmdHashResult2019 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CmdHashSummary2019 `json:"summary"`
	Hashes          []CmdHashEntry2019 `json:"uniqueHashes"`
	Recommendations []string           `json:"recommendations"`
}

type CmdHashSummary2019 struct {
	TotalContainers int `json:"totalContainers"`
	WithCommand     int `json:"withExplicitCommand"`
	WithArgs        int `json:"withExplicitArgs"`
	UniqueHashes    int `json:"uniqueCommandHashes"`
}

type CmdHashEntry2019 struct {
	Hash  string `json:"hash"`
	Image string `json:"image"`
	Count int    `json:"containerCount"`
}

func (s *Server) handleCmdHash(w http.ResponseWriter, r *http.Request) {
	result := CmdHashResult2019{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	hashMap := make(map[string]*CmdHashEntry2019)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if len(c.Command) > 0 {
				result.Summary.WithCommand++
			}
			if len(c.Args) > 0 {
				result.Summary.WithArgs++
			}

			// Compute hash from command+args+image
			input := c.Image
			for _, cmd := range c.Command {
				input += " " + cmd
			}
			for _, arg := range c.Args {
				input += " " + arg
			}
			h := sha256.Sum256([]byte(input))
			hash := hex.EncodeToString(h[:8])

			entry, ok := hashMap[hash]
			if !ok {
				hashMap[hash] = &CmdHashEntry2019{Hash: hash, Image: c.Image, Count: 1}
			} else {
				entry.Count++
			}
		}
	}

	result.Summary.UniqueHashes = len(hashMap)
	for _, entry := range hashMap {
		result.Hashes = append(result.Hashes, *entry)
	}
	sort.Slice(result.Hashes, func(i, j int) bool {
		return result.Hashes[i].Count > result.Hashes[j].Count
	})
	if len(result.Hashes) > 15 {
		result.Hashes = result.Hashes[:15]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with command, %d with args, %d unique hashes", result.Summary.TotalContainers, result.Summary.WithCommand, result.Summary.WithArgs, result.Summary.UniqueHashes))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Deployment Strategy Type
// ---------------------------------------------------------------

type StratTypeResult2019 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         StratTypeSummary2019 `json:"summary"`
	PerType         []StratTypeEntry2019 `json:"perType"`
	Recommendations []string             `json:"recommendations"`
}

type StratTypeSummary2019 struct {
	TotalDeployments int `json:"totalDeployments"`
	RollingUpdate    int `json:"rollingUpdate"`
	Recreate         int `json:"recreate"`
}

type StratTypeEntry2019 struct {
	Strategy string `json:"strategy"`
	Count    int    `json:"count"`
}

func (s *Server) handleStratType(w http.ResponseWriter, r *http.Request) {
	result := StratTypeResult2019{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	typeCounts := make(map[string]int)

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		strategy := string(dep.Spec.Strategy.Type)
		if strategy == "" {
			strategy = "RollingUpdate"
		}

		typeCounts[strategy]++
		if strategy == "RollingUpdate" {
			result.Summary.RollingUpdate++
		} else if strategy == "Recreate" {
			result.Summary.Recreate++
		}
	}

	for t, c := range typeCounts {
		result.PerType = append(result.PerType, StratTypeEntry2019{Strategy: t, Count: c})
	}
	sort.Slice(result.PerType, func(i, j int) bool {
		return result.PerType[i].Count > result.PerType[j].Count
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d RollingUpdate, %d Recreate", result.Summary.TotalDeployments, result.Summary.RollingUpdate, result.Summary.Recreate))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
