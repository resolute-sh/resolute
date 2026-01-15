package core

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ValidationError represents a single field validation failure.
type ValidationError struct {
	Field   string
	Tag     string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation failures.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate checks struct fields against validate tags.
// Supported tags: required, min=N, max=N, minlen=N, maxlen=N, oneof=a|b|c
func Validate(v interface{}) error {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ValidationErrors{{Field: "input", Tag: "required", Message: "input is nil"}}
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	var errs ValidationErrors
	validateStruct(val, "", &errs)

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateStruct(val reflect.Value, prefix string, errs *ValidationErrors) {
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		value := val.Field(i)

		if !field.IsExported() {
			continue
		}

		fieldName := field.Name
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		tag := field.Tag.Get("validate")
		if tag != "" && tag != "-" {
			for _, rule := range strings.Split(tag, ",") {
				if err := validateRule(fieldName, value, strings.TrimSpace(rule)); err != nil {
					*errs = append(*errs, *err)
				}
			}
		}

		if value.Kind() == reflect.Struct {
			validateStruct(value, fieldName, errs)
		}
		if value.Kind() == reflect.Ptr && !value.IsNil() && value.Elem().Kind() == reflect.Struct {
			validateStruct(value.Elem(), fieldName, errs)
		}
	}
}

func validateRule(fieldName string, value reflect.Value, rule string) *ValidationError {
	parts := strings.SplitN(rule, "=", 2)
	ruleName := parts[0]
	var ruleValue string
	if len(parts) > 1 {
		ruleValue = parts[1]
	}

	switch ruleName {
	case "required":
		if isZero(value) {
			return &ValidationError{Field: fieldName, Tag: rule, Message: "is required"}
		}

	case "min":
		minVal, _ := strconv.ParseInt(ruleValue, 10, 64)
		switch value.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if value.Int() < minVal {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be >= %d", minVal)}
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if value.Uint() < uint64(minVal) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be >= %d", minVal)}
			}
		case reflect.Float32, reflect.Float64:
			if value.Float() < float64(minVal) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be >= %d", minVal)}
			}
		}

	case "max":
		maxVal, _ := strconv.ParseInt(ruleValue, 10, 64)
		switch value.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if value.Int() > maxVal {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be <= %d", maxVal)}
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if value.Uint() > uint64(maxVal) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be <= %d", maxVal)}
			}
		case reflect.Float32, reflect.Float64:
			if value.Float() > float64(maxVal) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be <= %d", maxVal)}
			}
		}

	case "minlen":
		minLen, _ := strconv.Atoi(ruleValue)
		switch value.Kind() {
		case reflect.String:
			if len(value.String()) < minLen {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("length must be >= %d", minLen)}
			}
		case reflect.Slice, reflect.Array, reflect.Map:
			if value.Len() < minLen {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("length must be >= %d", minLen)}
			}
		}

	case "maxlen":
		maxLen, _ := strconv.Atoi(ruleValue)
		switch value.Kind() {
		case reflect.String:
			if len(value.String()) > maxLen {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("length must be <= %d", maxLen)}
			}
		case reflect.Slice, reflect.Array, reflect.Map:
			if value.Len() > maxLen {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("length must be <= %d", maxLen)}
			}
		}

	case "oneof":
		options := strings.Split(ruleValue, "|")
		if value.Kind() == reflect.String {
			strVal := value.String()
			found := false
			for _, opt := range options {
				if strVal == opt {
					found = true
					break
				}
			}
			if !found {
				return &ValidationError{Field: fieldName, Tag: rule, Message: fmt.Sprintf("must be one of: %s", ruleValue)}
			}
		}

	case "email":
		if value.Kind() == reflect.String {
			emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
			if !emailRegex.MatchString(value.String()) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: "must be a valid email address"}
			}
		}

	case "url":
		if value.Kind() == reflect.String {
			urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
			if !urlRegex.MatchString(value.String()) {
				return &ValidationError{Field: fieldName, Tag: rule, Message: "must be a valid URL"}
			}
		}
	}

	return nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
