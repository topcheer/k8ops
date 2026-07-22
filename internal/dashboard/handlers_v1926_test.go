package dashboard

import "testing"

func TestNamingAuditResult1926(t *testing.T) {
	r := NamingAuditResult1926{
		Summary: NamingAuditSummary1926{TotalResources: 200, CompliantCount: 185, ViolationCount: 15},
	}
	if r.Summary.CompliantCount != 185 {
		t.Errorf("expected 185, got %d", r.Summary.CompliantCount)
	}
}

func TestNamingViolation1926(t *testing.T) {
	v := NamingViolation1926{Name: "MyApp", Violation: "uppercase", Severity: "low", Suggestion: "myapp"}
	if v.Suggestion != "myapp" {
		t.Errorf("expected myapp")
	}
}

func TestEnvVarCatalogResult1926(t *testing.T) {
	r := EnvVarCatalogResult1926{
		Summary: EnvVarCatalogSummary1926{TotalEnvVars: 120, UniqueEnvVars: 45, SensitiveCount: 8, ConflictCount: 3},
	}
	if r.Summary.SensitiveCount != 8 {
		t.Errorf("expected 8, got %d", r.Summary.SensitiveCount)
	}
}

func TestEnvVarEntry1926(t *testing.T) {
	e := EnvVarEntry1926{Name: "DATABASE_URL", UsageCount: 5, Source: "direct", IsSensitive: false}
	if e.UsageCount != 5 {
		t.Errorf("expected 5")
	}
}

func TestEnvVarConflict1926(t *testing.T) {
	c := EnvVarConflict1926{EnvVar: "LOG_LEVEL", Values: []string{"debug", "info"}}
	if len(c.Values) != 2 {
		t.Errorf("expected 2 values")
	}
}

func TestAnnotationInventoryResult1926(t *testing.T) {
	r := AnnotationInventoryResult1926{
		Summary: AnnotationInventorySummary1926{TotalAnnotations: 500, UniqueKeys: 30, StandardKeys: 15, CustomKeys: 12, DeprecatedKeys: 3},
	}
	if r.Summary.DeprecatedKeys != 3 {
		t.Errorf("expected 3, got %d", r.Summary.DeprecatedKeys)
	}
}

func TestAnnotationEntry1926(t *testing.T) {
	a := AnnotationEntry1926{Key: "app.kubernetes.io/name", UsageCount: 50, Category: "standard", IsStandard: true}
	if !a.IsStandard {
		t.Errorf("expected standard")
	}
}

func TestAnnotationNSStat1926(t *testing.T) {
	s := AnnotationNSStat1926{Namespace: "prod", AnnotationCount: 100, UniqueKeys: 15}
	if s.UniqueKeys != 15 {
		t.Errorf("expected 15")
	}
}
