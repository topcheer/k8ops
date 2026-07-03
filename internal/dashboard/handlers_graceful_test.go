package dashboard

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDrainingGate_ReadyzDuringDrain(t *testing.T) {
	s := &Server{}

	// Before draining, readyz should return 200 (clientset is nil, so 503 for that reason)
	// But let's test the draining flag specifically
	s.draining.Store(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz during drain = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
	if rr.Body.String() != "draining\n" {
		t.Errorf("readyz body = %q, want %q", rr.Body.String(), "draining\n")
	}
}

func TestDrainingGate_ReadyzBeforeDrain(t *testing.T) {
	s := &Server{}
	// draining is false by default
	// clientset is nil so it returns 503 for "k8s client not initialized"
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	// Should NOT be "draining" — should be the client-not-initialized 503
	if rr.Body.String() == "draining\n" {
		t.Error("readyz returned 'draining' when not draining")
	}
}

func TestConnStateTracker_ActiveConnections(t *testing.T) {
	s := &Server{}

	// Simulate connection state transitions
	s.connStateTracker(nil, http.StateNew)
	if s.activeConns.Load() != 1 {
		t.Errorf("activeConns = %d, want 1 after StateNew", s.activeConns.Load())
	}

	s.connStateTracker(nil, http.StateActive)
	if s.activeConns.Load() != 2 {
		t.Errorf("activeConns = %d, want 2 after StateActive", s.activeConns.Load())
	}

	s.connStateTracker(nil, http.StateClosed)
	if s.activeConns.Load() != 1 {
		t.Errorf("activeConns = %d, want 1 after StateClosed", s.activeConns.Load())
	}

	s.connStateTracker(nil, http.StateHijacked)
	if s.activeConns.Load() != 0 {
		t.Errorf("activeConns = %d, want 0 after StateHijacked", s.activeConns.Load())
	}
}

func TestDrainStatus_Handler(t *testing.T) {
	now := time.Now()
	s := &Server{
		startTime: &now,
	}
	s.draining.Store(false)

	req := httptest.NewRequest(http.MethodGet, "/api/system/drain-status", nil)
	rr := httptest.NewRecorder()
	s.handleDrainStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("drain-status = %d, want %d", rr.Code, http.StatusOK)
	}

	// The response body contains JSON — just check it parses
	body := rr.Body.String()
	if !strings.Contains(body, "\"draining\":false") {
		t.Errorf("drain-status response does not contain draining:false: %s", body)
	}
	if !strings.Contains(body, "\"shutdownInitiated\":false") {
		t.Errorf("drain-status response does not contain shutdownInitiated:false: %s", body)
	}
}

func TestDrainStatus_DrainingTrue(t *testing.T) {
	s := &Server{}
	s.draining.Store(true)
	s.shutdownSignal.Store(true)
	s.activeConns.Store(5)

	req := httptest.NewRequest(http.MethodGet, "/api/system/drain-status", nil)
	rr := httptest.NewRecorder()
	s.handleDrainStatus(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "\"draining\":true") {
		t.Errorf("response should have draining:true: %s", body)
	}
	if !strings.Contains(body, "\"shutdownInitiated\":true") {
		t.Errorf("response should have shutdownInitiated:true: %s", body)
	}
	if !strings.Contains(body, "\"activeConnections\":5") {
		t.Errorf("response should have activeConnections:5: %s", body)
	}
}

// TestStop_SetsDrainingFlag verifies that Stop() sets the draining flag.
func TestStop_SetsDrainingFlag(t *testing.T) {
	// Create a minimal server with a real http.Server and logger
	s := &Server{
		log: slog.Default(),
	}
	s.server = &http.Server{}

	// Start a goroutine that checks draining flag shortly after Stop is called
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a short drain wait by calling Stop — it will fail since the server
	// isn't actually running, but the draining flag should still be set.
	go func() {
		_ = s.Stop(ctx)
	}()

	// Give it a moment to execute
	time.Sleep(100 * time.Millisecond)

	if !s.draining.Load() {
		t.Error("draining flag not set after Stop() called")
	}
	if !s.shutdownSignal.Load() {
		t.Error("shutdownSignal flag not set after Stop() called")
	}
}

func TestAtomicBool_Defaults(t *testing.T) {
	s := &Server{}

	if s.draining.Load() {
		t.Error("draining should default to false")
	}
	if s.shutdownSignal.Load() {
		t.Error("shutdownSignal should default to false")
	}
	if s.activeConns.Load() != 0 {
		t.Error("activeConns should default to 0")
	}
}

// Ensure atomic types are used correctly
func TestAtomicTypes(t *testing.T) {
	var b atomic.Bool
	b.Store(true)
	if !b.Load() {
		t.Error("atomic.Bool not working")
	}

	var i atomic.Int64
	i.Add(5)
	i.Add(-2)
	if i.Load() != 3 {
		t.Errorf("atomic.Int64 = %d, want 3", i.Load())
	}
}
