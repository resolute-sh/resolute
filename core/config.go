package core

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

// ConfigError represents a configuration loading error.
type ConfigError struct {
	Field   string
	EnvVar  string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config %s (%s): %s", e.Field, e.EnvVar, e.Message)
}

// LoadConfig loads configuration from environment variables into a struct.
// The prefix is prepended to all environment variable names with an underscore.
//
// Supported struct tags:
//   - env:"VAR_NAME" - the environment variable suffix (prefix_VAR_NAME)
//   - required:"true" - fail if the variable is not set
//   - default:"value" - use this value if the variable is not set
//
// Supported field types: string, int, int64, bool, float64, time.Duration
//
// Example:
//
//	type Config struct {
//	    BaseURL string        `env:"BASE_URL" required:"true"`
//	    Token   string        `env:"TOKEN" required:"true"`
//	    Timeout time.Duration `env:"TIMEOUT" default:"30s"`
//	    Retries int           `env:"RETRIES" default:"3"`
//	}
//
//	cfg, err := LoadConfig[Config]("JIRA")
//	// Reads JIRA_BASE_URL, JIRA_TOKEN, JIRA_TIMEOUT, JIRA_RETRIES
func LoadConfig[T any](prefix string) (T, error) {
	var cfg T
	val := reflect.ValueOf(&cfg).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		envKey := field.Tag.Get("env")
		if envKey == "" {
			continue
		}

		fullKey := prefix + "_" + envKey
		envValue := os.Getenv(fullKey)

		if envValue == "" {
			if field.Tag.Get("required") == "true" {
				return cfg, &ConfigError{
					Field:   field.Name,
					EnvVar:  fullKey,
					Message: "required but not set",
				}
			}

			defaultVal := field.Tag.Get("default")
			if defaultVal == "" {
				continue
			}
			envValue = defaultVal
		}

		if err := setConfigField(fieldVal, envValue); err != nil {
			return cfg, &ConfigError{
				Field:   field.Name,
				EnvVar:  fullKey,
				Message: err.Error(),
			}
		}
	}

	return cfg, nil
}

func setConfigField(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration: %w", err)
			}
			field.SetInt(int64(d))
			return nil
		}

		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int: %w", err)
		}
		field.SetInt(i)

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool: %w", err)
		}
		field.SetBool(b)

	case reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float: %w", err)
		}
		field.SetFloat(f)

	default:
		return fmt.Errorf("unsupported type: %s", field.Kind())
	}
	return nil
}

// MustLoadConfig loads configuration and panics if there's an error.
// Use this in main() or init() where you want to fail fast.
func MustLoadConfig[T any](prefix string) T {
	cfg, err := LoadConfig[T](prefix)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}
