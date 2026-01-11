package core

import (
	"testing"
	"time"
)

type testConfig struct {
	BaseURL    string        `env:"BASE_URL" required:"true"`
	Token      string        `env:"TOKEN" required:"true"`
	MaxRetries int           `env:"MAX_RETRIES" default:"3"`
	Timeout    time.Duration `env:"TIMEOUT" default:"30s"`
	Enabled    bool          `env:"ENABLED" default:"true"`
	Rate       float64       `env:"RATE" default:"1.5"`
}

type optionalConfig struct {
	Host    string `env:"HOST"`
	Port    int    `env:"PORT" default:"8080"`
	Verbose bool   `env:"VERBOSE"`
}

func TestLoadConfig_AllFieldsSet(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")
	t.Setenv("TEST_MAX_RETRIES", "5")
	t.Setenv("TEST_TIMEOUT", "60s")
	t.Setenv("TEST_ENABLED", "false")
	t.Setenv("TEST_RATE", "2.5")

	// when
	cfg, err := LoadConfig[testConfig]("TEST")

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
	if cfg.Token != "secret-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "secret-token")
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 5)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 60*time.Second)
	}
	if cfg.Enabled != false {
		t.Errorf("Enabled = %v, want %v", cfg.Enabled, false)
	}
	if cfg.Rate != 2.5 {
		t.Errorf("Rate = %v, want %v", cfg.Rate, 2.5)
	}
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")

	// when
	cfg, err := LoadConfig[testConfig]("TEST")

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want default %d", cfg.MaxRetries, 3)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want default %v", cfg.Timeout, 30*time.Second)
	}
	if cfg.Enabled != true {
		t.Errorf("Enabled = %v, want default %v", cfg.Enabled, true)
	}
	if cfg.Rate != 1.5 {
		t.Errorf("Rate = %v, want default %v", cfg.Rate, 1.5)
	}
}

func TestLoadConfig_RequiredFieldMissing(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")

	// when
	_, err := LoadConfig[testConfig]("TEST")

	// then
	if err == nil {
		t.Fatal("expected error for missing required field")
	}

	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}

	if configErr.Field != "Token" {
		t.Errorf("Field = %q, want %q", configErr.Field, "Token")
	}
	if configErr.EnvVar != "TEST_TOKEN" {
		t.Errorf("EnvVar = %q, want %q", configErr.EnvVar, "TEST_TOKEN")
	}
}

func TestLoadConfig_InvalidInt(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")
	t.Setenv("TEST_MAX_RETRIES", "not-a-number")

	// when
	_, err := LoadConfig[testConfig]("TEST")

	// then
	if err == nil {
		t.Fatal("expected error for invalid int")
	}

	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}

	if configErr.Field != "MaxRetries" {
		t.Errorf("Field = %q, want %q", configErr.Field, "MaxRetries")
	}
}

func TestLoadConfig_InvalidBool(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")
	t.Setenv("TEST_ENABLED", "not-a-bool")

	// when
	_, err := LoadConfig[testConfig]("TEST")

	// then
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestLoadConfig_InvalidDuration(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")
	t.Setenv("TEST_TIMEOUT", "not-a-duration")

	// when
	_, err := LoadConfig[testConfig]("TEST")

	// then
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoadConfig_InvalidFloat(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://api.example.com")
	t.Setenv("TEST_TOKEN", "secret-token")
	t.Setenv("TEST_RATE", "not-a-float")

	// when
	_, err := LoadConfig[testConfig]("TEST")

	// then
	if err == nil {
		t.Fatal("expected error for invalid float")
	}
}

func TestLoadConfig_OptionalFields(t *testing.T) {
	// given - no environment variables set

	// when
	cfg, err := LoadConfig[optionalConfig]("OPT")

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Host != "" {
		t.Errorf("Host = %q, want empty string", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want default %d", cfg.Port, 8080)
	}
	if cfg.Verbose != false {
		t.Errorf("Verbose = %v, want false", cfg.Verbose)
	}
}

func TestLoadConfig_DifferentPrefixes(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://jira.example.com")
	t.Setenv("JIRA_TOKEN", "jira-token")
	t.Setenv("CONFLUENCE_BASE_URL", "https://confluence.example.com")
	t.Setenv("CONFLUENCE_TOKEN", "confluence-token")

	// when
	jiraCfg, err := LoadConfig[testConfig]("JIRA")
	if err != nil {
		t.Fatalf("jira config error: %v", err)
	}

	confluenceCfg, err := LoadConfig[testConfig]("CONFLUENCE")
	if err != nil {
		t.Fatalf("confluence config error: %v", err)
	}

	// then
	if jiraCfg.BaseURL != "https://jira.example.com" {
		t.Errorf("jira BaseURL = %q, want %q", jiraCfg.BaseURL, "https://jira.example.com")
	}
	if confluenceCfg.BaseURL != "https://confluence.example.com" {
		t.Errorf("confluence BaseURL = %q, want %q", confluenceCfg.BaseURL, "https://confluence.example.com")
	}
}

func TestConfigError_Error(t *testing.T) {
	// given
	err := &ConfigError{
		Field:   "Token",
		EnvVar:  "JIRA_TOKEN",
		Message: "required but not set",
	}

	// when
	msg := err.Error()

	// then
	expected := "config Token (JIRA_TOKEN): required but not set"
	if msg != expected {
		t.Errorf("Error() = %q, want %q", msg, expected)
	}
}

func TestMustLoadConfig_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing required field")
		}
	}()

	// when - missing required field
	_ = MustLoadConfig[testConfig]("MISSING")
}

func TestMustLoadConfig_Success(t *testing.T) {
	t.Setenv("SUCCESS_BASE_URL", "https://api.example.com")
	t.Setenv("SUCCESS_TOKEN", "token")

	// when
	cfg := MustLoadConfig[testConfig]("SUCCESS")

	// then
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
}
