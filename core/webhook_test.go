package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhook_DefaultMethod(t *testing.T) {
	t.Parallel()

	// given/when
	trigger := Webhook("/hooks/test")

	// then
	if trigger.Type() != TriggerWebhook {
		t.Errorf("expected type %q, got %q", TriggerWebhook, trigger.Type())
	}

	config := trigger.Config()
	if config.WebhookPath != "/hooks/test" {
		t.Errorf("expected path %q, got %q", "/hooks/test", config.WebhookPath)
	}
	if config.WebhookMethod != "POST" {
		t.Errorf("expected method %q, got %q", "POST", config.WebhookMethod)
	}
}

func TestWebhook_WithMethod(t *testing.T) {
	t.Parallel()

	// given/when
	trigger := Webhook("/hooks/test").WithMethod("PUT")

	// then
	config := trigger.Config()
	if config.WebhookMethod != "PUT" {
		t.Errorf("expected method %q, got %q", "PUT", config.WebhookMethod)
	}
}

func TestWebhook_WithSecret(t *testing.T) {
	t.Parallel()

	// given/when
	trigger := Webhook("/hooks/test").WithSecret("my-secret")

	// then
	config := trigger.Config()
	if config.WebhookSecret != "my-secret" {
		t.Errorf("expected secret %q, got %q", "my-secret", config.WebhookSecret)
	}
}

func TestWebhook_PathNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"/hooks/test", "/hooks/test"},
		{"hooks/test", "/hooks/test"},
		{"/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			trigger := Webhook(tt.input)
			config := trigger.Config()
			if config.WebhookPath != tt.expected {
				t.Errorf("expected path %q, got %q", tt.expected, config.WebhookPath)
			}
		})
	}
}

func TestWebhook_ChainedConfiguration(t *testing.T) {
	t.Parallel()

	// given/when
	trigger := Webhook("/hooks/github").
		WithMethod("POST").
		WithSecret("github-secret")

	// then
	config := trigger.Config()
	if config.WebhookPath != "/hooks/github" {
		t.Errorf("expected path %q, got %q", "/hooks/github", config.WebhookPath)
	}
	if config.WebhookMethod != "POST" {
		t.Errorf("expected method %q, got %q", "POST", config.WebhookMethod)
	}
	if config.WebhookSecret != "github-secret" {
		t.Errorf("expected secret %q, got %q", "github-secret", config.WebhookSecret)
	}
}

func TestComputeWebhookSignature(t *testing.T) {
	t.Parallel()

	// given
	payload := []byte(`{"event":"push"}`)
	secret := "test-secret"

	// when
	signature := ComputeWebhookSignature(payload, secret)

	// then
	if signature == "" {
		t.Error("expected non-empty signature")
	}
	if len(signature) < 10 {
		t.Errorf("signature seems too short: %q", signature)
	}
	if signature[:7] != "sha256=" {
		t.Errorf("expected signature to start with 'sha256=', got %q", signature)
	}
}

func TestComputeWebhookSignature_Consistency(t *testing.T) {
	t.Parallel()

	// given
	payload := []byte(`{"event":"push"}`)
	secret := "test-secret"

	// when
	sig1 := ComputeWebhookSignature(payload, secret)
	sig2 := ComputeWebhookSignature(payload, secret)

	// then
	if sig1 != sig2 {
		t.Errorf("signatures should be consistent: %q != %q", sig1, sig2)
	}
}

func TestComputeWebhookSignature_DifferentSecrets(t *testing.T) {
	t.Parallel()

	// given
	payload := []byte(`{"event":"push"}`)

	// when
	sig1 := ComputeWebhookSignature(payload, "secret1")
	sig2 := ComputeWebhookSignature(payload, "secret2")

	// then
	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestVerifySignature(t *testing.T) {
	t.Parallel()

	// given
	payload := []byte(`{"event":"push"}`)
	secret := "test-secret"
	validSig := ComputeWebhookSignature(payload, secret)

	tests := []struct {
		name      string
		signature string
		valid     bool
	}{
		{"valid signature", validSig, true},
		{"empty signature", "", false},
		{"wrong signature", "sha256=invalid", false},
		{"tampered payload signature", ComputeWebhookSignature([]byte("tampered"), secret), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifySignature(payload, tt.signature, secret)
			if result != tt.valid {
				t.Errorf("expected %v, got %v", tt.valid, result)
			}
		})
	}
}

func TestGetWebhookPayload(t *testing.T) {
	t.Parallel()

	// given
	payload := []byte(`{"event":"push"}`)
	input := FlowInput{
		Data: map[string][]byte{
			"webhook_payload": payload,
		},
	}

	// when
	result := GetWebhookPayload(input)

	// then
	if !bytes.Equal(result, payload) {
		t.Errorf("expected %q, got %q", payload, result)
	}
}

func TestGetWebhookPayload_NilData(t *testing.T) {
	t.Parallel()

	// given
	input := FlowInput{}

	// when
	result := GetWebhookPayload(input)

	// then
	if result != nil {
		t.Errorf("expected nil, got %q", result)
	}
}

func TestGetWebhookHeaders(t *testing.T) {
	t.Parallel()

	// given
	headers := http.Header{
		"Content-Type": []string{"application/json"},
		"X-Custom":     []string{"value"},
	}
	headersJSON, _ := json.Marshal(headers)
	input := FlowInput{
		Data: map[string][]byte{
			"webhook_headers": headersJSON,
		},
	}

	// when
	result := GetWebhookHeaders(input)

	// then
	if result.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type header, got %v", result)
	}
	if result.Get("X-Custom") != "value" {
		t.Errorf("expected X-Custom header, got %v", result)
	}
}

func TestParseWebhookPayload(t *testing.T) {
	t.Parallel()

	// given
	type TestPayload struct {
		Event string `json:"event"`
		ID    int    `json:"id"`
	}
	payload := []byte(`{"event":"push","id":123}`)
	input := FlowInput{
		Data: map[string][]byte{
			"webhook_payload": payload,
		},
	}

	// when
	result, err := ParseWebhookPayload[TestPayload](input)

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Event != "push" {
		t.Errorf("expected event %q, got %q", "push", result.Event)
	}
	if result.ID != 123 {
		t.Errorf("expected id %d, got %d", 123, result.ID)
	}
}

func TestParseWebhookPayload_NoPayload(t *testing.T) {
	t.Parallel()

	// given
	type TestPayload struct{}
	input := FlowInput{}

	// when
	_, err := ParseWebhookPayload[TestPayload](input)

	// then
	if err == nil {
		t.Error("expected error for missing payload")
	}
}

func TestWebhookServer_RegisterFlow_NilFlow(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})

	// when
	err := server.RegisterFlow(nil)

	// then
	if err == nil {
		t.Error("expected error for nil flow")
	}
}

func TestWebhookServer_RegisterFlow_NoWebhookTrigger(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow := &Flow{
		name:    "test-flow",
		trigger: Manual("test"),
	}

	// when
	err := server.RegisterFlow(flow)

	// then
	if err == nil {
		t.Error("expected error for non-webhook trigger")
	}
}

func TestWebhookServer_Routes(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow := &Flow{
		name:    "test-flow",
		trigger: Webhook("/hooks/test"),
		steps:   []Step{},
	}

	// when
	_ = server.RegisterFlow(flow)
	routes := server.Routes()

	// then
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0] != "/hooks/test" {
		t.Errorf("expected route %q, got %q", "/hooks/test", routes[0])
	}
}

func TestWebhookServer_DuplicatePath(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow1 := &Flow{
		name:    "flow1",
		trigger: Webhook("/hooks/test"),
		steps:   []Step{},
	}
	flow2 := &Flow{
		name:    "flow2",
		trigger: Webhook("/hooks/test"),
		steps:   []Step{},
	}

	// when
	err1 := server.RegisterFlow(flow1)
	err2 := server.RegisterFlow(flow2)

	// then
	if err1 != nil {
		t.Fatalf("unexpected error: %v", err1)
	}
	if err2 == nil {
		t.Error("expected error for duplicate path")
	}
}

func TestWebhookServer_Handler_NotFound(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})

	// when
	req := httptest.NewRequest("POST", "/unknown", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	// then
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestWebhookServer_Handler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow := &Flow{
		name:    "test-flow",
		trigger: Webhook("/hooks/test").WithMethod("POST"),
		steps:   []Step{},
	}
	_ = server.RegisterFlow(flow)

	// when
	req := httptest.NewRequest("GET", "/hooks/test", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	// then
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestWebhookServer_Handler_InvalidSignature(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow := &Flow{
		name:    "test-flow",
		trigger: Webhook("/hooks/test").WithSecret("secret"),
		steps:   []Step{},
	}
	_ = server.RegisterFlow(flow)

	// when
	req := httptest.NewRequest("POST", "/hooks/test", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(WebhookSignatureHeader, "invalid")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	// then
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestWebhookServer_Handler_MissingSignature(t *testing.T) {
	t.Parallel()

	// given
	server := NewWebhookServer(WebhookServerConfig{})
	flow := &Flow{
		name:    "test-flow",
		trigger: Webhook("/hooks/test").WithSecret("secret"),
		steps:   []Step{},
	}
	_ = server.RegisterFlow(flow)

	// when
	req := httptest.NewRequest("POST", "/hooks/test", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	// then
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestWebhookTrigger_Build(t *testing.T) {
	t.Parallel()

	// given
	builder := Webhook("/test")

	// when
	trigger := builder.Build()

	// then
	if trigger.Type() != TriggerWebhook {
		t.Errorf("expected type %q, got %q", TriggerWebhook, trigger.Type())
	}
}
