package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// HardeningScoreResult provides a comprehensive security hardening posture
// score by evaluating multiple security dimensions. It aggregates findings
// across Pod Security Standards, RBAC, network policies, secrets management,
// admission control, and runtime security into a single actionable score// with prioritized remediation guidance.
type HardeningScoreResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	OverallScore    int             `json:"overallScore"`
	Grade           string          `json:"grade"`
	Dimensions      []HardeningDim  `json:"dimensions"`
	TopRisks        []HardeningRisk `json:"topRisks"`
	ByNamespace     []HardeningNS   `json:"byNamespace"`
	ComplianceMap   map[string]int  `json:"complianceMap"` // framework -> score
	Recommendations []string        `json:"recommendations"`
}

type HardeningDim struct {
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Score       int     `json:"score"`
	Weight      float64 `json:"weight"`
	MaxScore    int     `json:"maxScore"`
	Findings    int     `json:"findings"`
	Criticals   int     `json:"criticals"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
}

type HardeningRisk struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Finding   string `json:"finding"`
	FixAction string `json:"fixAction"`
}

type HardeningNS struct {
	Namespace string `json:"namespace"`
	Score     int    `json:"score"`
	Workloads int    `json:"workloads"`
	Risks     int    `json:"risks"`
}

// handleHardeningScore handles GET /api/security/hardening-score
func (s *Server) handleHardeningScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HardeningScoreResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	clusterRoles, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	var allRisks []HardeningRisk
	nsMap := make(map[string]*HardeningNS)

	// === Dimension 1: Pod Security Standards ===
	pssScore := 100
	pssFindings := 0
	pssCriticals := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			sc := c.SecurityContext
			if sc == nil {
				pssFindings++
				pssScore -= 3
				allRisks = append(allRisks, HardeningRisk{
					Workload: d.Name, Namespace: d.Namespace,
					Category: "PSS", Severity: "medium",
					Finding:   "容器缺少 securityContext",
					FixAction: "添加 securityContext.runAsNonRoot: true",
				})
				continue
			}
			if sc.Privileged != nil && *sc.Privileged {
				pssCriticals++
				pssScore -= 10
				allRisks = append(allRisks, HardeningRisk{
					Workload: d.Name, Namespace: d.Namespace,
					Category: "PSS", Severity: "critical",
					Finding:   "特权容器运行",
					FixAction: "设置 privileged: false",
				})
			}
			if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				pssFindings++
				pssScore -= 2
				allRisks = append(allRisks, HardeningRisk{
					Workload: d.Name, Namespace: d.Namespace,
					Category: "PSS", Severity: "medium",
					Finding:   "以 root 用户运行",
					FixAction: "设置 runAsNonRoot: true",
				})
			}
			if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
				pssFindings++
				pssScore -= 3
			}
			if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				pssFindings++
				pssScore -= 2
			}
		}
	}
	if pssScore < 0 {
		pssScore = 0
	}

	// === Dimension 2: Network Security ===
	netScore := 100
	nsCount := 0
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		nsCount++
	}
	netpolNamespaces := make(map[string]bool)
	for _, np := range netpols.Items {
		netpolNamespaces[np.Namespace] = true
	}
	nsNetpolCoverage := 0
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		if netpolNamespaces[ns.Name] {
			nsNetpolCoverage++
		}
	}
	if nsCount > 0 {
		coverage := pctInt(nsNetpolCoverage, nsCount)
		netScore = int(coverage)
		if coverage < 50 {
			allRisks = append(allRisks, HardeningRisk{
				Workload: "*", Namespace: "*",
				Category: "Network", Severity: "high",
				Finding:   fmt.Sprintf("仅 %d/%d 命名空间有网络策略", nsNetpolCoverage, nsCount),
				FixAction: "为所有命名空间添加默认拒绝网络策略",
			})
		}
	}

	// === Dimension 3: Secrets Management ===
	secretScore := 100
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	staleSecrets := 0
	plaintextSecrets := 0
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		// Check for plaintext secrets in env vars of deployments
	}
	// Check env vars for secrets
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					// Using secret ref is good
				} else if env.Value != "" && isSensitiveEnvVar(env.Name) {
					plaintextSecrets++
					secretScore -= 2
					allRisks = append(allRisks, HardeningRisk{
						Workload: d.Name, Namespace: d.Namespace,
						Category: "Secrets", Severity: "high",
						Finding:   fmt.Sprintf("环境变量 %s 包含明文敏感信息", env.Name),
						FixAction: "使用 SecretKeyRef 引用 Kubernetes Secret",
					})
				}
			}
		}
	}
	// Stale secrets (old creation timestamp)
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		age := time.Since(sec.CreationTimestamp.Time)
		if age > 90*24*time.Hour {
			staleSecrets++
		}
	}
	if staleSecrets > 10 {
		secretScore -= 10
	}
	if secretScore < 0 {
		secretScore = 0
	}

	// === Dimension 4: RBAC Hardening ===
	rbacScore := 100
	clusterAdminBindings := 0
	for _, rb := range roleBindings.Items {
		if rb.RoleRef.Kind == "ClusterRole" && (rb.RoleRef.Name == "cluster-admin" || rb.RoleRef.Name == "admin") {
			clusterAdminBindings++
		}
	}
	if clusterAdminBindings > 5 {
		rbacScore -= 15
		allRisks = append(allRisks, HardeningRisk{
			Workload: "*", Namespace: "*",
			Category: "RBAC", Severity: "medium",
			Finding:   fmt.Sprintf("%d 个 cluster-admin 角色绑定，存在权限过大的风险", clusterAdminBindings),
			FixAction: "使用最小权限原则，以 namespace-scoped 角色替代",
		})
	}
	// Check for wildcard permissions in cluster roles
	wildcardRoles := 0
	for _, cr := range clusterRoles.Items {
		for _, rule := range cr.Rules {
			for _, api := range rule.APIGroups {
				if api == "*" {
					wildcardRoles++
					break
				}
			}
		}
	}
	if wildcardRoles > 3 {
		rbacScore -= 10
	}
	if rbacScore < 0 {
		rbacScore = 0
	}

	// === Dimension 5: Admission Control ===
	admissionScore := 100
	// Check PSA labels on namespaces
	psEnforced := 0
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			psEnforced++ // system namespaces usually enforce it
			continue
		}
		if ns.Labels["pod-security.kubernetes.io/enforce"] != "" {
			psEnforced++
		}
	}
	if nsCount > 0 {
		psPct := pctInt(psEnforced-len(namespaces.Items)+nsCount, nsCount)
		if psPct < 50 {
			admissionScore -= 20
			allRisks = append(allRisks, HardeningRisk{
				Workload: "*", Namespace: "*",
				Category: "Admission", Severity: "high",
				Finding:   fmt.Sprintf("仅 %.0f%% 命名空间启用了 Pod Security Admission", psPct),
				FixAction: "为所有命名空间设置 pod-security.kubernetes.io/enforce=restricted",
			})
		}
	}
	if admissionScore < 0 {
		admissionScore = 0
	}

	// === Dimension 6: Image Security ===
	imageScore := 100
	latestCount := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if strings.HasSuffix(c.Image, ":latest") || (!strings.Contains(c.Image[strings.LastIndex(c.Image, "/")+1:], ":") && !strings.Contains(c.Image, "@")) {
				latestCount++
			}
		}
	}
	if len(pods.Items) > 0 && latestCount > 5 {
		imageScore -= 20
		allRisks = append(allRisks, HardeningRisk{
			Workload: "*", Namespace: "*",
			Category: "Image", Severity: "medium",
			Finding:   fmt.Sprintf("%d 个容器使用 :latest 或无标签镜像", latestCount),
			FixAction: "使用固定版本标签或 digest 引用",
		})
	}
	if imageScore < 0 {
		imageScore = 0
	}

	// Build dimensions
	result.Dimensions = []HardeningDim{
		{Name: "Pod Security Standards", Category: "PSS", Score: pssScore, Weight: 0.20, MaxScore: 100, Findings: pssFindings, Criticals: pssCriticals, Description: "容器安全上下文、特权、非 root 运行", Status: hardeningDimStatus(pssScore)},
		{Name: "Network Security", Category: "Network", Score: netScore, Weight: 0.20, MaxScore: 100, Findings: nsCount - nsNetpolCoverage, Description: "网络策略覆盖率和流量隔离", Status: hardeningDimStatus(netScore)},
		{Name: "Secrets Management", Category: "Secrets", Score: secretScore, Weight: 0.15, MaxScore: 100, Findings: plaintextSecrets + staleSecrets, Description: "密钥管理、明文检测、密钥轮换", Status: hardeningDimStatus(secretScore)},
		{Name: "RBAC Hardening", Category: "RBAC", Score: rbacScore, Weight: 0.15, MaxScore: 100, Findings: clusterAdminBindings + wildcardRoles, Description: "角色绑定、最小权限、通配符权限", Status: hardeningDimStatus(rbacScore)},
		{Name: "Admission Control", Category: "Admission", Score: admissionScore, Weight: 0.15, MaxScore: 100, Findings: nsCount - (psEnforced - (len(namespaces.Items) - nsCount)), Description: "Pod Security Admission、策略执行", Status: hardeningDimStatus(admissionScore)},
		{Name: "Image Security", Category: "Image", Score: imageScore, Weight: 0.15, MaxScore: 100, Findings: latestCount, Description: "镜像标签管理、digest 固定", Status: hardeningDimStatus(imageScore)},
	}

	// Weighted overall score
	weightedTotal := 0.0
	for _, d := range result.Dimensions {
		weightedTotal += float64(d.Score) * d.Weight
	}
	result.OverallScore = int(weightedTotal)
	switch {
	case result.OverallScore >= 80:
		result.Grade = "A"
	case result.OverallScore >= 65:
		result.Grade = "B"
	case result.OverallScore >= 50:
		result.Grade = "C"
	case result.OverallScore >= 35:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	// Top risks (sorted by severity)
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sort.Slice(allRisks, func(i, j int) bool {
		return sevOrder[allRisks[i].Severity] < sevOrder[allRisks[j].Severity]
	})
	if len(allRisks) > 30 {
		allRisks = allRisks[:30]
	}
	result.TopRisks = allRisks

	// Namespace scoring
	for _, risk := range allRisks {
		ns := risk.Namespace
		if ns == "*" {
			continue
		}
		if _, ok := nsMap[ns]; !ok {
			nsMap[ns] = &HardeningNS{Namespace: ns, Score: 100}
		}
		nsMap[ns].Risks++
		sevPenalty := 5
		if risk.Severity == "critical" {
			sevPenalty = 15
		} else if risk.Severity == "high" {
			sevPenalty = 10
		}
		nsMap[ns].Score -= sevPenalty
	}
	for _, ns := range nsMap {
		if ns.Score < 0 {
			ns.Score = 0
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Score < result.ByNamespace[j].Score
	})

	// Compliance map
	result.ComplianceMap = map[string]int{
		"CIS Benchmark":  minIntVal(pssScore, rbacScore),
		"SOC2":           minIntVal(netScore, admissionScore),
		"PCI-DSS":        minIntVal(secretScore, netScore),
		"HIPAA":          minIntVal(secretScore, admissionScore),
		"NIST SP 800-53": minIntVal(rbacScore, admissionScore),
	}

	result.Recommendations = buildHardeningRecs(&result)

	writeJSON(w, result)
}

func hardeningDimStatus(score int) string {
	if score >= 80 {
		return "healthy"
	} else if score >= 60 {
		return "warning"
	} else if score >= 40 {
		return "at-risk"
	}
	return "critical"
}

func isSensitiveEnvVar(name string) bool {
	lower := strings.ToLower(name)
	sensitivePatterns := []string{"password", "passwd", "secret", "token", "api_key", "apikey", "access_key", "private_key", "credential"}
	for _, p := range sensitivePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func minIntVal(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildHardeningRecs(r *HardeningScoreResult) []string {
	recs := []string{}
	// Sort dimensions by score ascending
	sorted := make([]HardeningDim, len(r.Dimensions))
	copy(sorted, r.Dimensions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score < sorted[j].Score
	})
	for _, d := range sorted {
		if d.Score < 70 {
			recs = append(recs, fmt.Sprintf("[%s] 得分 %d/100: %s", d.Name, d.Score, d.Description))
		}
	}
	criticalCount := 0
	for _, risk := range r.TopRisks {
		if risk.Severity == "critical" {
			criticalCount++
		}
	}
	if criticalCount > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个严重风险，建议立即修复", criticalCount))
	}
	if r.OverallScore < 50 {
		recs = append(recs, "整体安全硬化得分较低，建议制定系统性的安全加固计划")
	}
	if len(recs) == 0 {
		recs = append(recs, "安全硬化状态良好，建议持续监控和定期审查")
	}
	return recs
}

var _ = intstr.FromInt
