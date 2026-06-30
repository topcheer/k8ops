// Package k8s — shared helper functions for k8s tools.
package k8s

import (
	"fmt"
	"time"
)

// derefInt32 safely dereferences a *int32, returning defaultVal on nil.
func derefInt32(p *int32, defaultVal int32) int32 {
	if p != nil {
		return *p
	}
	return defaultVal
}

// derefStr safely dereferences a *string, returning "" on nil.
func derefStr(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// formatAge returns a human-readable duration since the given time.
func formatAge(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
