package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthServer_Live(t *testing.T) {
	t.Parallel()

	h := NewHealthServer()
	handler := h.Handler()

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var status HealthStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if status.Status != "alive" {
		t.Errorf("expected status 'alive', got %q", status.Status)
	}
}

func TestHealthServer_Ready(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ready      bool
		wantCode   int
		wantStatus string
	}{
		{
			name:       "not ready",
			ready:      false,
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not ready",
		},
		{
			name:       "ready",
			ready:      true,
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := NewHealthServer()
			h.SetReady(tt.ready)
			handler := h.Handler()

			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, rec.Code)
			}

			var status HealthStatus
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if status.Status != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, status.Status)
			}
		})
	}
}

func TestHealthServer_Startup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		started    bool
		wantCode   int
		wantStatus string
	}{
		{
			name:       "not started",
			started:    false,
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "starting",
		},
		{
			name:       "started",
			started:    true,
			wantCode:   http.StatusOK,
			wantStatus: "started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := NewHealthServer()
			h.SetStarted(tt.started)
			handler := h.Handler()

			req := httptest.NewRequest(http.MethodGet, "/health/startup", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, rec.Code)
			}

			var status HealthStatus
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if status.Status != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, status.Status)
			}
		})
	}
}

func TestHealthServer_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := NewHealthServer()
	handler := h.Handler()

	endpoints := []string{"/health/live", "/health/ready", "/health/startup"}
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, endpoint := range endpoints {
		for _, method := range methods {
			t.Run(method+" "+endpoint, func(t *testing.T) {
				req := httptest.NewRequest(method, endpoint, nil)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusMethodNotAllowed {
					t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
				}
			})
		}
	}
}

func TestHealthServer_StateAccessors(t *testing.T) {
	t.Parallel()

	h := NewHealthServer()

	if h.IsReady() {
		t.Error("expected IsReady() to be false initially")
	}
	if h.IsStarted() {
		t.Error("expected IsStarted() to be false initially")
	}

	h.SetReady(true)
	h.SetStarted(true)

	if !h.IsReady() {
		t.Error("expected IsReady() to be true after SetReady(true)")
	}
	if !h.IsStarted() {
		t.Error("expected IsStarted() to be true after SetStarted(true)")
	}

	h.SetReady(false)
	h.SetStarted(false)

	if h.IsReady() {
		t.Error("expected IsReady() to be false after SetReady(false)")
	}
	if h.IsStarted() {
		t.Error("expected IsStarted() to be false after SetStarted(false)")
	}
}

func TestHealthServer_StartAndShutdown(t *testing.T) {
	t.Parallel()

	h := NewHealthServer()

	if err := h.StartAsync(":0"); err != nil {
		t.Fatalf("failed to start health server: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.Shutdown(ctx); err != nil {
		t.Errorf("failed to shutdown health server: %v", err)
	}
}

func TestHealthServer_ShutdownWithoutStart(t *testing.T) {
	t.Parallel()

	h := NewHealthServer()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.Shutdown(ctx); err != nil {
		t.Errorf("shutdown without start should not error: %v", err)
	}
}
