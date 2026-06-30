package cost

import (
	"os"
	"testing"
)

// --- PricingFromEnv Tests ---

func TestPricingFromEnv_Default(t *testing.T) {
	os.Unsetenv("K8OPS_CLOUD_PROVIDER")
	os.Unsetenv("K8OPS_CPU_PRICE")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 28 {
		t.Errorf("Default CPUPricePerCore = %f, want 28", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 3.5 {
		t.Errorf("Default RAMPricePerGB = %f, want 3.5", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_AWSPreset(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "aws")
	os.Unsetenv("K8OPS_CPU_PRICE")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 31 {
		t.Errorf("AWS CPUPricePerCore = %f, want 31", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 4 {
		t.Errorf("AWS RAMPricePerGB = %f, want 4", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_AzurePreset(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "azure")
	os.Unsetenv("K8OPS_CPU_PRICE")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 27 {
		t.Errorf("Azure CPUPricePerCore = %f, want 27", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 3.2 {
		t.Errorf("Azure RAMPricePerGB = %f, want 3.2", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_GCPPreset(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "gcp")
	os.Unsetenv("K8OPS_CPU_PRICE")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 26 {
		t.Errorf("GCP CPUPricePerCore = %f, want 26", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 3.3 {
		t.Errorf("GCP RAMPricePerGB = %f, want 3.3", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_UnknownCloudFallsBack(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "digitalocean")
	os.Unsetenv("K8OPS_CPU_PRICE")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	// Unknown provider should fall back to default
	if p.CPUPricePerCore != 28 {
		t.Errorf("Unknown cloud CPUPricePerCore = %f, want 28 (default)", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 3.5 {
		t.Errorf("Unknown cloud RAMPricePerGB = %f, want 3.5 (default)", p.RAMPricePerGB)
	}
}

// --- applyEnvOverrides Tests ---

func TestPricingFromEnv_CPUOverrideOnAWS(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "aws")
	t.Setenv("K8OPS_CPU_PRICE", "50")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 50 {
		t.Errorf("Override CPUPricePerCore = %f, want 50", p.CPUPricePerCore)
	}
	// RAM should still be preset
	if p.RAMPricePerGB != 4 {
		t.Errorf("Non-overridden RAMPricePerGB = %f, want 4", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_RAMOverrideOnGCP(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "gcp")
	os.Unsetenv("K8OPS_CPU_PRICE")
	t.Setenv("K8OPS_RAM_PRICE", "7.5")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 26 {
		t.Errorf("Non-overridden CPUPricePerCore = %f, want 26", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 7.5 {
		t.Errorf("Override RAMPricePerGB = %f, want 7.5", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_BothOverrides(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "azure")
	t.Setenv("K8OPS_CPU_PRICE", "100")
	t.Setenv("K8OPS_RAM_PRICE", "10")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 100 {
		t.Errorf("Override CPUPricePerCore = %f, want 100", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 10 {
		t.Errorf("Override RAMPricePerGB = %f, want 10", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_InvalidCPUOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "aws")
	t.Setenv("K8OPS_CPU_PRICE", "not-a-number")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	// Should keep preset when override is invalid
	if p.CPUPricePerCore != 31 {
		t.Errorf("Invalid override should keep preset: CPUPricePerCore = %f, want 31", p.CPUPricePerCore)
	}
}

func TestPricingFromEnv_InvalidRAMOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "gcp")
	os.Unsetenv("K8OPS_CPU_PRICE")
	t.Setenv("K8OPS_RAM_PRICE", "abc")
	p := PricingFromEnv()
	if p.RAMPricePerGB != 3.3 {
		t.Errorf("Invalid override should keep preset: RAMPricePerGB = %f, want 3.3", p.RAMPricePerGB)
	}
}

func TestPricingFromEnv_ZeroCPUOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "aws")
	t.Setenv("K8OPS_CPU_PRICE", "0")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	// Zero is a valid float and >= 0, so it should be applied
	if p.CPUPricePerCore != 0 {
		t.Errorf("Zero CPU override = %f, want 0", p.CPUPricePerCore)
	}
}

func TestPricingFromEnv_NegativeCPUOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "default")
	t.Setenv("K8OPS_CPU_PRICE", "-5")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	// Negative values are rejected by the >= 0 guard
	if p.CPUPricePerCore != 28 {
		t.Errorf("Negative override should be rejected: CPUPricePerCore = %f, want 28", p.CPUPricePerCore)
	}
}

func TestPricingFromEnv_EmptyCPUOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "aws")
	t.Setenv("K8OPS_CPU_PRICE", "")
	os.Unsetenv("K8OPS_RAM_PRICE")
	p := PricingFromEnv()
	// Empty string should not override
	if p.CPUPricePerCore != 31 {
		t.Errorf("Empty override should keep preset: CPUPricePerCore = %f, want 31", p.CPUPricePerCore)
	}
}

func TestPricingFromEnv_FractionalOverride(t *testing.T) {
	t.Setenv("K8OPS_CLOUD_PROVIDER", "default")
	t.Setenv("K8OPS_CPU_PRICE", "33.5")
	t.Setenv("K8OPS_RAM_PRICE", "4.25")
	p := PricingFromEnv()
	if p.CPUPricePerCore != 33.5 {
		t.Errorf("Fractional CPU override = %f, want 33.5", p.CPUPricePerCore)
	}
	if p.RAMPricePerGB != 4.25 {
		t.Errorf("Fractional RAM override = %f, want 4.25", p.RAMPricePerGB)
	}
}

// --- Currency Tests ---

func TestPricingFromEnv_CurrencyAlwaysUSD(t *testing.T) {
	providers := []string{"aws", "azure", "gcp", "default", "unknown"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("K8OPS_CLOUD_PROVIDER", provider)
			os.Unsetenv("K8OPS_CPU_PRICE")
			os.Unsetenv("K8OPS_RAM_PRICE")
			p := PricingFromEnv()
			if p.Currency != "USD" {
				t.Errorf("Currency = %q, want USD for provider %q", p.Currency, provider)
			}
		})
	}
}
