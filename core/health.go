package core

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type HealthStatus struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type HealthServer struct {
	ready   atomic.Bool
	started atomic.Bool

	server *http.Server
	mu     sync.Mutex
}

func NewHealthServer() *HealthServer {
	return &HealthServer{}
}

func (h *HealthServer) SetReady(ready bool) {
	h.ready.Store(ready)
}

func (h *HealthServer) SetStarted(started bool) {
	h.started.Store(started)
}

func (h *HealthServer) IsReady() bool {
	return h.ready.Load()
}

func (h *HealthServer) IsStarted() bool {
	return h.started.Load()
}

func (h *HealthServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health/live", h.handleLive)
	mux.HandleFunc("/health/ready", h.handleReady)
	mux.HandleFunc("/health/startup", h.handleStartup)

	return mux
}

func (h *HealthServer) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(HealthStatus{
		Status:    "alive",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		log.Printf("health: failed to encode liveness response: %v", err)
	}
}

func (h *HealthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.ready.Load() {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(HealthStatus{
			Status:    "ready",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Printf("health: failed to encode readiness response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthStatus{
			Status:    "not ready",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Printf("health: failed to encode readiness response: %v", err)
		}
	}
}

func (h *HealthServer) handleStartup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.started.Load() {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(HealthStatus{
			Status:    "started",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Printf("health: failed to encode startup response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthStatus{
			Status:    "starting",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Printf("health: failed to encode startup response: %v", err)
		}
	}
}

func (h *HealthServer) Start(addr string) error {
	h.mu.Lock()
	h.server = &http.Server{
		Addr:              addr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	h.mu.Unlock()

	return h.server.ListenAndServe()
}

func (h *HealthServer) StartAsync(addr string) error {
	h.mu.Lock()
	h.server = &http.Server{
		Addr:              addr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	h.mu.Unlock()

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("health: server error: %v", err)
		}
	}()

	return nil
}

func (h *HealthServer) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	server := h.server
	h.mu.Unlock()

	if server != nil {
		return server.Shutdown(ctx)
	}
	return nil
}
