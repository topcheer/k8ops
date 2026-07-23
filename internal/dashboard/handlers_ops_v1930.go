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
// v19.30 — Operations Dimension (Round 8)
// 1. PVC Lifecycle Monitor — PVC binding health & reclaim tracking
// 2. Service Endpoint Latency — endpoint readiness latency analysis
// 3. Container State Forensics — container exit code & state analysis
// ============================================================

// ---------------------------------------------------------------
// 1. PVC Lifecycle Monitor — PVC binding health & reclaim tracking
// ---------------------------------------------------------------

type PVCLifecycleResult1930 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PVCLifecycleSummary1930 `json:"summary"`
	PVCs            []PVCLifecycleEntry1930 `json:"pvcs"`
	PendingPVCs     []PVCPendingEntry1930   `json:"pendingPVCs"`
	Reclaimable     []PVCReclaimEntry1930   `json:"reclaimable"`
	Recommendations []string                `json:"recommendations"`
}

type PVCLifecycleSummary1930 struct {
	TotalPVCs     int `json:"totalPVCs"`
	BoundPVCs     int `json:"boundPVCs"`
	PendingPVCs   int `json:"pendingPVCs"`
	ReleasedPVs   int `json:"releasedPVs"`
	OrphanedPVCs  int `json:"orphanedPVCs"`
	TotalPVs      int `json:"totalPVs"`
	ReclaimablePV int `json:"reclaimablePVs"`
}

type PVCLifecycleEntry1930 struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Phase        string `json:"phase"`
	StorageClass string `json:"storageClass"`
	Size         string `json:"size"`
	AccessMode   string `json:"accessMode"`
	Age          string `json:"age"`
	BoundPV      string `json:"boundPV"`
}

type PVCPendingEntry1930 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Size      string `json:"size"`
	Reason    string `json:"reason"`
	Age       string `json:"age"`
}

type PVCReclaimEntry1930 struct {
	PVName   string `json:"pvName"`
	Phase    string `json:"phase"`
	Reason   string `json:"reason"`
	ReclaimP string `json:"reclaimPolicy"`
	Size     string `json:"size"`
}

func (s *Server) handlePVCLifecycle(w http.ResponseWriter, r *http.Request) {
	result := PVCLifecycleResult1930{
		ScannedAt: time.Now(),
	}
	score := 100

	pvcList, err := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	pvList, err := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	result.Summary.TotalPVs = len(pvList.Items)

	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		phase := string(pvc.Status.Phase)
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		size := pvc.Spec.Resources.Requests.Storage().String()
		accessMode := ""
		if len(pvc.Spec.AccessModes) > 0 {
			accessMode = string(pvc.Spec.AccessModes[0])
		}
		age := fmt.Sprintf("%.0fd", time.Since(pvc.CreationTimestamp.Time).Hours()/24)
		boundPV := ""
		if pvc.Spec.VolumeName != "" {
			boundPV = pvc.Spec.VolumeName
		}

		entry := PVCLifecycleEntry1930{
			Name: pvc.Name, Namespace: pvc.Namespace, Phase: phase,
			StorageClass: scName, Size: size, AccessMode: accessMode,
			Age: age, BoundPV: boundPV,
		}
		result.PVCs = append(result.PVCs, entry)

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
			result.PendingPVCs = append(result.PendingPVCs, PVCPendingEntry1930{
				Name: pvc.Name, Namespace: pvc.Namespace, Size: size,
				Reason: "PVC is pending — waiting for PV binding or storage provisioning",
				Age:    age,
			})
			score -= 5
		}
	}

	// Check PVs for reclaimable ones
	for _, pv := range pvList.Items {
		if pv.Status.Phase == corev1.VolumeReleased {
			result.Summary.ReleasedPVs++
			result.Summary.ReclaimablePV++
			result.Reclaimable = append(result.Reclaimable, PVCReclaimEntry1930{
				PVName: pv.Name, Phase: string(pv.Status.Phase),
				Reason:   "PV is Released — can be reclaimed or deleted",
				ReclaimP: string(pv.Spec.PersistentVolumeReclaimPolicy),
				Size:     pv.Spec.Capacity.Storage().String(),
			})
		}
		if pv.Status.Phase == corev1.VolumeAvailable {
			result.Summary.ReclaimablePV++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PendingPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pending PVCs — check storage class and provisioner", result.Summary.PendingPVCs))
	}
	if result.Summary.ReleasedPVs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d released PVs — clean up to free storage", result.Summary.ReleasedPVs))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Endpoint Latency — endpoint readiness latency analysis
// ---------------------------------------------------------------

type EndpointLatencyResult1930 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         EndpointLatencySummary1930 `json:"summary"`
	Services        []EndpointLatencyEntry1930 `json:"services"`
	SlowServices    []EndpointSlowEntry1930    `json:"slowServices"`
	Recommendations []string                   `json:"recommendations"`
}

type EndpointLatencySummary1930 struct {
	TotalServices    int     `json:"totalServices"`
	WithEndpoints    int     `json:"withEndpoints"`
	WithoutEndpoints int     `json:"withoutEndpoints"`
	TotalEndpoints   int     `json:"totalEndpoints"`
	NotReadyCount    int     `json:"notReadyCount"`
	AvgReadyRatio    float64 `json:"avgReadyRatio"`
}

type EndpointLatencyEntry1930 struct {
	Name          string  `json:"name"`
	Namespace     string  `json:"namespace"`
	ReadyAddrs    int     `json:"readyAddresses"`
	NotReadyAddrs int     `json:"notReadyAddresses"`
	ReadyRatio    float64 `json:"readyRatio"`
	Age           string  `json:"age"`
}

type EndpointSlowEntry1930 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

func (s *Server) handleEndpointLatency(w http.ResponseWriter, r *http.Request) {
	result := EndpointLatencyResult1930{
		ScannedAt: time.Now(),
	}
	score := 100

	epList, err := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	var totalRatio float64
	var ratioCount int

	for _, ep := range epList.Items {
		if isSystemNamespace(ep.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		readyCount := 0
		notReadyCount := 0
		for _, subset := range ep.Subsets {
			readyCount += len(subset.Addresses)
			notReadyCount += len(subset.NotReadyAddresses)
		}

		totalAddrs := readyCount + notReadyCount
		readyRatio := 1.0
		if totalAddrs > 0 {
			readyRatio = float64(readyCount) / float64(totalAddrs)
		} else {
			readyRatio = 0
		}

		age := "unknown"
		if !ep.CreationTimestamp.IsZero() {
			age = fmt.Sprintf("%.0fd", time.Since(ep.CreationTimestamp.Time).Hours()/24)
		}

		entry := EndpointLatencyEntry1930{
			Name: ep.Name, Namespace: ep.Namespace,
			ReadyAddrs: readyCount, NotReadyAddrs: notReadyCount,
			ReadyRatio: readyRatio, Age: age,
		}
		result.Services = append(result.Services, entry)

		if totalAddrs > 0 {
			result.Summary.WithEndpoints++
			result.Summary.TotalEndpoints += totalAddrs
			totalRatio += readyRatio
			ratioCount++
		} else {
			result.Summary.WithoutEndpoints++
			result.SlowServices = append(result.SlowServices, EndpointSlowEntry1930{
				Name: ep.Name, Namespace: ep.Namespace,
				Issue:    "Service has no endpoint addresses — no pods serving traffic",
				Severity: "high",
			})
			score -= 5
		}

		if notReadyCount > 0 {
			result.Summary.NotReadyCount++
			result.SlowServices = append(result.SlowServices, EndpointSlowEntry1930{
				Name: ep.Name, Namespace: ep.Namespace,
				Issue:    fmt.Sprintf("%d of %d endpoints not ready — traffic being dropped", notReadyCount, totalAddrs),
				Severity: "medium",
			})
			score -= 2
		}

		if readyRatio < 0.5 && totalAddrs > 1 {
			result.SlowServices = append(result.SlowServices, EndpointSlowEntry1930{
				Name: ep.Name, Namespace: ep.Namespace,
				Issue:    fmt.Sprintf("Only %.0f%% endpoints ready — degraded service", readyRatio*100),
				Severity: "high",
			})
			score -= 3
		}
	}

	if ratioCount > 0 {
		result.Summary.AvgReadyRatio = totalRatio / float64(ratioCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutEndpoints > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services with no endpoints — pods not ready or selector mismatch", result.Summary.WithoutEndpoints))
	}
	if result.Summary.NotReadyCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services with not-ready endpoints — check pod health probes", result.Summary.NotReadyCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container State Forensics — container exit code & state analysis
// ---------------------------------------------------------------

type ContainerForensicsResult1930 struct {
	ScannedAt       time.Time                     `json:"scannedAt"`
	HealthScore     int                           `json:"healthScore"`
	Grade           string                        `json:"grade"`
	Summary         ContainerForensicsSummary1930 `json:"summary"`
	Containers      []ContainerStateEntry1930     `json:"containers"`
	ExitCodes       []ExitCodeStat1930            `json:"exitCodes"`
	Recommendations []string                      `json:"recommendations"`
}

type ContainerForensicsSummary1930 struct {
	TotalContainers int   `json:"totalContainers"`
	RunningCount    int   `json:"runningCount"`
	WaitingCount    int   `json:"waitingCount"`
	TerminatedCount int   `json:"terminatedCount"`
	OOMKilledCount  int   `json:"oomKilledCount"`
	ErrorExitCount  int   `json:"errorExitCount"`
	TotalRestarts   int32 `json:"totalRestarts"`
}

type ContainerStateEntry1930 struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	State        string `json:"state"`
	Reason       string `json:"reason"`
	ExitCode     int32  `json:"exitCode"`
	RestartCount int32  `json:"restartCount"`
	OOMKilled    bool   `json:"oomKilled"`
}

type ExitCodeStat1930 struct {
	ExitCode int32  `json:"exitCode"`
	Count    int    `json:"count"`
	Meaning  string `json:"meaning"`
}

var exitCodeMeanings = map[int32]string{
	0:   "Success",
	1:   "General error",
	2:   "Misuse of shell builtin",
	126: "Command cannot execute",
	127: "Command not found",
	128: "Invalid exit argument",
	137: "Killed (SIGKILL / OOM)",
	139: "Segmentation fault",
	143: "Terminated (SIGTERM)",
}

func (s *Server) handleContainerForensics(w http.ResponseWriter, r *http.Request) {
	result := ContainerForensicsResult1930{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	exitCodeMap := make(map[int32]int)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			result.Summary.TotalRestarts += int32(cs.RestartCount)

			entry := ContainerStateEntry1930{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				Container:    cs.Name,
				RestartCount: cs.RestartCount,
			}

			if cs.State.Running != nil {
				entry.State = "running"
				result.Summary.RunningCount++
			} else if cs.State.Waiting != nil {
				entry.State = "waiting"
				entry.Reason = cs.State.Waiting.Reason
				result.Summary.WaitingCount++
				if cs.State.Waiting.Reason == "CrashLoopBackOff" {
					score -= 5
				}
			} else if cs.State.Terminated != nil {
				entry.State = "terminated"
				entry.ExitCode = cs.State.Terminated.ExitCode
				entry.Reason = cs.State.Terminated.Reason
				result.Summary.TerminatedCount++
				exitCodeMap[cs.State.Terminated.ExitCode]++

				if cs.State.Terminated.Reason == "OOMKilled" {
					entry.OOMKilled = true
					result.Summary.OOMKilledCount++
					score -= 5
				}
				if cs.State.Terminated.ExitCode != 0 {
					result.Summary.ErrorExitCount++
					score -= 2
				}
			}

			// Only add non-running or restarted containers
			if entry.State != "running" || cs.RestartCount > 0 {
				result.Containers = append(result.Containers, entry)
			}
		}
	}

	// Build exit code stats
	for code, count := range exitCodeMap {
		meaning := exitCodeMeanings[code]
		if meaning == "" {
			meaning = "Unknown exit code"
		}
		result.ExitCodes = append(result.ExitCodes, ExitCodeStat1930{
			ExitCode: code, Count: count, Meaning: meaning,
		})
	}
	sort.Slice(result.ExitCodes, func(i, j int) bool {
		return result.ExitCodes[i].Count > result.ExitCodes[j].Count
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OOMKilledCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers OOMKilled — increase memory limits or optimize app", result.Summary.OOMKilledCount))
	}
	if result.Summary.WaitingCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers in Waiting state — check image pulls and runtime issues", result.Summary.WaitingCount))
	}
	if result.Summary.ErrorExitCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers exited with error codes — review application logs", result.Summary.ErrorExitCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
