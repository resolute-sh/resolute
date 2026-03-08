package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const (
	defaultTemporalHost = "localhost:7233"
	defaultNamespace    = "default"
)

// WorkerConfig holds configuration for connecting to Temporal and running the worker.
type WorkerConfig struct {
	TemporalHost  string // Default: TEMPORAL_HOST env or "localhost:7233"
	TaskQueue     string // Required - no default
	Namespace     string // Default: TEMPORAL_NAMESPACE env or "default"
	MaxConcurrent int    // Default: 0 (unlimited)
}

// loadDefaults populates empty fields with environment variables or defaults.
func (c *WorkerConfig) loadDefaults() {
	if c.TemporalHost == "" {
		c.TemporalHost = getEnvOrDefault("TEMPORAL_HOST", defaultTemporalHost)
	}
	if c.Namespace == "" {
		c.Namespace = getEnvOrDefault("TEMPORAL_NAMESPACE", defaultNamespace)
	}
}

// Validate checks that required fields are set.
func (c *WorkerConfig) Validate() error {
	if c.TaskQueue == "" {
		return fmt.Errorf("TaskQueue is required")
	}
	return nil
}

// WorkerBuilder provides a fluent API for constructing and running a Temporal worker.
type WorkerBuilder struct {
	config          WorkerConfig
	flows           []*Flow
	providers       []Provider
	client          client.Client
	worker          worker.Worker
	webhookAddr     string
	webhookServer   *WebhookServer
	healthAddr      string
	healthServer    *HealthServer
	metricsExporter MetricsExporter
}

// NewWorker creates a new worker builder with environment defaults loaded.
func NewWorker() *WorkerBuilder {
	return &WorkerBuilder{
		providers: make([]Provider, 0),
	}
}

// WithConfig sets the worker configuration.
// Empty fields will be populated from environment variables or defaults.
func (b *WorkerBuilder) WithConfig(cfg WorkerConfig) *WorkerBuilder {
	b.config = cfg
	return b
}

// WithFlow adds a flow to be registered with this worker.
// Multiple flows can be registered by calling WithFlow multiple times.
func (b *WorkerBuilder) WithFlow(f *Flow) *WorkerBuilder {
	b.flows = append(b.flows, f)
	return b
}

// WithProviders adds providers whose activities will be registered with the worker.
func (b *WorkerBuilder) WithProviders(providers ...Provider) *WorkerBuilder {
	b.providers = append(b.providers, providers...)
	return b
}

// WithWebhookServer enables the webhook server on the specified address.
// If the flow has a webhook trigger, incoming webhooks will start workflow executions.
//
// Example:
//
//	worker := core.NewWorker().
//	    WithConfig(cfg).
//	    WithFlow(flow).
//	    WithWebhookServer(":8080").
//	    Run()
func (b *WorkerBuilder) WithWebhookServer(addr string) *WorkerBuilder {
	b.webhookAddr = addr
	return b
}

// WithHealthServer enables Kubernetes-compatible health endpoints on the specified address.
// Provides /health/live, /health/ready, and /health/startup endpoints.
//
// Example:
//
//	worker := core.NewWorker().
//	    WithConfig(cfg).
//	    WithFlow(flow).
//	    WithHealthServer(":8081").
//	    Run()
func (b *WorkerBuilder) WithHealthServer(addr string) *WorkerBuilder {
	b.healthAddr = addr
	return b
}

// WithMetrics enables metrics collection with the provided exporter.
// Metrics are recorded for flow executions, activity durations, errors, and rate limiting.
//
// Example:
//
//	worker := core.NewWorker().
//	    WithConfig(cfg).
//	    WithFlow(flow).
//	    WithMetrics(core.NewPrometheusExporter()).
//	    Run()
func (b *WorkerBuilder) WithMetrics(exporter MetricsExporter) *WorkerBuilder {
	b.metricsExporter = exporter
	return b
}

// Build creates the Temporal client and worker without starting them.
// This is useful for testing or custom lifecycle management.
// Safe to call multiple times — subsequent calls are no-ops.
func (b *WorkerBuilder) Build() error {
	if b.worker != nil {
		return nil
	}
	b.config.loadDefaults()

	if err := b.config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if b.metricsExporter != nil {
		SetMetricsExporter(b.metricsExporter)
	}

	if err := b.runProviderHealthChecks(); err != nil {
		return err
	}

	c, err := client.Dial(client.Options{
		HostPort:  b.config.TemporalHost,
		Namespace: b.config.Namespace,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	b.client = c

	opts := worker.Options{}
	if b.config.MaxConcurrent > 0 {
		opts.MaxConcurrentActivityExecutionSize = b.config.MaxConcurrent
	}

	b.worker = worker.New(c, b.config.TaskQueue, opts)

	for _, f := range b.flows {
		b.worker.RegisterWorkflowWithOptions(f.Execute, workflow.RegisterOptions{
			Name: f.Name(),
		})
	}

	for _, p := range b.providers {
		for _, act := range p.Activities() {
			b.worker.RegisterActivityWithOptions(act.Function, activity.RegisterOptions{
				Name: act.Name,
			})
		}
	}

	if err := b.setupSchedule(); err != nil {
		b.client.Close()
		return fmt.Errorf("setup schedule: %w", err)
	}

	if b.webhookAddr != "" && len(b.flows) > 0 {
		b.webhookServer = NewWebhookServer(WebhookServerConfig{
			Client:    b.client,
			TaskQueue: b.config.TaskQueue,
		})
		for _, f := range b.flows {
			if f.Trigger() != nil && f.Trigger().Type() == TriggerWebhook {
				if err := b.webhookServer.RegisterFlow(f); err != nil {
					return fmt.Errorf("register webhook flow %s: %w", f.Name(), err)
				}
			}
		}
	}

	if b.healthAddr != "" {
		b.healthServer = NewHealthServer()
	}

	return nil
}

// Run builds and runs the worker, blocking until interrupted.
// This is the typical entry point for a worker process.
func (b *WorkerBuilder) Run() error {
	if err := b.Build(); err != nil {
		return err
	}
	defer b.client.Close()

	log.Printf("Starting worker for task queue: %s (host: %s, namespace: %s)",
		b.config.TaskQueue, b.config.TemporalHost, b.config.Namespace)

	if b.healthServer != nil {
		log.Printf("Starting health server on %s", b.healthAddr)
		if err := b.healthServer.StartAsync(b.healthAddr); err != nil {
			return fmt.Errorf("start health server: %w", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = b.healthServer.Shutdown(ctx)
		}()
	}

	if b.webhookServer != nil {
		log.Printf("Starting webhook server on %s", b.webhookAddr)
		go func() {
			if err := b.webhookServer.Start(b.webhookAddr); err != nil {
				log.Printf("Webhook server error: %v", err)
			}
		}()
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = b.webhookServer.Shutdown(ctx)
		}()
	}

	// Mark health as ready after all servers are started
	if b.healthServer != nil {
		b.healthServer.SetReady(true)
		b.healthServer.SetStarted(true)
	}

	if err := b.worker.Run(worker.InterruptCh()); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}

	return nil
}

// RunAsync builds and starts the worker in the background.
// Returns a shutdown function that should be called to stop the worker.
func (b *WorkerBuilder) RunAsync() (shutdown func(), err error) {
	if err := b.Build(); err != nil {
		return nil, err
	}

	log.Printf("Starting worker for task queue: %s (host: %s, namespace: %s)",
		b.config.TaskQueue, b.config.TemporalHost, b.config.Namespace)

	if b.healthServer != nil {
		log.Printf("Starting health server on %s", b.healthAddr)
		if err := b.healthServer.StartAsync(b.healthAddr); err != nil {
			b.client.Close()
			return nil, fmt.Errorf("start health server: %w", err)
		}
	}

	if b.webhookServer != nil {
		log.Printf("Starting webhook server on %s", b.webhookAddr)
		if err := b.webhookServer.StartAsync(b.webhookAddr); err != nil {
			if b.healthServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = b.healthServer.Shutdown(ctx)
				cancel()
			}
			b.client.Close()
			return nil, fmt.Errorf("start webhook server: %w", err)
		}
	}

	if err := b.worker.Start(); err != nil {
		if b.webhookServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = b.webhookServer.Shutdown(ctx)
			cancel()
		}
		if b.healthServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = b.healthServer.Shutdown(ctx)
			cancel()
		}
		b.client.Close()
		return nil, fmt.Errorf("start worker: %w", err)
	}

	// Mark health as ready after worker is started
	if b.healthServer != nil {
		b.healthServer.SetReady(true)
		b.healthServer.SetStarted(true)
	}

	return func() {
		b.worker.Stop()
		if b.webhookServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = b.webhookServer.Shutdown(ctx)
			cancel()
		}
		if b.healthServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = b.healthServer.Shutdown(ctx)
			cancel()
		}
		b.client.Close()
	}, nil
}

// Client returns the underlying Temporal client after Build() has been called.
// Returns nil if Build() has not been called.
func (b *WorkerBuilder) Client() client.Client {
	return b.client
}

// Worker returns the underlying Temporal worker after Build() has been called.
// Returns nil if Build() has not been called.
func (b *WorkerBuilder) Worker() worker.Worker {
	return b.worker
}

// WebhookServer returns the webhook server if configured.
// Returns nil if webhook server is not enabled or Build() has not been called.
func (b *WorkerBuilder) WebhookServer() *WebhookServer {
	return b.webhookServer
}

// HealthServer returns the health server if configured.
// Returns nil if health server is not enabled or Build() has not been called.
func (b *WorkerBuilder) HealthServer() *HealthServer {
	return b.healthServer
}

const defaultProviderHealthCheckTimeout = 30 * time.Second

func (b *WorkerBuilder) runProviderHealthChecks() error {
	if len(b.providers) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultProviderHealthCheckTimeout)
	defer cancel()

	for _, p := range b.providers {
		if err := p.HealthCheck(ctx); err != nil {
			return fmt.Errorf("provider %s health check failed: %w", p.Name(), err)
		}
	}

	return nil
}

func (b *WorkerBuilder) setupSchedule() error {
	for _, f := range b.flows {
		if f.Trigger() == nil || f.Trigger().Type() != TriggerSchedule {
			continue
		}

		cronExpr := f.Trigger().Config().CronSchedule
		scheduleID := f.Name() + "-schedule"

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		action := &client.ScheduleWorkflowAction{
			ID:                       f.Name(),
			Workflow:                 f.Name(),
			TaskQueue:                b.config.TaskQueue,
			WorkflowExecutionTimeout: 1 * time.Hour,
		}

		_, err := b.client.ScheduleClient().Create(ctx, client.ScheduleOptions{
			ID:      scheduleID,
			Spec:    client.ScheduleSpec{CronExpressions: []string{cronExpr}},
			Action:  action,
			Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		})
		if err == nil {
			log.Printf("Created schedule %s (cron: %s)", scheduleID, cronExpr)
			cancel()
			continue
		}

		handle := b.client.ScheduleClient().GetHandle(ctx, scheduleID)
		updateErr := handle.Update(ctx, client.ScheduleUpdateOptions{
			DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
				input.Description.Schedule.Spec = &client.ScheduleSpec{
					CronExpressions: []string{cronExpr},
				}
				input.Description.Schedule.Action = action
				return &client.ScheduleUpdate{
					Schedule: &input.Description.Schedule,
				}, nil
			},
		})
		cancel()
		if updateErr != nil {
			return fmt.Errorf("create schedule: %w; update schedule: %w", err, updateErr)
		}

		log.Printf("Updated schedule %s (cron: %s)", scheduleID, cronExpr)
	}
	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
