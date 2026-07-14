package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeClassResult is the container runtime class & OCI image compliance audit.
type RuntimeClassResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         RuntimeClassSummary    `json:"summary"`
	RuntimeClasses  []RuntimeClassEntry    `json:"runtimeClasses"`
	ByNode          []RuntimeClassNodeStat `json:"byNode"`
	PodsNoRuntime   []RuntimeClassPodGap   `json:"podsNoRuntimeClass"`
	ImageCompliance []ImageComplianceEntry `json:"imageCompliance"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// RuntimeClassSummary aggregates runtime class statistics.
type RuntimeClassSummary struct {
	TotalRuntimeClasses int  `json:"totalRuntimeClasses"`
	NodesWithRuntime    int  `json:"nodesWithRuntime"` // nodes with RuntimeClass support
	PodsUsingRuntime    int  `json:"podsUsingRuntime"` // pods with runtimeClassName set
	PodsNoRuntime       int  `json:"podsNoRuntime"`    // pods without runtimeClassName
	TotalPods           int  `json:"totalPods"`
	ImagesWithLatest    int  `json:"imagesWithLatest"` // images using :latest
	ImagesNoDigest      int  `json:"imagesNoDigest"`   // images without digest reference
	UnsafeImages        int  `json:"unsafeImages"`     // images from untrusted registries
	HasContainerd       bool `json:"hasContainerd"`
	HasCrio             bool `json:"hasCrio"`
	HasKata             bool `json:"hasKata"`
}

// RuntimeClassEntry describes a RuntimeClass resource.
type RuntimeClassEntry struct {
	Name       string `json:"name"`
	Handler    string `json:"handler"`
	Scheduling string `json:"scheduling"` // node selector info
	Status     string `json:"status"`
}

// RuntimeClassNodeStat shows runtime info per node.
type RuntimeClassNodeStat struct {
	NodeName         string `json:"nodeName"`
	ContainerRuntime string `json:"containerRuntime"`
	HasRuntimeClass  bool   `json:"hasRuntimeClass"`
	OSImage          string `json:"osImage"`
}

// RuntimeClassPodGap describes a pod without a runtime class.
type RuntimeClassPodGap struct {
	Namespace    string `json:"namespace"`
	PodName      string `json:"podName"`
	RuntimeClass string `json:"runtimeClass"`
	Severity     string `json:"severity"`
}

// ImageComplianceEntry describes an image compliance issue.
type ImageComplianceEntry struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleRuntimeClass audits container runtime class & OCI image compliance.
// GET /api/product/runtime-class
func (s *Server) handleRuntimeClass(w http.ResponseWriter, r *http.Request) {
	result := RuntimeClassResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. List RuntimeClasses
	runtimeClasses, err := rc.clientset.NodeV1().RuntimeClasses().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, rc := range runtimeClasses.Items {
			scheduling := "all nodes"
			if rc.Scheduling != nil && rc.Scheduling.NodeSelector != nil {
				scheduling = fmt.Sprintf("selector: %v", rc.Scheduling.NodeSelector)
			}

			result.RuntimeClasses = append(result.RuntimeClasses, RuntimeClassEntry{
				Name:       rc.Name,
				Handler:    rc.Handler,
				Scheduling: scheduling,
				Status:     "available",
			})

			result.Summary.TotalRuntimeClasses++

			switch rc.Handler {
			case "kata":
				result.Summary.HasKata = true
			case "runc":
				// default
			case "crun":
				// default
			}
		}
	}

	// 2. Check container runtime on nodes
	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			cr := node.Status.NodeInfo.ContainerRuntimeVersion
			runtimeType := "unknown"
			if strings.Contains(cr, "containerd") {
				runtimeType = "containerd"
				result.Summary.HasContainerd = true
			} else if strings.Contains(cr, "cri-o") || strings.Contains(cr, "crio") {
				runtimeType = "cri-o"
				result.Summary.HasCrio = true
			} else if strings.Contains(cr, "docker") {
				runtimeType = "docker"
			}

			result.ByNode = append(result.ByNode, RuntimeClassNodeStat{
				NodeName:         node.Name,
				ContainerRuntime: runtimeType,
				HasRuntimeClass:  result.Summary.TotalRuntimeClasses > 0,
				OSImage:          node.Status.NodeInfo.OSImage,
			})
			result.Summary.NodesWithRuntime++
		}
	}

	// 3. Check pods for runtimeClassName and image compliance
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		systemNamespaces := map[string]bool{
			"kube-system":     true,
			"kube-public":     true,
			"kube-node-lease": true,
		}

		knownTrustedRegistries := map[string]bool{
			"registry.k8s.io":   true,
			"k8s.gcr.io":        true,
			"gcr.io":            true,
			"docker.io":         true,
			"registry.iot2.win": true,
			"quay.io":           true,
		}

		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}
			result.Summary.TotalPods++

			hasRuntimeClass := pod.Spec.RuntimeClassName != nil
			if hasRuntimeClass {
				result.Summary.PodsUsingRuntime++
			} else {
				result.Summary.PodsNoRuntime++
				// Only flag if runtime classes exist
				if result.Summary.TotalRuntimeClasses > 0 {
					result.PodsNoRuntime = append(result.PodsNoRuntime, RuntimeClassPodGap{
						Namespace:    pod.Namespace,
						PodName:      pod.Name,
						RuntimeClass: "",
						Severity:     "low",
					})
				}
			}

			// Check image compliance for all containers
			allContainers := append([]corev1.Container{}, pod.Spec.Containers...)
			allContainers = append(allContainers, pod.Spec.InitContainers...)

			for _, c := range allContainers {
				image := c.Image

				// Check for :latest tag
				if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
					result.Summary.ImagesWithLatest++
					result.ImageCompliance = append(result.ImageCompliance, ImageComplianceEntry{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						Container: c.Name,
						Image:     image,
						Issue:     "Using :latest tag or no tag specified",
						Severity:  "medium",
					})
				}

				// Check for digest reference (sha256)
				if !strings.Contains(image, "@sha256:") {
					result.Summary.ImagesNoDigest++
				}

				// Check for untrusted registry
				isTrusted := false
				for registry := range knownTrustedRegistries {
					if strings.HasPrefix(strings.ToLower(image), registry) || strings.Contains(image, registry+"/") {
						isTrusted = true
						break
					}
				}
				if !isTrusted && strings.Contains(image, "/") {
					// Check if it's a trusted registry prefix
					parts := strings.SplitN(image, "/", 2)
					if len(parts) == 2 {
						registry := strings.Split(parts[0], ":")[0]
						if knownTrustedRegistries[registry] {
							isTrusted = true
						}
					}
				}
				if !isTrusted && !strings.Contains(image, "/") {
					// Official Docker Hub images (e.g. "nginx:1.21")
					isTrusted = true
				}
				if !isTrusted {
					result.Summary.UnsafeImages++
					result.ImageCompliance = append(result.ImageCompliance, ImageComplianceEntry{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						Container: c.Name,
						Image:     image,
						Issue:     "Image from untrusted registry",
						Severity:  "low",
					})
				}
			}
		}
	}

	// Sort results
	sort.Slice(result.PodsNoRuntime, func(i, j int) bool {
		return result.PodsNoRuntime[i].Severity > result.PodsNoRuntime[j].Severity
	})
	sort.Slice(result.ImageCompliance, func(i, j int) bool {
		return result.ImageCompliance[i].Severity > result.ImageCompliance[j].Severity
	})

	// Recommendations
	if result.Summary.ImagesWithLatest > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Replace %d :latest image tags with pinned versions for reproducibility", result.Summary.ImagesWithLatest))
	}
	if result.Summary.UnsafeImages > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Review %d images from untrusted registries", result.Summary.UnsafeImages))
	}
	if result.Summary.TotalRuntimeClasses > 0 && result.Summary.PodsNoRuntime > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Consider assigning runtimeClassName to %d pods for sandbox isolation", result.Summary.PodsNoRuntime))
	}
	if result.Summary.TotalRuntimeClasses == 0 {
		result.Recommendations = append(result.Recommendations,
			"Consider defining RuntimeClasses for sandbox-based isolation (kata, gVisor)")
	}

	// Health score
	score := 100
	score -= result.Summary.ImagesWithLatest * 2
	score -= result.Summary.UnsafeImages * 1
	if result.Summary.TotalRuntimeClasses == 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

var _ = nodev1.RuntimeClass{}
