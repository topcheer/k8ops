package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadFingerprintResult creates a unique fingerprint for each workload
// based on its configuration hash, resource profile, and behavioral patterns.
// This enables drift detection, anomaly identification, and workload classification.
type WorkloadFingerprintResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         FingerprintSummary `json:"summary"`
	Fingerprints    []WorkloadFP       `json:"fingerprints"`
	Duplicates      []FingerprintDup   `json:"duplicates"`
	ByProfile       []ProfileStat      `json:"byProfile"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type FingerprintSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	UniqueProfiles int `json:"uniqueProfiles"`
	Duplicates     int `json:"duplicateWorkloads"`
	IdleWorkloads  int `json:"idleWorkloads"`
}

type WorkloadFP struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Replicas   int    `json:"replicas"`
	ConfigHash string `json:"configHash"`
	ImageHash  string `json:"imageHash"`
	Profile    string `json:"profile"` // web, batch, database, cache, gateway, custom
	CPUProfile string `json:"cpuProfile"`
	MemProfile string `json:"memProfile"`
}

type FingerprintDup struct {
	Profile    string   `json:"profile"`
	ConfigHash string   `json:"configHash"`
	Workloads  []string `json:"workloads"`
	Count      int      `json:"count"`
}

type ProfileStat struct {
	Profile string `json:"profile"`
	Count   int    `json:"count"`
	AvgCPU  string `json:"avgCPU"`
	AvgMem  string `json:"avgMem"`
}

// handleWorkloadFingerprint handles GET /api/product/workload-fingerprint
func (s *Server) handleWorkloadFingerprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := WorkloadFingerprintResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	hashMap := make(map[string][]string) // configHash -> []workload names
	var fps []WorkloadFP
	profileMap := make(map[string][]WorkloadFP)

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		var reqCPU, reqMem float64
		imgList := ""
		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests["cpu"]; ok {
				reqCPU += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests["memory"]; ok {
				reqMem += v.AsApproximateFloat64() / 1e9
			}
			imgList += c.Image + ","
		}

		configHash := shortHash(d.Namespace + "/" + d.Name + "|" + fmt.Sprintf("%.2f", reqCPU) + "|" + fmt.Sprintf("%.2f", reqMem))
		imageHash := shortHash(imgList)
		profile := classifyProfile(d.Name, reqCPU, reqMem)

		fp := WorkloadFP{
			Name: d.Name, Namespace: d.Namespace,
			Replicas:   int(ptrInt32(d.Spec.Replicas)),
			ConfigHash: configHash, ImageHash: imageHash,
			Profile:    profile,
			CPUProfile: fmt.Sprintf("%.2f", reqCPU),
			MemProfile: fmt.Sprintf("%.1fGB", reqMem),
		}

		if fp.Replicas == 0 {
			result.Summary.IdleWorkloads++
		}

		fps = append(fps, fp)
		hashMap[configHash] = append(hashMap[configHash], d.Namespace+"/"+d.Name)
		profileMap[profile] = append(profileMap[profile], fp)
	}

	// Find duplicates (same config hash = same resource profile + name)
	var dups []FingerprintDup
	for hash, workloads := range hashMap {
		if len(workloads) > 1 {
			result.Summary.Duplicates++
			prof := "unknown"
			for _, fp := range fps {
				if fp.ConfigHash == hash {
					prof = fp.Profile
					break
				}
			}
			dups = append(dups, FingerprintDup{
				Profile: prof, ConfigHash: hash,
				Workloads: workloads, Count: len(workloads),
			})
		}
	}
	sort.Slice(dups, func(i, j int) bool {
		return dups[i].Count > dups[j].Count
	})
	result.Duplicates = dups

	// Profile stats
	for prof, fpList := range profileMap {
		totalCPU := 0.0
		totalMem := 0.0
		for _, fp := range fpList {
			fmt.Sscanf(fp.CPUProfile, "%f", &totalCPU)
			fmt.Sscanf(fp.MemProfile, "%fGB", &totalMem)
		}
		avgCPU := totalCPU / float64(len(fpList))
		avgMem := totalMem / float64(len(fpList))
		result.ByProfile = append(result.ByProfile, ProfileStat{
			Profile: prof, Count: len(fpList),
			AvgCPU: fmt.Sprintf("%.2f", avgCPU),
			AvgMem: fmt.Sprintf("%.1fGB", avgMem),
		})
	}
	sort.Slice(result.ByProfile, func(i, j int) bool {
		return result.ByProfile[i].Count > result.ByProfile[j].Count
	})
	result.Summary.UniqueProfiles = len(profileMap)

	result.Fingerprints = fps

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = (result.Summary.TotalWorkloads - result.Summary.Duplicates - result.Summary.IdleWorkloads) * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildFingerprintRecs(&result)
	writeJSON(w, result)
}

func classifyProfile(name string, cpu, mem float64) string {
	nameLower := name
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			nameLower += string(c + 32)
		} else {
			nameLower += string(c)
		}
	}
	// Strip first char from nameLower (it's doubled)
	if len(nameLower) > len(name) {
		nameLower = nameLower[len(name):]
	}

	if containsLowerSimple(nameLower, "redis") || containsLowerSimple(nameLower, "cache") || containsLowerSimple(nameLower, "memcache") {
		return "cache"
	}
	if containsLowerSimple(nameLower, "postgres") || containsLowerSimple(nameLower, "mysql") || containsLowerSimple(nameLower, "mongo") {
		return "database"
	}
	if containsLowerSimple(nameLower, "gateway") || containsLowerSimple(nameLower, "proxy") || containsLowerSimple(nameLower, "nginx") || containsLowerSimple(nameLower, "traefik") {
		return "gateway"
	}
	if containsLowerSimple(nameLower, "job") || containsLowerSimple(nameLower, "batch") || containsLowerSimple(nameLower, "worker") {
		return "batch"
	}
	if cpu > 2 || mem > 4 {
		return "compute"
	}
	return "web"
}

func containsLowerSimple(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func buildFingerprintRecs(r *WorkloadFingerprintResult) []string {
	recs := []string{}
	if r.Summary.Duplicates > 0 {
		recs = append(recs, fmt.Sprintf("%d 组配置相同的工作负载，可能可以合并", r.Summary.Duplicates))
	}
	if r.Summary.IdleWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d 个零副本工作负载可能已废弃", r.Summary.IdleWorkloads))
	}
	if len(r.ByProfile) > 0 {
		top := r.ByProfile[0]
		recs = append(recs, fmt.Sprintf("主要工作负载类型: %s (%d 个)", top.Profile, top.Count))
	}
	if len(recs) == 0 {
		recs = append(recs, "工作负载指纹记录完成")
	}
	return recs
}
