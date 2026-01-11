package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/sdk/workflow"
)

// RateLimiter controls the rate of operations.
type RateLimiter interface {
	Wait(ctx context.Context) error
	TryAcquire() bool
}

// TokenBucket implements a token bucket rate limiter.
// It allows bursts up to the bucket capacity while maintaining
// an average rate over time.
type TokenBucket struct {
	mu         sync.Mutex
	rate       float64
	capacity   float64
	tokens     float64
	lastUpdate time.Time
}

// RateLimitConfig holds rate limit configuration.
type RateLimitConfig struct {
	Requests int
	Per      time.Duration
}

// NewTokenBucket creates a new token bucket rate limiter.
// requests is the number of allowed requests per duration.
// The bucket starts full, allowing an initial burst.
func NewTokenBucket(requests int, per time.Duration) *TokenBucket {
	rate := float64(requests) / per.Seconds()
	return &TokenBucket{
		rate:       rate,
		capacity:   float64(requests),
		tokens:     float64(requests),
		lastUpdate: time.Now(),
	}
}

// Wait blocks until a token is available or the context is cancelled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		tb.refill()

		if tb.tokens >= 1 {
			tb.tokens--
			tb.mu.Unlock()
			return nil
		}

		waitTime := time.Duration((1 - tb.tokens) / tb.rate * float64(time.Second))
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
// Returns true if a token was acquired, false otherwise.
func (tb *TokenBucket) TryAcquire() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// refill adds tokens based on elapsed time.
// Must be called with lock held.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate)
	tb.lastUpdate = now

	tb.tokens += tb.rate * elapsed.Seconds()
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
}

// Tokens returns the current number of available tokens.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// rateLimiterRegistry stores rate limiters by ID for lookup during workflow execution.
var (
	rateLimiterRegistry   = make(map[string]*TokenBucket)
	rateLimiterRegistryMu sync.RWMutex
)

// registerRateLimiter registers a rate limiter with the global registry.
func registerRateLimiter(id string, limiter *TokenBucket) {
	rateLimiterRegistryMu.Lock()
	defer rateLimiterRegistryMu.Unlock()
	rateLimiterRegistry[id] = limiter
}

// getRateLimiter retrieves a rate limiter from the global registry.
func getRateLimiter(id string) *TokenBucket {
	rateLimiterRegistryMu.RLock()
	defer rateLimiterRegistryMu.RUnlock()
	return rateLimiterRegistry[id]
}

// unregisterRateLimiter removes a rate limiter from the registry.
func unregisterRateLimiter(id string) {
	rateLimiterRegistryMu.Lock()
	defer rateLimiterRegistryMu.Unlock()
	delete(rateLimiterRegistry, id)
}

// clearRateLimiterRegistry removes all rate limiters (for testing).
func clearRateLimiterRegistry() {
	rateLimiterRegistryMu.Lock()
	defer rateLimiterRegistryMu.Unlock()
	rateLimiterRegistry = make(map[string]*TokenBucket)
}

// RateLimitWaitActivity is a local activity that waits on a rate limiter.
// This is used to rate limit workflow activities without violating determinism.
func RateLimitWaitActivity(ctx context.Context, limiterID string) error {
	limiter := getRateLimiter(limiterID)
	if limiter == nil {
		return nil
	}
	return limiter.Wait(ctx)
}

// executeWithRateLimit wraps activity execution with rate limiting.
// It uses a local activity to wait on the rate limiter before executing.
func executeWithRateLimit(ctx workflow.Context, limiterID string) error {
	if limiterID == "" {
		return nil
	}

	localCtx := workflow.WithLocalActivityOptions(ctx, workflow.LocalActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	})

	return workflow.ExecuteLocalActivity(localCtx, RateLimitWaitActivity, limiterID).Get(ctx, nil)
}

// rateLimiterIDCounter generates unique IDs for rate limiters.
var (
	rateLimiterIDCounter   uint64
	rateLimiterIDCounterMu sync.Mutex
)

// generateRateLimiterID creates a unique ID for a rate limiter.
func generateRateLimiterID(prefix string) string {
	rateLimiterIDCounterMu.Lock()
	defer rateLimiterIDCounterMu.Unlock()
	rateLimiterIDCounter++
	return fmt.Sprintf("%s-%d", prefix, rateLimiterIDCounter)
}

// SharedRateLimiter represents a rate limiter that can be shared across multiple nodes.
type SharedRateLimiter struct {
	id      string
	limiter *TokenBucket
}

// NewSharedRateLimiter creates a new shared rate limiter.
func NewSharedRateLimiter(name string, requests int, per time.Duration) *SharedRateLimiter {
	id := generateRateLimiterID(name)
	limiter := NewTokenBucket(requests, per)
	registerRateLimiter(id, limiter)

	return &SharedRateLimiter{
		id:      id,
		limiter: limiter,
	}
}

// ID returns the rate limiter's unique identifier.
func (s *SharedRateLimiter) ID() string {
	return s.id
}

// Close unregisters the rate limiter from the global registry.
func (s *SharedRateLimiter) Close() {
	unregisterRateLimiter(s.id)
}
