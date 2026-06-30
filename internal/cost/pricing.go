// Package cost provides resource cost estimation for Kubernetes clusters.
// It calculates monthly costs based on pod resource requests and configurable
// pricing, and generates right-sizing recommendations.
package cost

import (
	"os"
	"strconv"
)

// Pricing holds the unit prices used for monthly cost estimation.
type Pricing struct {
	// CPUPricePerCore is the monthly cost of one CPU core in USD.
	CPUPricePerCore float64 `json:"cpuPricePerCore"`
	// RAMPricePerGB is the monthly cost of one GB of RAM in USD.
	RAMPricePerGB float64 `json:"ramPricePerGB"`
	// Currency is the ISO 4217 currency code.
	Currency string `json:"currency"`
}

// DefaultPricing returns pricing that represents an average across
// major cloud providers (AWS, Azure, GCP) for on-demand compute.
func DefaultPricing() Pricing {
	return Pricing{
		CPUPricePerCore: 28.0, // avg $28/core/month across AWS/Azure/GCP
		RAMPricePerGB:   3.5,  // avg $3.5/GB/month
		Currency:        "USD",
	}
}

// PricingFromEnv returns pricing configured from environment variables,
// falling back to defaults for any that are unset.
//
// Env vars:
//   - K8OPS_CPU_PRICE  — override CPU price per core (USD/month)
//   - K8OPS_RAM_PRICE  — override RAM price per GB (USD/month)
//   - K8OPS_CLOUD_PROVIDER — preset: "aws", "azure", "gcp", or "default"
func PricingFromEnv() Pricing {
	switch os.Getenv("K8OPS_CLOUD_PROVIDER") {
	case "aws":
		p := AWSPricing()
		applyEnvOverrides(&p)
		return p
	case "azure":
		p := AzurePricing()
		applyEnvOverrides(&p)
		return p
	case "gcp":
		p := GCPPricing()
		applyEnvOverrides(&p)
		return p
	default:
		p := DefaultPricing()
		applyEnvOverrides(&p)
		return p
	}
}

// AWSPricing returns representative AWS on-demand pricing.
func AWSPricing() Pricing {
	return Pricing{
		CPUPricePerCore: 31.0, // ~$0.0347/hour/core (m5 average)
		RAMPricePerGB:   4.0,
		Currency:        "USD",
	}
}

// AzurePricing returns representative Azure on-demand pricing.
func AzurePricing() Pricing {
	return Pricing{
		CPUPricePerCore: 27.0, // ~$0.0302/hour/core (D-series average)
		RAMPricePerGB:   3.2,
		Currency:        "USD",
	}
}

// GCPPricing returns representative GCP on-demand pricing.
func GCPPricing() Pricing {
	return Pricing{
		CPUPricePerCore: 26.0, // ~$0.0291/hour/core (n2 average)
		RAMPricePerGB:   3.3,
		Currency:        "USD",
	}
}

// applyEnvOverrides applies K8OPS_CPU_PRICE and K8OPS_RAM_PRICE overrides in place.
func applyEnvOverrides(p *Pricing) {
	if v := os.Getenv("K8OPS_CPU_PRICE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			p.CPUPricePerCore = f
		}
	}
	if v := os.Getenv("K8OPS_RAM_PRICE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			p.RAMPricePerGB = f
		}
	}
}
