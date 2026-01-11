package core

// Trigger defines how a flow is initiated.
type Trigger interface {
	// Type returns the trigger type identifier.
	Type() TriggerType

	// Config returns the trigger-specific configuration.
	Config() TriggerConfig
}

// TriggerType identifies the type of trigger.
type TriggerType string

const (
	// TriggerManual indicates the flow is started via API call.
	TriggerManual TriggerType = "manual"

	// TriggerSchedule indicates the flow runs on a cron schedule.
	TriggerSchedule TriggerType = "schedule"

	// TriggerSignal indicates the flow starts from a Temporal signal.
	TriggerSignal TriggerType = "signal"
)

// TriggerConfig holds trigger-specific configuration.
type TriggerConfig struct {
	// ID is the identifier for manual triggers (API endpoint path).
	ID string

	// CronSchedule is the cron expression for scheduled triggers.
	CronSchedule string

	// SignalName is the Temporal signal name for signal triggers.
	SignalName string

	// WebhookPath is the HTTP path for webhook triggers.
	WebhookPath string

	// WebhookMethod is the HTTP method for webhook triggers (default: POST).
	WebhookMethod string

	// WebhookSecret is the HMAC secret for webhook signature verification.
	WebhookSecret string
}

// manualTrigger implements Trigger for API-initiated flows.
type manualTrigger struct {
	id string
}

func (t *manualTrigger) Type() TriggerType {
	return TriggerManual
}

func (t *manualTrigger) Config() TriggerConfig {
	return TriggerConfig{ID: t.id}
}

// Manual creates a trigger for API-initiated flow execution.
//
// Example:
//
//	flow := core.NewFlow("my-flow").
//	    TriggeredBy(core.Manual("api-trigger")).
//	    Then(myNode).
//	    Build()
func Manual(id string) Trigger {
	return &manualTrigger{id: id}
}

// scheduleTrigger implements Trigger for cron-scheduled flows.
type scheduleTrigger struct {
	cron string
}

func (t *scheduleTrigger) Type() TriggerType {
	return TriggerSchedule
}

func (t *scheduleTrigger) Config() TriggerConfig {
	return TriggerConfig{CronSchedule: t.cron}
}

// Schedule creates a trigger for cron-scheduled flow execution.
// Uses standard cron expression format (minute hour day month weekday).
//
// Example:
//
//	flow := core.NewFlow("daily-sync").
//	    TriggeredBy(core.Schedule("0 2 * * *")).  // Daily at 2 AM
//	    Then(syncNode).
//	    Build()
func Schedule(cron string) Trigger {
	return &scheduleTrigger{cron: cron}
}

// signalTrigger implements Trigger for signal-initiated flows.
type signalTrigger struct {
	name string
}

func (t *signalTrigger) Type() TriggerType {
	return TriggerSignal
}

func (t *signalTrigger) Config() TriggerConfig {
	return TriggerConfig{SignalName: t.name}
}

// Signal creates a trigger that starts the flow from a Temporal signal.
//
// Example:
//
//	flow := core.NewFlow("event-handler").
//	    TriggeredBy(core.Signal("new-event")).
//	    Then(handleEventNode).
//	    Build()
func Signal(name string) Trigger {
	return &signalTrigger{name: name}
}
