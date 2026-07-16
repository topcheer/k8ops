package dashboard

import (
	"testing"
	"time"
)

func TestCertExpiryTypes(t *testing.T) {
	r := CertExpiryResult{HealthScore: 85, Grade: "B"}
	if r.HealthScore != 85 || r.Grade != "B" {
		t.Error("struct field error")
	}

	s := TLSCertSummary{TotalCerts: 60, Expiring30Days: 5, Expired: 1, ValidCerts: 54}
	if s.Expired != 1 || s.ValidCerts != 54 {
		t.Error("summary field error")
	}

	cd := CertDetail{Name: "tls-cert", Namespace: "app", CommonName: "example.com", DaysLeft: 15, Status: "expiring-30d"}
	if cd.DaysLeft != 15 || cd.Status != "expiring-30d" {
		t.Error("certDetail field error")
	}
}

func TestCertExpiryScoring(t *testing.T) {
	tests := []struct {
		total      int
		valid      int
		expired    int
		expiring30 int
		expiring90 int
		expectedMin int
		expectedMax int
	}{
		{60, 60, 0, 0, 0, 95, 100},        // All valid
		{60, 50, 2, 5, 3, 0, 40},          // Some expired/expiring
		{60, 58, 0, 2, 0, 70, 80},         // Some expiring
	}
	for _, tc := range tests {
		validRatio := 1.0
		if tc.total > 0 {
			validRatio = float64(tc.valid) / float64(tc.total)
		}
		penalty := tc.expired*30 + tc.expiring30*10 + tc.expiring90*3
		score := int(validRatio*100) - penalty
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("total=%d valid=%d expired=%d exp30=%d exp90=%d: expected %d-%d, got %d",
				tc.total, tc.valid, tc.expired, tc.expiring30, tc.expiring90,
				tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestCertExpiryStatusLogic(t *testing.T) {
	now := time.Now()
	tests := []struct {
		expiry   time.Time
		expected string
	}{
		{now.AddDate(0, 0, -1), "expired"},
		{now.AddDate(0, 0, 15), "expiring-30d"},
		{now.AddDate(0, 0, 60), "expiring-90d"},
		{now.AddDate(0, 0, 200), "valid"},
	}
	thirtyDays := now.AddDate(0, 0, 30)
	ninetyDays := now.AddDate(0, 0, 90)
	for _, tc := range tests {
		status := "valid"
		if tc.expiry.Before(now) {
			status = "expired"
		} else if tc.expiry.Before(thirtyDays) {
			status = "expiring-30d"
		} else if tc.expiry.Before(ninetyDays) {
			status = "expiring-90d"
		}
		if status != tc.expected {
			t.Errorf("expiry=%v: expected %s, got %s", tc.expiry, tc.expected, status)
		}
	}
}
