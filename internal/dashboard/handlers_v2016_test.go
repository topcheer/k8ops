package dashboard

import "testing"

func TestVWConfigResult2016(t *testing.T) {
	r := VWConfigResult2016{Summary: VWConfigSummary2016{TotalConfigs: 5, WithTLS: 4}}
	if r.Summary.WithTLS != 4 {
		t.Errorf("expected 4")
	}
}
func TestVWConfigEntry2016(t *testing.T) {
	e := VWConfigEntry2016{Name: "wh-1", WebhookCount: 3, ServiceName: "wh-svc", HasTLS: true}
	if !e.HasTLS {
		t.Errorf("expected true")
	}
}
func TestIngClassResult2016(t *testing.T) {
	r := IngClassResult2016{Summary: IngClassSummary2016{TotalClasses: 3, HasDefault: true}}
	if !r.Summary.HasDefault {
		t.Errorf("expected true")
	}
}
func TestIngClassEntry2016(t *testing.T) {
	e := IngClassEntry2016{Name: "nginx", Controller: "k8s.io/nginx", IsDefault: true, HasParams: false}
	if !e.IsDefault {
		t.Errorf("expected true")
	}
}
func TestAPISvcResult2016(t *testing.T) {
	r := APISvcResult2016{Summary: APISvcSummary2016{TotalAPIServices: 10, AvailableCount: 9, UnavailableCount: 1}}
	if r.Summary.UnavailableCount != 1 {
		t.Errorf("expected 1")
	}
}
func TestAPISvcEntry2016(t *testing.T) {
	e := APISvcEntry2016{Name: "v1.custom.io", Group: "custom.io", Version: "v1", Available: true}
	if !e.Available {
		t.Errorf("expected true")
	}
}
func TestVWConfigSummary2016(t *testing.T) {
	s := VWConfigSummary2016{WithNSScope: 3}
	if s.WithNSScope != 3 {
		t.Errorf("expected 3")
	}
}
func TestAPISvcSummary2016(t *testing.T) {
	s := APISvcSummary2016{WithCA: 5}
	if s.WithCA != 5 {
		t.Errorf("expected 5")
	}
}
