package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func cronTestReq(objects ...runtime.Object) (*Server, *http.Request) {
	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/operations/cronjobs/health", clientset)
	return &Server{}, req
}

func makeCronJob(name, ns, schedule string, suspend bool, lastSchedule *metav1.Time, lastSuccess *metav1.Time) *batchv1.CronJob {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       batchv1.CronJobSpec{Schedule: schedule, Suspend: &suspend},
		Status:     batchv1.CronJobStatus{},
	}
	if lastSchedule != nil {
		cj.Status.LastScheduleTime = lastSchedule
	}
	if lastSuccess != nil {
		cj.Status.LastSuccessfulTime = lastSuccess
	}
	return cj
}

func makeJobForCron(name, ns, cronName string, succeeded, failed int32, startTime, completionTime *metav1.Time) *batchv1.Job {
	ownerRef := metav1.OwnerReference{Kind: "CronJob", Name: cronName, UID: "test-uid", Controller: ptrBool(true)}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: batchv1.JobSpec{Completions: ptrInt32Ptr(1)},
		Status: batchv1.JobStatus{
			Succeeded: succeeded,
			StartTime: startTime,
		},
	}
	// Set CreationTimestamp from startTime for sorting
	if startTime != nil {
		job.CreationTimestamp = *startTime
	}
	if failed > 0 {
		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			Reason: "BackoffLimitExceeded",
		})
	}
	if completionTime != nil {
		job.Status.CompletionTime = completionTime
	}
	return job
}

// --- Suspended CronJob ---

func TestCron_Suspended(t *testing.T) {
	suspend := true
	cj := makeCronJob("paused", "app", "*/5 * * * *", suspend, nil, nil)
	srv, req := cronTestReq(cj)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.CronJobs) != 1 {
		t.Fatalf("expected 1 cronjob, got %d", len(result.CronJobs))
	}
	if result.CronJobs[0].Status != CronHealthSuspended {
		t.Errorf("expected suspended, got %s", result.CronJobs[0].Status)
	}
	if result.Summary.Suspended != 1 {
		t.Errorf("expected 1 suspended in summary, got %d", result.Summary.Suspended)
	}
}

// --- No runs ---

func TestCron_NoRuns(t *testing.T) {
	cj := makeCronJob("new", "app", "*/5 * * * *", false, nil, nil)
	srv, req := cronTestReq(cj)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.CronJobs[0].Status != CronHealthNoRuns {
		t.Errorf("expected no-runs, got %s", result.CronJobs[0].Status)
	}
}

// --- Healthy (all succeeded) ---

func TestCron_Healthy(t *testing.T) {
	now := metav1.Now()
	cj := makeCronJob("healthy", "app", "*/30 * * * *", false, &now, &now)
	job1 := makeJobForCron("healthy-1", "app", "healthy", 1, 0, &now, &now)
	job2 := makeJobForCron("healthy-2", "app", "healthy", 1, 0, &now, &now)
	srv, req := cronTestReq(cj, job1, job2)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	if c.Status != CronHealthHealthy {
		t.Errorf("expected healthy, got %s (issues: %v)", c.Status, c.Issues)
	}
	if c.SuccessfulJobs != 2 {
		t.Errorf("expected 2 successful, got %d", c.SuccessfulJobs)
	}
}

// --- Failing (3+ consecutive failures) ---

func TestCron_Failing(t *testing.T) {
	now := metav1.Now()
	cj := makeCronJob("broken", "app", "*/5 * * * *", false, &now, nil)
	job1 := makeJobForCron("broken-1", "app", "broken", 0, 1, &now, &now)
	job2 := makeJobForCron("broken-2", "app", "broken", 0, 1, &now, &now)
	job3 := makeJobForCron("broken-3", "app", "broken", 0, 1, &now, &now)
	srv, req := cronTestReq(cj, job1, job2, job3)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	if c.Status != CronHealthFailing {
		t.Errorf("expected failing, got %s (issues: %v)", c.Status, c.Issues)
	}
	if c.ConsecutiveFail != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", c.ConsecutiveFail)
	}
}

// --- Warning (1-2 failures) ---

func TestCron_WarningSingleFailure(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.Time{Time: now.Time.Add(-10 * time.Minute)}
	cj := makeCronJob("flaky", "app", "*/5 * * * *", false, &now, nil)
	// Earlier job succeeded, later job failed
	successJob := makeJobForCron("flaky-1", "app", "flaky", 1, 0, &earlier, &earlier)
	failJob := makeJobForCron("flaky-2", "app", "flaky", 0, 1, &now, &now)
	srv, req := cronTestReq(cj, failJob, successJob)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	if c.Status != CronHealthWarning {
		t.Errorf("expected warning for 1 recent failure, got %s (issues: %v)", c.Status, c.Issues)
	}
	if c.ConsecutiveFail != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", c.ConsecutiveFail)
	}
}

// --- Low success rate ---

func TestCron_LowSuccessRate(t *testing.T) {
	now := metav1.Now()
	cj := makeCronJob("unreliable", "app", "*/5 * * * *", false, &now, nil)
	jobs := []runtime.Object{cj}
	// 1 success, 3 failures (interleaved)
	jobs = append(jobs, makeJobForCron("j1", "app", "unreliable", 1, 0, &now, &now))
	jobs = append(jobs, makeJobForCron("j2", "app", "unreliable", 0, 1, &now, &now))
	jobs = append(jobs, makeJobForCron("j3", "app", "unreliable", 1, 0, &now, &now))
	jobs = append(jobs, makeJobForCron("j4", "app", "unreliable", 0, 1, &now, &now))

	srv, req := cronTestReq(jobs...)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	// 2 success / 4 total = 50% — borderline; should be at most warning
	if c.Status == CronHealthFailing {
		t.Errorf("expected at most warning for 50%% success rate with interleaved results, got failing")
	}
}

// --- Job status detection ---

func TestCron_JobStatusString(t *testing.T) {
	// Running
	runningJob := batchv1.Job{Status: batchv1.JobStatus{Active: 1}}
	if jobStatusString(runningJob) != "running" {
		t.Error("expected running")
	}

	// Succeeded
	successJob := batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}
	if jobStatusString(successJob) != "succeeded" {
		t.Error("expected succeeded")
	}

	// Failed
	failedJob := batchv1.Job{
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
			},
		},
	}
	if jobStatusString(failedJob) != "failed" {
		t.Error("expected failed")
	}
}

// --- Schedule interval estimation ---

func TestCron_EstimateScheduleInterval(t *testing.T) {
	tests := []struct {
		schedule string
		expected float64
	}{
		{"0 */6 * * *", 6},   // every 6 hours
		{"0 */12 * * *", 12}, // every 12 hours
		{"30 3 * * *", 24},   // daily at 3:30
		{"0 0 * * 0", 168},   // weekly
		{"*/5 * * * *", 0},   // sub-hour
		{"invalid", 0},       // invalid
	}
	for _, tc := range tests {
		got := estimateScheduleIntervalHours(tc.schedule)
		if got != tc.expected {
			t.Errorf("schedule %q: expected %.0f, got %.0f", tc.schedule, tc.expected, got)
		}
	}
}

// --- Sorting ---

func TestCron_Sorting(t *testing.T) {
	now := metav1.Now()
	// Healthy cron
	healthyCJ := makeCronJob("ok", "ns1", "*/5 * * * *", false, &now, &now)
	healthyJob := makeJobForCron("ok-1", "ns1", "ok", 1, 0, &now, &now)

	// Failing cron
	failingCJ := makeCronJob("bad", "ns2", "*/5 * * * *", false, &now, nil)
	failingJob := makeJobForCron("bad-1", "ns2", "bad", 0, 1, &now, &now)
	failingJob2 := makeJobForCron("bad-2", "ns2", "bad", 0, 1, &now, &now)
	failingJob3 := makeJobForCron("bad-3", "ns2", "bad", 0, 1, &now, &now)

	srv, req := cronTestReq(healthyCJ, healthyJob, failingCJ, failingJob, failingJob2, failingJob3)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.CronJobs) < 2 {
		t.Fatalf("expected 2 cronjobs, got %d", len(result.CronJobs))
	}
	// Failing should be first
	if result.CronJobs[0].Status != CronHealthFailing {
		t.Errorf("expected failing first, got %s", result.CronJobs[0].Status)
	}
}

// --- Summary ---

func TestCron_SummaryCounts(t *testing.T) {
	now := metav1.Now()
	suspended := true
	cj1 := makeCronJob("paused", "app", "*/5 * * * *", suspended, nil, nil)
	cj2 := makeCronJob("healthy", "app", "*/5 * * * *", false, &now, &now)
	job := makeJobForCron("healthy-1", "app", "healthy", 1, 0, &now, &now)

	srv, req := cronTestReq(cj1, cj2, job)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.TotalCronJobs != 2 {
		t.Errorf("expected 2 total, got %d", result.Summary.TotalCronJobs)
	}
	if result.Summary.Suspended != 1 {
		t.Errorf("expected 1 suspended, got %d", result.Summary.Suspended)
	}
	if result.Summary.ByStatus[string(CronHealthHealthy)] != 1 {
		t.Errorf("expected 1 healthy in byStatus, got %d", result.Summary.ByStatus[string(CronHealthHealthy)])
	}
}

// --- Job duration ---

func TestCron_JobDuration(t *testing.T) {
	start := metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
	end := metav1.Time{Time: time.Now()}
	cj := makeCronJob("timed", "app", "*/5 * * * *", false, &end, &end)
	job := makeJobForCron("timed-1", "app", "timed", 1, 0, &start, &end)

	srv, req := cronTestReq(cj, job)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	if len(c.RecentJobs) == 0 {
		t.Fatal("expected at least 1 recent job")
	}
	dur := c.RecentJobs[0].Duration
	if dur < 4*time.Minute || dur > 6*time.Minute {
		t.Errorf("expected ~5min duration, got %v", dur)
	}
}

// --- Empty cluster ---

func TestCron_EmptyCluster(t *testing.T) {
	srv, req := cronTestReq()
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.TotalCronJobs != 0 {
		t.Errorf("expected 0 cronjobs, got %d", result.Summary.TotalCronJobs)
	}
}

// --- Multiple namespaces ---

func TestCron_MultiNamespace(t *testing.T) {
	now := metav1.Now()
	cj1 := makeCronJob("cron-a", "ns-a", "*/5 * * * *", false, &now, &now)
	cj2 := makeCronJob("cron-b", "ns-b", "*/5 * * * *", false, &now, &now)

	srv, req := cronTestReq(cj1, cj2)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.CronJobs) != 2 {
		t.Fatalf("expected 2 cronjobs, got %d", len(result.CronJobs))
	}
}

// --- Stale detection ---

func TestCron_StaleDetection(t *testing.T) {
	// Last schedule was 3 days ago, but schedule is every 30 min
	staleTime := metav1.Time{Time: time.Now().Add(-72 * time.Hour)}
	cj := makeCronJob("stale", "app", "*/30 * * * *", false, &staleTime, &staleTime)
	job := makeJobForCron("stale-1", "app", "stale", 1, 0, &staleTime, &staleTime)

	srv, req := cronTestReq(cj, job)
	rr := httptest.NewRecorder()
	srv.handleCronJobHealth(rr, req)

	var result CronHealthResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	c := result.CronJobs[0]
	found := false
	for _, issue := range c.Issues {
		if containsStr(issue, "No execution") {
			found = true
		}
	}
	if !found {
		t.Error("expected stale detection issue")
	}
}

// --- Helpers ---

func ptrBool(b bool) *bool { return &b }

func ptrInt32Val(v int) *intstr.IntOrString {
	is := intstr.FromInt(v)
	return &is
}

// Suppress unused
var _ = ptrInt32Val
