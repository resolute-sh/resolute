package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucket_InitialBurst(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(10, time.Second)

	// when - acquire 10 tokens immediately (initial burst)
	for i := 0; i < 10; i++ {
		if !limiter.TryAcquire() {
			t.Fatalf("expected to acquire token %d", i)
		}
	}

	// then - 11th should fail
	if limiter.TryAcquire() {
		t.Error("expected 11th acquire to fail")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(10, time.Second)

	// when - exhaust all tokens
	for i := 0; i < 10; i++ {
		limiter.TryAcquire()
	}

	// then - wait for refill
	time.Sleep(150 * time.Millisecond)

	// should have ~1.5 tokens refilled
	if !limiter.TryAcquire() {
		t.Error("expected token to be available after refill")
	}
}

func TestTokenBucket_Wait(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(10, time.Second)

	// exhaust tokens
	for i := 0; i < 10; i++ {
		limiter.TryAcquire()
	}

	// when - wait should block until token available
	ctx := context.Background()
	start := time.Now()

	err := limiter.Wait(ctx)

	elapsed := time.Since(start)

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// should have waited ~100ms for 1 token at 10/sec rate
	if elapsed < 50*time.Millisecond {
		t.Errorf("expected to wait for token, but only waited %v", elapsed)
	}
}

func TestTokenBucket_WaitContextCancelled(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(1, time.Minute) // very slow refill
	limiter.TryAcquire()                      // exhaust the one token

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// when
	err := limiter.Wait(ctx)

	// then
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(100, time.Second)

	var acquired int64
	var wg sync.WaitGroup

	// when - 50 goroutines each try to acquire 10 tokens
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if limiter.TryAcquire() {
					atomic.AddInt64(&acquired, 1)
				}
			}
		}()
	}

	wg.Wait()

	// then - should have acquired exactly 100 (initial capacity)
	if acquired != 100 {
		t.Errorf("expected 100 acquires, got %d", acquired)
	}
}

func TestTokenBucket_Tokens(t *testing.T) {
	t.Parallel()

	// given
	limiter := NewTokenBucket(10, time.Second)

	// when/then - initial tokens (allow small variance due to timing)
	tokens := limiter.Tokens()
	if tokens < 9.9 || tokens > 10.1 {
		t.Errorf("expected ~10 tokens, got %f", tokens)
	}

	// when/then - after acquiring
	limiter.TryAcquire()
	tokens = limiter.Tokens()
	if tokens < 8.9 || tokens > 9.1 {
		t.Errorf("expected ~9 tokens after acquire, got %f", tokens)
	}
}

func TestSharedRateLimiter_Creation(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given/when
	limiter := NewSharedRateLimiter("test-limiter", 100, time.Minute)

	// then
	if limiter.ID() == "" {
		t.Error("expected non-empty ID")
	}

	// should be registered
	retrieved := getRateLimiter(limiter.ID())
	if retrieved == nil {
		t.Error("expected limiter to be registered")
	}
}

func TestSharedRateLimiter_Close(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	limiter := NewSharedRateLimiter("test-limiter", 100, time.Minute)
	id := limiter.ID()

	// when
	limiter.Close()

	// then
	if getRateLimiter(id) != nil {
		t.Error("expected limiter to be unregistered after Close")
	}
}

func TestNode_WithRateLimit(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	node := NewNode("test-node", func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, nil
	}, struct{}{})

	// when
	node.WithRateLimit(100, time.Minute)

	// then
	if node.RateLimiterID() == "" {
		t.Error("expected rate limiter ID to be set")
	}

	// limiter should be registered
	if getRateLimiter(node.RateLimiterID()) == nil {
		t.Error("expected limiter to be registered")
	}
}

func TestNode_WithSharedRateLimit(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	limiter := NewSharedRateLimiter("shared", 100, time.Minute)

	node1 := NewNode("node1", func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, nil
	}, struct{}{})

	node2 := NewNode("node2", func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, nil
	}, struct{}{})

	// when
	node1.WithSharedRateLimit(limiter)
	node2.WithSharedRateLimit(limiter)

	// then - both nodes should share the same limiter ID
	if node1.RateLimiterID() != node2.RateLimiterID() {
		t.Error("expected nodes to share the same rate limiter ID")
	}

	if node1.RateLimiterID() != limiter.ID() {
		t.Error("expected node to use the shared limiter ID")
	}
}

type rlTestInput struct {
	Value string
}

type rlTestOutput struct {
	Result string
}

func TestFlowTester_RateLimitingDisabledByDefault(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	node := NewNode("rate-limited", func(ctx context.Context, in rlTestInput) (rlTestOutput, error) {
		return rlTestOutput{}, nil
	}, rlTestInput{}).WithRateLimit(1, time.Hour) // very restrictive

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockValue("rate-limited", rlTestOutput{Result: "ok"})

	// when - run multiple times quickly (should not be rate limited)
	start := time.Now()
	for i := 0; i < 5; i++ {
		_, err := tester.Run(flow, FlowInput{})
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// then - should complete quickly (no rate limiting)
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fast execution without rate limiting, took %v", elapsed)
	}
}

func TestFlowTester_WithRateLimiting(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given - use a very restrictive rate limit (1 per second)
	node := NewNode("rate-limited", func(ctx context.Context, in rlTestInput) (rlTestOutput, error) {
		return rlTestOutput{}, nil
	}, rlTestInput{}).WithRateLimit(1, time.Second)

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		WithRateLimiting().
		MockValue("rate-limited", rlTestOutput{Result: "ok"})

	// when - first call uses the initial token
	_, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}

	// second call should wait for token refill
	start := time.Now()
	_, err = tester.Run(flow, FlowInput{})
	elapsed := time.Since(start)

	// then
	if err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}

	// should have waited for rate limit (at least 500ms for 1/sec rate)
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected to wait for rate limit (~1s), only waited %v", elapsed)
	}
}

func TestFlowTester_RateLimitContextCancellation(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	node := NewNode("rate-limited", func(ctx context.Context, in rlTestInput) (rlTestOutput, error) {
		return rlTestOutput{}, nil
	}, rlTestInput{}).WithRateLimit(1, time.Hour) // very restrictive

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		WithRateLimiting().
		MockValue("rate-limited", rlTestOutput{Result: "ok"})

	// exhaust the one token
	_, _ = tester.Run(flow, FlowInput{})

	// when - run with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tester.RunWithContext(ctx, flow, FlowInput{})

	// then
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestBaseProvider_WithRateLimit(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	provider := NewProvider("test-provider", "1.0.0")

	// when
	provider.WithRateLimit(100, time.Minute)

	// then
	if provider.GetRateLimiter() == nil {
		t.Error("expected rate limiter to be set")
	}

	if provider.GetRateLimiter().ID() == "" {
		t.Error("expected rate limiter to have an ID")
	}
}

func TestRateLimitWaitActivity(t *testing.T) {
	t.Parallel()
	defer clearRateLimiterRegistry()

	// given
	limiter := NewSharedRateLimiter("test", 10, time.Second)
	id := limiter.ID()

	// when - call the activity
	err := RateLimitWaitActivity(context.Background(), id)

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// should have consumed a token
	tokens := getRateLimiter(id).Tokens()
	if tokens >= 10 {
		t.Errorf("expected tokens to decrease, got %f", tokens)
	}
}

func TestRateLimitWaitActivity_UnknownLimiter(t *testing.T) {
	t.Parallel()

	// given - unknown limiter ID

	// when
	err := RateLimitWaitActivity(context.Background(), "unknown-limiter")

	// then - should succeed (no-op)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
