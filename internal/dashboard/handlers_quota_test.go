package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyQuotaStatus(t *testing.T) {
	tests := []struct {
		pct      float64
		expected QuotaStatus
	}{
		{50, QuotaOK},
		{69, QuotaOK},
		{70, QuotaWarning},
		{84, QuotaWarning},
		{85, QuotaCritical},
		{99.9, QuotaCritical},
		{100, QuotaCritical},
		{100.1, QuotaExceeded},
		{150, QuotaExceeded},
		{0, QuotaOK},
	}

	for _, tt := range tests {
		got := classifyQuotaStatus(tt.pct)
		if got != tt.expected {
			t.Errorf("classifyQuotaStatus(%.1f) = %s, want %s", tt.pct, got, tt.expected)
		}
	}
}

func TestQuotaStatusRank(t *testing.T) {
	if quotaStatusRank(QuotaExceeded) >= quotaStatusRank(QuotaCritical) {
		t.Error("exceeded should rank before critical")
	}
	if quotaStatusRank(QuotaCritical) >= quotaStatusRank(QuotaWarning) {
		t.Error("critical should rank before warning")
	}
	if quotaStatusRank(QuotaWarning) >= quotaStatusRank(QuotaOK) {
		t.Error("warning should rank before ok")
	}
	if quotaStatusRank(QuotaOK) >= quotaStatusRank(QuotaNoLimit) {
		t.Error("ok should rank before no-limit")
	}
}

func TestCalculateUsagePercent_CPU(t *testing.T) {
	used := resource.MustParse("500m")
	hard := resource.MustParse("1000m")
	pct := calculateUsagePercent(corev1.ResourceRequestsCPU, used, hard)
	if pct < 49.9 || pct > 50.1 {
		t.Errorf("expected ~50%% CPU usage, got %.1f%%", pct)
	}
}

func TestCalculateUsagePercent_Memory(t *testing.T) {
	used := resource.MustParse("800Mi")
	hard := resource.MustParse("1Gi")
	pct := calculateUsagePercent(corev1.ResourceRequestsMemory, used, hard)
	if pct < 77 || pct > 79 {
		t.Errorf("expected ~78%% memory usage, got %.1f%%", pct)
	}
}

func TestCalculateUsagePercent_Count(t *testing.T) {
	used := resource.MustParse("8")
	hard := resource.MustParse("10")
	pct := calculateUsagePercent("count/pods", used, hard)
	if pct != 80 {
		t.Errorf("expected 80%% count usage, got %.1f%%", pct)
	}
}

func TestCalculateUsagePercent_Storage(t *testing.T) {
	used := resource.MustParse("50Gi")
	hard := resource.MustParse("100Gi")
	pct := calculateUsagePercent("requests.storage", used, hard)
	if pct < 49.9 || pct > 50.1 {
		t.Errorf("expected ~50%% storage usage, got %.1f%%", pct)
	}
}

func TestCalculateUsagePercent_ZeroHard(t *testing.T) {
	used := resource.MustParse("5")
	hard := resource.MustParse("0")
	pct := calculateUsagePercent("count/pods", used, hard)
	if pct != 0 {
		t.Errorf("expected 0%% for zero hard limit, got %.1f%%", pct)
	}
}

func TestAnalyzeNamespaceQuota_NoQuota(t *testing.T) {
	nq := analyzeNamespaceQuota("default", nil, nil)

	if nq.HasQuota {
		t.Error("expected hasQuota=false")
	}
	if nq.WorstStatus != QuotaNoLimit {
		t.Errorf("expected no-limit, got %s", nq.WorstStatus)
	}
	if len(nq.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(nq.Items))
	}
}

func TestAnalyzeNamespaceQuota_WithQuota(t *testing.T) {
	rq := corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota-1", Namespace: "production"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("4"),
				corev1.ResourceRequestsMemory: resource.MustParse("8Gi"),
				"count/pods":                  resource.MustParse("20"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("3.5"),
				corev1.ResourceRequestsMemory: resource.MustParse("6Gi"),
				"count/pods":                  resource.MustParse("18"),
			},
		},
	}

	nq := analyzeNamespaceQuota("production", []corev1.ResourceQuota{rq}, nil)

	if !nq.HasQuota {
		t.Error("expected hasQuota=true")
	}
	if len(nq.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(nq.Items))
	}

	// Pods should be 90% (warning level at minimum)
	var podsItem QuotaItem
	for _, item := range nq.Items {
		if item.Resource == "count/pods" {
			podsItem = item
			break
		}
	}
	if podsItem.UsagePercent < 89 || podsItem.UsagePercent > 91 {
		t.Errorf("expected ~90%% pods usage, got %.1f%%", podsItem.UsagePercent)
	}
	if podsItem.Status != QuotaCritical {
		t.Errorf("expected critical status for 90%% usage, got %s", podsItem.Status)
	}
}

func TestAnalyzeNamespaceQuota_Exceeded(t *testing.T) {
	rq := corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota-1", Namespace: "over-limit"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("2"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("3"),
			},
		},
	}

	nq := analyzeNamespaceQuota("over-limit", []corev1.ResourceQuota{rq}, nil)

	if nq.WorstStatus != QuotaExceeded {
		t.Errorf("expected exceeded, got %s", nq.WorstStatus)
	}
	if nq.ExceededCount != 1 {
		t.Errorf("expected 1 exceeded, got %d", nq.ExceededCount)
	}
}

func TestAnalyzeNamespaceQuota_WithLimitRange(t *testing.T) {
	lr := corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "default"},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
	}

	nq := analyzeNamespaceQuota("default", nil, []corev1.LimitRange{lr})

	if !nq.HasLimitRange {
		t.Error("expected hasLimitRange=true")
	}
	if len(nq.LimitRanges) == 0 {
		t.Error("expected at least 1 limit range item")
	}
}

func TestAnalyzeNamespaceQuota_ItemsSorted(t *testing.T) {
	rq := corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("10"),
				corev1.ResourceRequestsMemory: resource.MustParse("100Gi"),
				"count/pods":                  resource.MustParse("100"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("8"),    // 80%
				corev1.ResourceRequestsMemory: resource.MustParse("50Gi"), // 50%
				"count/pods":                  resource.MustParse("90"),   // 90%
			},
		},
	}

	nq := analyzeNamespaceQuota("ns", []corev1.ResourceQuota{rq}, nil)

	if len(nq.Items) < 3 {
		t.Fatalf("expected 3 items, got %d", len(nq.Items))
	}
	// First item should have highest usage (pods at 90%)
	if nq.Items[0].UsagePercent < 89 {
		t.Errorf("expected first item to be highest usage, got %.1f%%", nq.Items[0].UsagePercent)
	}
}

func TestQuotaReport_JSON(t *testing.T) {
	report := QuotaReport{
		Summary: QuotaSummary{
			TotalNamespaces:   10,
			WithQuota:         6,
			WithoutQuota:      4,
			WithLimitRange:    3,
			ExceededResources: 2,
			CriticalResources: 5,
			ByStatus:          map[string]int{"ok": 15, "warning": 5, "critical": 3, "exceeded": 2},
		},
		Namespaces: []NamespaceQuota{
			{Namespace: "prod", HasQuota: true, WorstStatus: QuotaCritical, ExceededCount: 0},
		},
		TopOffenders: []NamespaceQuota{
			{Namespace: "prod", WorstStatus: QuotaCritical},
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded QuotaReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Summary.TotalNamespaces != 10 {
		t.Errorf("expected 10 namespaces, got %d", decoded.Summary.TotalNamespaces)
	}
	if len(decoded.Namespaces) != 1 {
		t.Errorf("expected 1 namespace entry, got %d", len(decoded.Namespaces))
	}
	if len(decoded.TopOffenders) != 1 {
		t.Errorf("expected 1 offender, got %d", len(decoded.TopOffenders))
	}
}

func TestSummarizeQuotaIssues(t *testing.T) {
	items := []QuotaItem{
		{Resource: "requests.cpu", Hard: "4", Used: "5", UsagePercent: 125, Status: QuotaExceeded},
		{Resource: "requests.memory", Hard: "8Gi", Used: "7Gi", UsagePercent: 87.5, Status: QuotaCritical},
		{Resource: "count/pods", Hard: "20", Used: "10", UsagePercent: 50, Status: QuotaOK},
	}

	issues := summarizeQuotaIssues(items)

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestRoundTo_QuotaContext(t *testing.T) {
	val := roundTo(87.54321, 1)
	if val != 87.5 {
		t.Errorf("expected 87.5, got %.1f", val)
	}
}

// Ensure time import is used
var _ = time.Now
