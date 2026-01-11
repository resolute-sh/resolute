package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
)

// Provider represents a collection of related activities.
// Providers are the building blocks of resolute workflows, each exposing
// a set of activities that can be used in flow definitions.
type Provider interface {
	Name() string
	Version() string
	Activities() []ActivityMeta
	HealthCheck(ctx context.Context) error
}

// ActivityMeta describes a single activity for registration and discovery.
type ActivityMeta struct {
	Name        string
	Description string
	Function    ActivityFunc
}

// ActivityFunc is the function signature for all activity implementations.
// Using a concrete type instead of `any` for type safety.
type ActivityFunc interface{}

// ValidateActivityFunc checks if a function has a valid activity signature.
// Valid signatures are: func(context.Context, I) (O, error)
func ValidateActivityFunc(fn ActivityFunc) error {
	if fn == nil {
		return fmt.Errorf("activity function cannot be nil")
	}
	return nil
}

// ProviderRegistry tracks available providers and their activities.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
// Returns an error if a provider with the same name is already registered.
func (r *ProviderRegistry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	name := p.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}

	r.providers[name] = p
	return nil
}

// Get returns a provider by name.
func (r *ProviderRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	return p, ok
}

// List returns all registered providers.
func (r *ProviderRegistry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// RegisterActivities registers all activities from a provider with a Temporal worker.
func (r *ProviderRegistry) RegisterActivities(w worker.Worker, providerName string) error {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("provider %q not found", providerName)
	}

	for _, act := range p.Activities() {
		w.RegisterActivityWithOptions(act.Function, activity.RegisterOptions{
			Name: act.Name,
		})
	}

	return nil
}

// RegisterAllActivities registers all activities from all providers with a Temporal worker.
func (r *ProviderRegistry) RegisterAllActivities(w worker.Worker) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.providers {
		for _, act := range p.Activities() {
			w.RegisterActivityWithOptions(act.Function, activity.RegisterOptions{
				Name: act.Name,
			})
		}
	}
}

// HealthCheckFunc is the signature for provider health check functions.
type HealthCheckFunc func(ctx context.Context) error

// BaseProvider provides a default implementation of the Provider interface.
// Embed this in provider implementations to reduce boilerplate.
type BaseProvider struct {
	ProviderName    string
	ProviderVersion string
	ActivityList    []ActivityMeta
	RateLimiter     *SharedRateLimiter
	healthFn        HealthCheckFunc
}

// Name returns the provider name.
func (p *BaseProvider) Name() string {
	return p.ProviderName
}

// Version returns the provider version.
func (p *BaseProvider) Version() string {
	return p.ProviderVersion
}

// Activities returns the list of activities.
func (p *BaseProvider) Activities() []ActivityMeta {
	return p.ActivityList
}

// HealthCheck verifies provider connectivity and configuration.
// Returns nil if healthy, or an error describing the failure.
func (p *BaseProvider) HealthCheck(ctx context.Context) error {
	if p.healthFn != nil {
		return p.healthFn(ctx)
	}
	return nil
}

// WithHealthCheck configures a health check function for the provider.
// The health check is called during worker startup to validate configuration.
//
// Example:
//
//	provider := core.NewProvider("jira", "1.0.0").
//	    WithHealthCheck(func(ctx context.Context) error {
//	        _, err := client.GetServerInfo(ctx)
//	        return err
//	    })
func (p *BaseProvider) WithHealthCheck(fn HealthCheckFunc) *BaseProvider {
	p.healthFn = fn
	return p
}

// NewProvider creates a new base provider with the given name and version.
func NewProvider(name, version string) *BaseProvider {
	return &BaseProvider{
		ProviderName:    name,
		ProviderVersion: version,
		ActivityList:    make([]ActivityMeta, 0),
	}
}

// AddActivity adds an activity to the provider.
func (p *BaseProvider) AddActivity(name string, fn ActivityFunc) *BaseProvider {
	p.ActivityList = append(p.ActivityList, ActivityMeta{
		Name:     name,
		Function: fn,
	})
	return p
}

// AddActivityWithDescription adds an activity with a description.
func (p *BaseProvider) AddActivityWithDescription(name, description string, fn ActivityFunc) *BaseProvider {
	p.ActivityList = append(p.ActivityList, ActivityMeta{
		Name:        name,
		Description: description,
		Function:    fn,
	})
	return p
}

// WithRateLimit configures a shared rate limiter for all activities in this provider.
// All activities registered through this provider will share the same rate limit,
// which is useful for preventing overwhelming external APIs.
//
// Example:
//
//	jiraProvider := jira.NewProvider().WithRateLimit(100, time.Minute)
func (p *BaseProvider) WithRateLimit(requests int, per time.Duration) *BaseProvider {
	p.RateLimiter = NewSharedRateLimiter(p.ProviderName, requests, per)
	return p
}

// GetRateLimiter returns the provider's rate limiter, if configured.
func (p *BaseProvider) GetRateLimiter() *SharedRateLimiter {
	return p.RateLimiter
}

// RegisterActivitiesFunc is a helper type for provider packages that want to
// provide a simple RegisterActivities function without the full Provider interface.
type RegisterActivitiesFunc func(w worker.Worker)

// ActivityRegistrar is implemented by types that can register their activities.
type ActivityRegistrar interface {
	RegisterActivities(w worker.Worker)
}

// RegisterProviderActivities is a helper function to register activities from a provider
// directly with a Temporal worker without using the registry.
func RegisterProviderActivities(w worker.Worker, p Provider) {
	for _, act := range p.Activities() {
		w.RegisterActivityWithOptions(act.Function, activity.RegisterOptions{
			Name: act.Name,
		})
	}
}

// ExampleActivity is a sample activity function signature for documentation.
// All activities must follow this pattern: func(context.Context, Input) (Output, error)
func ExampleActivity(ctx context.Context, input struct{}) (struct{}, error) {
	return struct{}{}, nil
}
