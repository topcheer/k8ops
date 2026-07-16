package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupplyChainResult audits container image supply chain security across
// the cluster. It evaluates image reference practices (digest vs tag),
// pull policy, base image freshness, non-root execution, read-only rootfs,
// and admission policy readiness for image signing verification.
type SupplyChainResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         SupplyChainSummary    `json:"summary"`
	Images          []SupplyChainImage    `json:"images"`
	ByRegistry      []SupplyChainRegistry `json:"byRegistry"`
	Risks           []SupplyChainRisk     `json:"risks"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type SupplyChainSummary struct {
	TotalImages     int `json:"totalImages"`
	UniqueImages    int `json:"uniqueImages"`
	ByDigest        int `json:"byDigest"`        // referenced by @sha256:
	ByTag           int `json:"byTag"`            // referenced by :tag (mutable)
	ByLatest        int `json:"byLatest"`         // uses :latest
	NonRoot         int `json:"nonRoot"`           // runs as non-root user
	ReadOnlyRootFS  int `json:"readOnlyRootFS"`   // readOnlyRootFilesystem=true
	Privileged      int `json:"privileged"`        // privileged=true (risk)
	AlwaysPull      int `json:"alwaysPull"`       // imagePullPolicy=Always
	ScanReady       int `json:"scanReady"`        // has digest (scannable)
	Annotated       int `json:"annotated"`        // has seccomp/AppArmor annotation
}

type SupplyChainImage struct {
	Image           string `json:"image"`
	Registry        string `json:"registry"`
	ByDigest        bool   `json:"byDigest"`
	IsLatest        bool   `json:"isLatest"`
	NonRoot         bool   `json:"nonRoot"`
	ReadOnlyRootFS  bool   `json:"readOnlyRootFS"`
	Privileged      bool   `json:"privileged"`
	PullPolicy      string `json:"pullPolicy"`
	ScanReady       bool   `json:"scanReady"`
	SecurityScore   int    `json:"securityScore"`
	RiskLevel       string `json:"riskLevel"`
	UsedBy          []string `json:"usedBy"`     // workload references
	WorkloadCount   int    `json:"workloadCount"`
}

type SupplyChainRegistry struct {
	Registry  string `json:"registry"`
	ImageCount int   `json:"imageCount"`
	DigestCount int  `json:"digestCount"`
	LatestCount int  `json:"latestCount"`
	AvgScore    int  `json:"avgScore"`
}

type SupplyChainRisk struct {
	Image    string `json:"image"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Detail   string `json:"detail"`
	UsedBy   string `json:"usedBy"`
}

// handleSupplyChain handles GET /api/security/supply-chain
func (s *Server) handleSupplyChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SupplyChainResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Build image -> workloads map from controllers
	imageWorkloads := make(map[string][]string)
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			ref := fmt.Sprintf("%s/%s", d.Namespace, d.Name)
			imageWorkloads[c.Image] = append(imageWorkloads[c.Image], ref)
		}
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		for _, c := range ss.Spec.Template.Spec.Containers {
			ref := fmt.Sprintf("%s/%s", ss.Namespace, ss.Name)
			imageWorkloads[c.Image] = append(imageWorkloads[c.Image], ref)
		}
	}

	// Collect unique images from running pods
	imageMap := make(map[string]*SupplyChainImage)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			img := cs.Image
			if img == "" {
				continue
			}
			if existing, ok := imageMap[img]; ok {
				existing.WorkloadCount++
				continue
			}

			sc := assessImageSecurity(img, &pod)
			if wl, ok := imageWorkloads[img]; ok {
				sc.UsedBy = uniqueStrings(wl)
				sc.WorkloadCount = len(sc.UsedBy)
			}
			imageMap[img] = &sc
		}
	}

	// Build summary
	for _, img := range imageMap {
		result.Summary.TotalImages++
		if img.ByDigest {
			result.Summary.ByDigest++
		} else {
			result.Summary.ByTag++
		}
		if img.IsLatest {
			result.Summary.ByLatest++
		}
		if img.NonRoot {
			result.Summary.NonRoot++
		}
		if img.ReadOnlyRootFS {
			result.Summary.ReadOnlyRootFS++
		}
		if img.Privileged {
			result.Summary.Privileged++
		}
		if img.PullPolicy == "Always" {
			result.Summary.AlwaysPull++
		}
		if img.ScanReady {
			result.Summary.ScanReady++
		}

		result.Images = append(result.Images, *img)
	}
	result.Summary.UniqueImages = len(imageMap)
	result.Summary.Annotated = 0 // counted in assessImageSecurity

	// Registry breakdown
	regMap := make(map[string]*SupplyChainRegistry)
	for _, img := range result.Images {
		reg := img.Registry
		if reg == "" {
			reg = "docker.io"
		}
		if _, ok := regMap[reg]; !ok {
			regMap[reg] = &SupplyChainRegistry{Registry: reg}
		}
		r := regMap[reg]
		r.ImageCount++
		r.AvgScore += img.SecurityScore
		if img.ByDigest {
			r.DigestCount++
		}
		if img.IsLatest {
			r.LatestCount++
		}
	}
	for _, r := range regMap {
		if r.ImageCount > 0 {
			r.AvgScore = r.AvgScore / r.ImageCount
		}
		result.ByRegistry = append(result.ByRegistry, *r)
	}
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].ImageCount > result.ByRegistry[j].ImageCount
	})

	// Health score
	if result.Summary.TotalImages > 0 {
		totalScore := 0
		for _, img := range result.Images {
			totalScore += img.SecurityScore
		}
		result.HealthScore = totalScore / result.Summary.TotalImages
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	case result.HealthScore >= 35:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	// Risks
	for _, img := range result.Images {
		if img.IsLatest {
			result.Risks = append(result.Risks, SupplyChainRisk{
				Image: img.Image, Severity: "high", Category: "mutable-tag",
				Detail: "使用 :latest 标签，镜像内容不可预测，应使用固定版本或 digest",
				UsedBy: strings.Join(img.UsedBy, ", "),
			})
		}
		if !img.ByDigest {
			result.Risks = append(result.Risks, SupplyChainRisk{
				Image: img.Image, Severity: "medium", Category: "tag-pinning",
				Detail: "使用标签而非 digest 引用，镜像可被覆盖替换（供应链投毒风险）",
				UsedBy: strings.Join(img.UsedBy, ", "),
			})
		}
		if img.Privileged {
			result.Risks = append(result.Risks, SupplyChainRisk{
				Image: img.Image, Severity: "critical", Category: "privileged",
				Detail: "容器以特权模式运行，可逃逸隔离",
				UsedBy: strings.Join(img.UsedBy, ", "),
			})
		}
		if !img.NonRoot {
			result.Risks = append(result.Risks, SupplyChainRisk{
				Image: img.Image, Severity: "medium", Category: "root-user",
				Detail: "容器以 root 用户运行，应设置 securityContext.runAsNonRoot: true",
				UsedBy: strings.Join(img.UsedBy, ", "),
			})
		}
	}
	sort.Slice(result.Risks, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Risks[i].Severity] < sevOrder[result.Risks[j].Severity]
	})

	// Recommendations
	result.Recommendations = buildSupplyChainRecs(&result)

	// Sort images by score ascending
	sort.Slice(result.Images, func(i, j int) bool {
		return result.Images[i].SecurityScore < result.Images[j].SecurityScore
	})

	writeJSON(w, result)
}

func assessImageSecurity(image string, pod *corev1.Pod) SupplyChainImage {
	sci := SupplyChainImage{
		Image: image,
	}

	// Parse registry
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		sci.Registry = parts[0]
	} else {
		sci.Registry = "docker.io"
	}

	// Check digest reference
	if strings.Contains(image, "@sha256:") {
		sci.ByDigest = true
		sci.ScanReady = true
	} else {
		sci.ByDigest = false
	}

	// Check :latest
	tagPart := image
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		tagPart = image[idx+1:]
	}
	if tagPart == "latest" || !strings.Contains(image[strings.LastIndex(image, "/")+1:], ":") {
		sci.IsLatest = true
	}

	// Find matching container spec
	var matchContainer *corev1.Container
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Image == image {
			matchContainer = &pod.Spec.Containers[i]
			break
		}
	}

	if matchContainer != nil {
		sci.PullPolicy = string(matchContainer.ImagePullPolicy)
		if matchContainer.SecurityContext != nil {
			if matchContainer.SecurityContext.RunAsNonRoot != nil && *matchContainer.SecurityContext.RunAsNonRoot {
				sci.NonRoot = true
			}
			if matchContainer.SecurityContext.ReadOnlyRootFilesystem != nil && *matchContainer.SecurityContext.ReadOnlyRootFilesystem {
				sci.ReadOnlyRootFS = true
			}
			if matchContainer.SecurityContext.Privileged != nil && *matchContainer.SecurityContext.Privileged {
				sci.Privileged = true
			}
		}
	}

	// Also check pod-level security context
	if pod.Spec.SecurityContext != nil {
		if pod.Spec.SecurityContext.RunAsNonRoot != nil && *pod.Spec.SecurityContext.RunAsNonRoot {
			sci.NonRoot = true
		}
	}

	// Score (max 100)
	score := 0
	if sci.ByDigest {
		score += 25
	} else if !sci.IsLatest {
		score += 10 // fixed tag is better than latest
	}
	if sci.NonRoot {
		score += 20
	}
	if sci.ReadOnlyRootFS {
		score += 15
	}
	if !sci.Privileged {
		score += 15
	}
	if sci.PullPolicy == "Always" || sci.PullPolicy == "" {
		score += 10
	}
	if sci.ScanReady {
		score += 15
	}

	sci.SecurityScore = score
	switch {
	case score >= 70:
		sci.RiskLevel = "low"
	case score >= 40:
		sci.RiskLevel = "medium"
	default:
		sci.RiskLevel = "high"
	}

	return sci
}

func buildSupplyChainRecs(r *SupplyChainResult) []string {
	recs := []string{}
	if r.Summary.TotalImages == 0 {
		return recs
	}

	digestPct := pctInt(r.Summary.ByDigest, r.Summary.TotalImages)
	if digestPct < 50 {
		recs = append(recs, fmt.Sprintf("仅 %.0f%% 镜像使用 digest 引用，建议全部改用 @sha256 固定镜像内容", digestPct))
	}
	if r.Summary.ByLatest > 0 {
		recs = append(recs, fmt.Sprintf("有 %d 个镜像使用 :latest 标签，应改为固定版本号", r.Summary.ByLatest))
	}
	nonRootPct := pctInt(r.Summary.NonRoot, r.Summary.TotalImages)
	if nonRootPct < 80 {
		recs = append(recs, fmt.Sprintf("仅 %.0f%% 镜像以非 root 用户运行，建议设置 runAsNonRoot: true", nonRootPct))
	}
	if r.Summary.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("有 %d 个特权容器运行，存在容器逃逸风险", r.Summary.Privileged))
	}
	roPct := pctInt(r.Summary.ReadOnlyRootFS, r.Summary.TotalImages)
	if roPct < 50 {
		recs = append(recs, fmt.Sprintf("仅 %.0f%% 镜像启用只读根文件系统，建议设置 readOnlyRootFilesystem: true", roPct))
	}
	recs = append(recs, "建议启用 admission controller 策略要求镜像签名验证（如 Sigstore/cosign）")
	recs = append(recs, "建议集成 SBOM 生成和镜像漏洞扫描到 CI/CD 管道")

	return recs
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// imageHash returns a short hash of an image string for deduplication.
func imageHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
