package dashboard

import (
	"testing"
)

func TestAnyReady(t *testing.T) {
	components := []MetricsComponent{
		{Kind: "metrics-server", Ready: false},
		{Kind: "kube-state-metrics", Ready: true},
		{Kind: "node-exporter", Ready: false},
	}
	if !anyReady(components, "kube-state-metrics") {
		t.Error("expected kube-state-metrics to be ready")
	}
	if anyReady(components, "metrics-server") {
		t.Error("expected metrics-server to not be ready")
	}
	if anyReady(components, "nonexistent") {
		t.Error("expected nonexistent kind to not be ready")
	}
}

func TestComputeMetricsPipelineScore(t *testing.T) {
	// All missing → low score
	score := computeMetricsPipelineScore(MetricsPipelineSummary{}, 3)
	if score > 50 {
		t.Fatalf("all-missing score = %d, expected <= 50", score)
	}

	// All present and healthy
	score = computeMetricsPipelineScore(MetricsPipelineSummary{
		MetricsServerDetected: true,
		KSMDetected:           true,
		NodeExporterDetected:  true,
		UnhealthyComponents:   0,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Missing metrics-server is most critical
	score = computeMetricsPipelineScore(MetricsPipelineSummary{
		KSMDetected:          true,
		NodeExporterDetected: true,
	}, 0)
	if score > 80 {
		t.Fatalf("missing-ms score = %d, expected <= 80", score)
	}

	// Missing KSM is warning level
	score = computeMetricsPipelineScore(MetricsPipelineSummary{
		MetricsServerDetected: true,
		NodeExporterDetected:  true,
	}, 0)
	if score > 90 {
		t.Fatalf("missing-ksm score = %d, expected <= 90", score)
	}

	// Unhealthy components have high impact
	score = computeMetricsPipelineScore(MetricsPipelineSummary{
		MetricsServerDetected: true,
		KSMDetected:           true,
		NodeExporterDetected:  true,
		UnhealthyComponents:   3,
	}, 3)
	if score > 70 {
		t.Fatalf("unhealthy score = %d, expected <= 70", score)
	}
}
