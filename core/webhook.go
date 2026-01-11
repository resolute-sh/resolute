package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
)

const (
	// TriggerWebhook indicates the flow is started via HTTP webhook.
	TriggerWebhook TriggerType = "webhook"

	// DefaultWebhookMethod is the default HTTP method for webhooks.
	DefaultWebhookMethod = "POST"

	// WebhookSignatureHeader is the header containing the HMAC signature.
	WebhookSignatureHeader = "X-Webhook-Signature"
)

// webhookTrigger implements Trigger for HTTP webhook-initiated flows.
type webhookTrigger struct {
	path   string
	method string
	secret string
}

func (t *webhookTrigger) Type() TriggerType {
	return TriggerWebhook
}

func (t *webhookTrigger) Config() TriggerConfig {
	return TriggerConfig{
		WebhookPath:   t.path,
		WebhookMethod: t.method,
		WebhookSecret: t.secret,
	}
}

// WebhookBuilder provides a fluent API for configuring webhook triggers.
type WebhookBuilder struct {
	trigger *webhookTrigger
}

// Webhook creates a trigger for HTTP webhook-initiated flow execution.
// The path should start with "/" and will be the endpoint that receives webhook requests.
//
// Example:
//
//	flow := core.NewFlow("github-handler").
//	    TriggeredBy(core.Webhook("/hooks/github").WithMethod("POST")).
//	    Then(processWebhookNode).
//	    Build()
func Webhook(path string) *WebhookBuilder {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &WebhookBuilder{
		trigger: &webhookTrigger{
			path:   path,
			method: DefaultWebhookMethod,
		},
	}
}

// WithMethod sets the HTTP method for the webhook (default: POST).
func (b *WebhookBuilder) WithMethod(method string) *WebhookBuilder {
	b.trigger.method = strings.ToUpper(method)
	return b
}

// WithSecret sets the HMAC secret for webhook signature verification.
// When set, incoming webhooks must include a valid X-Webhook-Signature header.
func (b *WebhookBuilder) WithSecret(secret string) *WebhookBuilder {
	b.trigger.secret = secret
	return b
}

// Build returns the configured trigger.
// This is called implicitly when passed to TriggeredBy.
func (b *WebhookBuilder) Build() Trigger {
	return b.trigger
}

// Type implements Trigger interface for WebhookBuilder.
func (b *WebhookBuilder) Type() TriggerType {
	return b.trigger.Type()
}

// Config implements Trigger interface for WebhookBuilder.
func (b *WebhookBuilder) Config() TriggerConfig {
	return b.trigger.Config()
}

// WebhookServer handles incoming webhook requests and starts Temporal workflows.
type WebhookServer struct {
	mu        sync.RWMutex
	flows     map[string]*webhookRoute
	client    client.Client
	taskQueue string
	server    *http.Server
	mux       *http.ServeMux
}

type webhookRoute struct {
	flow   *Flow
	method string
	secret string
}

// WebhookServerConfig holds configuration for the webhook server.
type WebhookServerConfig struct {
	Client    client.Client
	TaskQueue string
}

// NewWebhookServer creates a new webhook server.
func NewWebhookServer(cfg WebhookServerConfig) *WebhookServer {
	s := &WebhookServer{
		flows:     make(map[string]*webhookRoute),
		client:    cfg.Client,
		taskQueue: cfg.TaskQueue,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("/", s.handleWebhook)
	return s
}

// RegisterFlow registers a flow with a webhook trigger.
// Returns an error if the flow doesn't have a webhook trigger or if the path is already registered.
func (s *WebhookServer) RegisterFlow(flow *Flow) error {
	if flow == nil {
		return fmt.Errorf("flow cannot be nil")
	}

	trigger := flow.Trigger()
	if trigger == nil || trigger.Type() != TriggerWebhook {
		return fmt.Errorf("flow %q does not have a webhook trigger", flow.Name())
	}

	config := trigger.Config()
	path := config.WebhookPath

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.flows[path]; exists {
		return fmt.Errorf("webhook path %q is already registered", path)
	}

	s.flows[path] = &webhookRoute{
		flow:   flow,
		method: config.WebhookMethod,
		secret: config.WebhookSecret,
	}

	return nil
}

// handleWebhook processes incoming webhook requests.
func (s *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	s.mu.RLock()
	route, exists := s.flows[path]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}

	if r.Method != route.method {
		http.Error(w, fmt.Sprintf("method not allowed, expected %s", route.method), http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if route.secret != "" {
		signature := r.Header.Get(WebhookSignatureHeader)
		if !verifySignature(body, signature, route.secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	workflowID := fmt.Sprintf("%s-%s", route.flow.Name(), uuid.New().String())

	input := FlowInput{
		Data: map[string][]byte{
			"webhook_payload": body,
			"webhook_headers": mustMarshalHeaders(r.Header),
			"webhook_path":    []byte(path),
			"webhook_method":  []byte(r.Method),
		},
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}

	run, err := s.client.ExecuteWorkflow(r.Context(), workflowOptions, route.flow.Execute, input)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to start workflow: %v", err), http.StatusInternalServerError)
		return
	}

	response := WebhookResponse{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
		Status:     "started",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("webhook: failed to encode response: %v", err)
	}
}

// WebhookResponse is returned to webhook callers.
type WebhookResponse struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
}

// Start starts the webhook server on the given address.
func (s *WebhookServer) Start(addr string) error {
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s.server.ListenAndServe()
}

// StartAsync starts the webhook server in the background.
// Returns immediately after the server starts listening.
func (s *WebhookServer) StartAsync(addr string) error {
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// Shutdown gracefully shuts down the webhook server.
func (s *WebhookServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Handler returns the HTTP handler for use with custom servers.
func (s *WebhookServer) Handler() http.Handler {
	return s.mux
}

// Routes returns the registered webhook paths.
func (s *WebhookServer) Routes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := make([]string, 0, len(s.flows))
	for path := range s.flows {
		paths = append(paths, path)
	}
	return paths
}

// verifySignature checks the HMAC-SHA256 signature of the payload.
func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	expectedSig := computeSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// computeSignature computes the HMAC-SHA256 signature for a payload.
func computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// ComputeWebhookSignature computes a signature for testing or external webhook senders.
func ComputeWebhookSignature(payload []byte, secret string) string {
	return computeSignature(payload, secret)
}

func mustMarshalHeaders(headers http.Header) []byte {
	data, _ := json.Marshal(headers)
	return data
}

// GetWebhookPayload extracts the webhook payload from FlowInput.
func GetWebhookPayload(input FlowInput) []byte {
	if input.Data == nil {
		return nil
	}
	return input.Data["webhook_payload"]
}

// GetWebhookHeaders extracts the webhook headers from FlowInput.
func GetWebhookHeaders(input FlowInput) http.Header {
	if input.Data == nil {
		return nil
	}
	data := input.Data["webhook_headers"]
	if data == nil {
		return nil
	}
	var headers http.Header
	if err := json.Unmarshal(data, &headers); err != nil {
		return nil
	}
	return headers
}

// ParseWebhookPayload parses the webhook payload into a typed struct.
func ParseWebhookPayload[T any](input FlowInput) (T, error) {
	var result T
	payload := GetWebhookPayload(input)
	if payload == nil {
		return result, fmt.Errorf("no webhook payload in input")
	}
	err := json.Unmarshal(payload, &result)
	return result, err
}
