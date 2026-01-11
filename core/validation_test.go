package core

import (
	"errors"
	"testing"
)

func TestValidate_Required(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "required string present",
			input: struct {
				Name string `validate:"required"`
			}{Name: "test"},
			wantErr: false,
		},
		{
			name: "required string missing",
			input: struct {
				Name string `validate:"required"`
			}{Name: ""},
			wantErr: true,
		},
		{
			name: "required int zero",
			input: struct {
				Count int `validate:"required"`
			}{Count: 0},
			wantErr: true,
		},
		{
			name: "required int non-zero",
			input: struct {
				Count int `validate:"required"`
			}{Count: 5},
			wantErr: false,
		},
		{
			name: "required pointer nil",
			input: struct {
				Ref *string `validate:"required"`
			}{Ref: nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_MinMax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "min satisfied",
			input: struct {
				Age int `validate:"min=18"`
			}{Age: 20},
			wantErr: false,
		},
		{
			name: "min not satisfied",
			input: struct {
				Age int `validate:"min=18"`
			}{Age: 15},
			wantErr: true,
		},
		{
			name: "max satisfied",
			input: struct {
				Count int `validate:"max=100"`
			}{Count: 50},
			wantErr: false,
		},
		{
			name: "max not satisfied",
			input: struct {
				Count int `validate:"max=100"`
			}{Count: 150},
			wantErr: true,
		},
		{
			name: "min and max satisfied",
			input: struct {
				Value int `validate:"min=1,max=10"`
			}{Value: 5},
			wantErr: false,
		},
		{
			name: "min and max - below min",
			input: struct {
				Value int `validate:"min=1,max=10"`
			}{Value: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_Length(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "minlen satisfied",
			input: struct {
				Name string `validate:"minlen=3"`
			}{Name: "abc"},
			wantErr: false,
		},
		{
			name: "minlen not satisfied",
			input: struct {
				Name string `validate:"minlen=3"`
			}{Name: "ab"},
			wantErr: true,
		},
		{
			name: "maxlen satisfied",
			input: struct {
				Code string `validate:"maxlen=5"`
			}{Code: "abc"},
			wantErr: false,
		},
		{
			name: "maxlen not satisfied",
			input: struct {
				Code string `validate:"maxlen=5"`
			}{Code: "abcdef"},
			wantErr: true,
		},
		{
			name: "slice minlen satisfied",
			input: struct {
				Items []string `validate:"minlen=2"`
			}{Items: []string{"a", "b", "c"}},
			wantErr: false,
		},
		{
			name: "slice minlen not satisfied",
			input: struct {
				Items []string `validate:"minlen=2"`
			}{Items: []string{"a"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_OneOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "oneof satisfied",
			input: struct {
				Status string `validate:"oneof=active|inactive|pending"`
			}{Status: "active"},
			wantErr: false,
		},
		{
			name: "oneof not satisfied",
			input: struct {
				Status string `validate:"oneof=active|inactive|pending"`
			}{Status: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_NestedStruct(t *testing.T) {
	t.Parallel()

	type Address struct {
		City string `validate:"required"`
	}
	type Person struct {
		Name    string  `validate:"required"`
		Address Address
	}

	tests := []struct {
		name    string
		input   Person
		wantErr bool
	}{
		{
			name: "nested valid",
			input: Person{
				Name:    "John",
				Address: Address{City: "NYC"},
			},
			wantErr: false,
		},
		{
			name: "nested invalid - missing city",
			input: Person{
				Name:    "John",
				Address: Address{City: ""},
			},
			wantErr: true,
		},
		{
			name: "nested invalid - missing name",
			input: Person{
				Name:    "",
				Address: Address{City: "NYC"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_NilInput(t *testing.T) {
	t.Parallel()

	err := Validate(nil)
	if err != nil {
		t.Errorf("Validate(nil) should return nil, got %v", err)
	}
}

func TestValidate_NonStruct(t *testing.T) {
	t.Parallel()

	err := Validate("string value")
	if err != nil {
		t.Errorf("Validate(string) should return nil, got %v", err)
	}
}

func TestValidationErrors_Error(t *testing.T) {
	t.Parallel()

	errs := ValidationErrors{
		{Field: "Name", Tag: "required", Message: "is required"},
		{Field: "Age", Tag: "min=0", Message: "must be >= 0"},
	}

	errStr := errs.Error()
	if errStr == "" {
		t.Error("ValidationErrors.Error() should not be empty")
	}

	var validationErrs ValidationErrors
	if !errors.As(errs, &validationErrs) {
		t.Error("should be able to use errors.As with ValidationErrors")
	}
}
