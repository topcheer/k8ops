package dashboard

import "testing"

func TestIngressHealthResult1924(t *testing.T) {
	r := IngressHealthResult1924{
		Summary: IngressHealthSummary1924{TotalIngresses: 15, WithTLS: 10, WithoutTLS: 5, HostConflicts: 2},
	}
	if r.Summary.WithTLS != 10 {
		t.Errorf("expected 10, got %d", r.Summary.WithTLS)
	}
}

func TestIngressConflict1924(t *testing.T) {
	c := IngressConflict1924{Type: "cross-namespace-host", Host: "app.example.com", Ingress1: "ing-a", Ingress2: "ing-b"}
	if c.Host != "app.example.com" {
		t.Errorf("expected app.example.com")
	}
}

func TestIngressIssue1924(t *testing.T) {
	i := IngressIssue1924{Name: "web", Namespace: "prod", IssueType: "no-tls", Severity: "warning"}
	if i.Severity != "warning" {
		t.Errorf("expected warning")
	}
}

func TestJobLifecycleResult1924(t *testing.T) {
	r := JobLifecycleResult1924{
		Summary: JobLifecycleSummary1924{TotalJobs: 50, SucceededJobs: 42, FailedJobs: 5, StaleJobCount: 8, SuccessRate: 84.0},
	}
	if r.Summary.SuccessRate != 84.0 {
		t.Errorf("expected 84.0, got %f", r.Summary.SuccessRate)
	}
}

func TestCronJobEntry1924(t *testing.T) {
	c := CronJobEntry1924{Name: "backup", Namespace: "data", Schedule: "0 2 * * *", Suspended: false}
	if c.Schedule != "0 2 * * *" {
		t.Errorf("expected 0 2 * * *")
	}
}

func TestStaleJobEntry1924(t *testing.T) {
	s := StaleJobEntry1924{Name: "old-job", Namespace: "dev", Age: "15d", Reason: "no TTL"}
	if s.Age != "15d" {
		t.Errorf("expected 15d")
	}
}

func TestLeaderElectionResult1924(t *testing.T) {
	r := LeaderElectionResult1924{
		Summary: LeaderElectionSummary1924{TotalLeases: 10, ActiveHolders: 8, StaleLeases: 1, ControllerCount: 5},
	}
	if r.Summary.ActiveHolders != 8 {
		t.Errorf("expected 8, got %d", r.Summary.ActiveHolders)
	}
}

func TestLeaseEntry1924(t *testing.T) {
	l := LeaseEntry1924{Name: "controller-leader", Namespace: "kube-system", Holder: "pod-1", DurationSec: 15}
	if l.DurationSec != 15 {
		t.Errorf("expected 15, got %d", l.DurationSec)
	}
}
