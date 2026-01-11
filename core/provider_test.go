package core

import (
	"context"
	"testing"
)

func sampleActivity(ctx context.Context, input string) (string, error) {
	return "result: " + input, nil
}

func TestNewProviderRegistry(t *testing.T) {
	t.Parallel()

	// when
	registry := NewProviderRegistry()

	// then
	if registry == nil {
		t.Fatal("NewProviderRegistry returned nil")
	}

	if registry.providers == nil {
		t.Error("providers map is nil")
	}
}

func TestProviderRegistry_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		provider  Provider
		wantErr   bool
		errSubstr string
	}{
		{
			name: "registers valid provider",
			provider: &BaseProvider{
				ProviderName:    "test-provider",
				ProviderVersion: "1.0.0",
			},
			wantErr: false,
		},
		{
			name:      "rejects nil provider",
			provider:  nil,
			wantErr:   true,
			errSubstr: "cannot be nil",
		},
		{
			name: "rejects empty name",
			provider: &BaseProvider{
				ProviderName:    "",
				ProviderVersion: "1.0.0",
			},
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			registry := NewProviderRegistry()

			// when
			err := registry.Register(tt.provider)

			// then
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestProviderRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()

	// given
	registry := NewProviderRegistry()
	provider := &BaseProvider{
		ProviderName:    "test-provider",
		ProviderVersion: "1.0.0",
	}

	// when
	err1 := registry.Register(provider)
	err2 := registry.Register(provider)

	// then
	if err1 != nil {
		t.Errorf("first registration failed: %v", err1)
	}

	if err2 == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestProviderRegistry_Get(t *testing.T) {
	t.Parallel()

	// given
	registry := NewProviderRegistry()
	provider := &BaseProvider{
		ProviderName:    "test-provider",
		ProviderVersion: "1.0.0",
	}
	_ = registry.Register(provider)

	// when
	got, ok := registry.Get("test-provider")

	// then
	if !ok {
		t.Error("provider not found")
	}

	if got.Name() != "test-provider" {
		t.Errorf("got name %q, want %q", got.Name(), "test-provider")
	}
}

func TestProviderRegistry_GetNotFound(t *testing.T) {
	t.Parallel()

	// given
	registry := NewProviderRegistry()

	// when
	_, ok := registry.Get("nonexistent")

	// then
	if ok {
		t.Error("expected false for nonexistent provider")
	}
}

func TestProviderRegistry_List(t *testing.T) {
	t.Parallel()

	// given
	registry := NewProviderRegistry()
	_ = registry.Register(&BaseProvider{ProviderName: "provider-a", ProviderVersion: "1.0.0"})
	_ = registry.Register(&BaseProvider{ProviderName: "provider-b", ProviderVersion: "2.0.0"})

	// when
	providers := registry.List()

	// then
	if len(providers) != 2 {
		t.Errorf("got %d providers, want 2", len(providers))
	}

	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name()] = true
	}

	if !names["provider-a"] {
		t.Error("provider-a not found in list")
	}

	if !names["provider-b"] {
		t.Error("provider-b not found in list")
	}
}

func TestBaseProvider(t *testing.T) {
	t.Parallel()

	// given
	provider := NewProvider("my-provider", "1.2.3")

	// then
	if provider.Name() != "my-provider" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "my-provider")
	}

	if provider.Version() != "1.2.3" {
		t.Errorf("Version() = %q, want %q", provider.Version(), "1.2.3")
	}

	if len(provider.Activities()) != 0 {
		t.Errorf("Activities() = %d, want 0", len(provider.Activities()))
	}
}

func TestBaseProvider_AddActivity(t *testing.T) {
	t.Parallel()

	// given
	provider := NewProvider("my-provider", "1.0.0")

	// when
	provider.AddActivity("my-provider.DoSomething", sampleActivity)

	// then
	activities := provider.Activities()
	if len(activities) != 1 {
		t.Fatalf("got %d activities, want 1", len(activities))
	}

	if activities[0].Name != "my-provider.DoSomething" {
		t.Errorf("activity name = %q, want %q", activities[0].Name, "my-provider.DoSomething")
	}

	if activities[0].Function == nil {
		t.Error("activity function is nil")
	}
}

func TestBaseProvider_AddActivityWithDescription(t *testing.T) {
	t.Parallel()

	// given
	provider := NewProvider("my-provider", "1.0.0")

	// when
	provider.AddActivityWithDescription(
		"my-provider.DoSomething",
		"Does something important",
		sampleActivity,
	)

	// then
	activities := provider.Activities()
	if len(activities) != 1 {
		t.Fatalf("got %d activities, want 1", len(activities))
	}

	if activities[0].Description != "Does something important" {
		t.Errorf("description = %q, want %q", activities[0].Description, "Does something important")
	}
}

func TestBaseProvider_ChainedAddActivity(t *testing.T) {
	t.Parallel()

	// given/when
	provider := NewProvider("my-provider", "1.0.0").
		AddActivity("activity-1", sampleActivity).
		AddActivity("activity-2", sampleActivity).
		AddActivity("activity-3", sampleActivity)

	// then
	if len(provider.Activities()) != 3 {
		t.Errorf("got %d activities, want 3", len(provider.Activities()))
	}
}

func TestValidateActivityFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fn      ActivityFunc
		wantErr bool
	}{
		{
			name:    "valid function",
			fn:      sampleActivity,
			wantErr: false,
		},
		{
			name:    "nil function",
			fn:      nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// when
			err := ValidateActivityFunc(tt.fn)

			// then
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBaseProvider_HealthCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		healthFn HealthCheckFunc
		wantErr bool
	}{
		{
			name:     "no health check configured returns nil",
			healthFn: nil,
			wantErr:  false,
		},
		{
			name: "healthy provider returns nil",
			healthFn: func(ctx context.Context) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "unhealthy provider returns error",
			healthFn: func(ctx context.Context) error {
				return context.DeadlineExceeded
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			provider := NewProvider("test-provider", "1.0.0")
			if tt.healthFn != nil {
				provider.WithHealthCheck(tt.healthFn)
			}

			// when
			err := provider.HealthCheck(context.Background())

			// then
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBaseProvider_WithHealthCheck(t *testing.T) {
	t.Parallel()

	// given
	called := false
	healthFn := func(ctx context.Context) error {
		called = true
		return nil
	}

	// when
	provider := NewProvider("test-provider", "1.0.0").WithHealthCheck(healthFn)
	_ = provider.HealthCheck(context.Background())

	// then
	if !called {
		t.Error("health check function was not called")
	}
}

func TestBaseProvider_HealthCheckRespectsContext(t *testing.T) {
	t.Parallel()

	// given
	provider := NewProvider("test-provider", "1.0.0").
		WithHealthCheck(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		})

	// when - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := provider.HealthCheck(ctx)

	// then
	if err == nil {
		t.Error("expected context cancellation error")
	}
}
