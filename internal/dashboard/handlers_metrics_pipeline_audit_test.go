package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestMetricsPipelineEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/metrics-pipeline-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleMetricsPipelineHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result MetricsPipelineAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.HealthScore >= 50 {
		t.Errorf("expected low score with no components, got %d", result.HealthScore)
	}
}

func TestMetricsPipelineFull(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-0", Namespace: "monitoring"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "prometheus", Ready: true, Image: "prom:v2.40.0"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "node-exporter-abc", Namespace: "monitoring"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "exporter", Ready: true}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-state-metrics-def", Namespace: "monitoring"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "ksm", Ready: true}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "grafana-xyz", Namespace: "monitoring"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "grafana", Ready: true}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "alertmanager-0", Namespace: "monitoring"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "am", Ready: true}}},
		},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/metrics-pipeline-audit", clientset)
	w := httptest.NewRecorder()
	s.handleMetricsPipelineHealth(w, req)
	var result MetricsPipelineAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Summary.HasScraper {
		t.Error("expected scraper")
	}
	if !result.Summary.HasNodeExporter {
		t.Error("expected node-exporter")
	}
	if !result.Summary.HasVisualizer {
		t.Error("expected visualizer")
	}
	if !result.Summary.HasAlerter {
		t.Error("expected alerter")
	}
	if result.Scorecard.Overall < 80 {
		t.Errorf("expected >=80, got %d", result.Scorecard.Overall)
	}
}

func TestMetricsPipelineGaps(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/metrics-pipeline-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleMetricsPipelineHealth(w, req)
	var result MetricsPipelineAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	// With no components, should have critical gaps
	foundCritical := false
	for _, gap := range result.Gaps {
		if gap.Severity == "critical" {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Error("expected critical gap with no components")
	}
}

func TestMetricsPipelineCoverage(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app", Namespace: "prod",
				Annotations: map[string]string{"prometheus.io/scrape": "true"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/metrics-pipeline-audit", clientset)
	w := httptest.NewRecorder()
	s.handleMetricsPipelineHealth(w, req)
	var result MetricsPipelineAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	// 1 of 2 namespaces has monitoring
	if result.Coverage.NSCoveragePct != 50 {
		t.Errorf("expected 50%%, got %f", result.Coverage.NSCoveragePct)
	}
}

func TestMetricsPipelineRecs(t *testing.T) {
	result := MetricsPipelineAuditResult{
		Summary:   MetricsPipelineAuditSummary{HasScraper: true, HealthyComponents: 3},
		Gaps:      []MetricsAuditGap{{Severity: "high", Issue: "No grafana", Suggestion: "Deploy it"}},
		Scorecard: MetricsAuditScorecard{Overall: 65},
		Coverage:  MetricsAuditCoverage{NSCoveragePct: 30},
	}
	recs := generateMetricsPipelineRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundScorecard := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "scorecard") {
			foundScorecard = true
		}
	}
	if !foundScorecard {
		t.Error("expected scorecard recommendation")
	}
}
