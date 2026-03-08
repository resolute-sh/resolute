package core

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	outputMarkerPrefix = "{{output:"
	outputMarkerSuffix = "}}"
	cursorMarkerPrefix = "{{cursor:"
	cursorMarkerSuffix = "}}"
	inputMarkerPrefix  = "{{input:"
	inputMarkerSuffix  = "}}"
)

// cursorMarkerID is used to generate unique IDs for cursor markers.
var cursorMarkerID atomic.Uint64

// cursorMarkerRegistry maps marker IDs to their source names.
var cursorMarkerRegistry sync.Map

// CursorMarker is a special time value that acts as a placeholder for cursor resolution.
// The marker contains a unique ID that maps to the cursor source.
type CursorMarker struct {
	id     uint64
	source string
}

// CursorFor creates a magic marker that resolves to the persisted cursor for a source.
// The framework replaces this with the actual cursor value at execution time.
//
// Example:
//
//	jira.FetchIssues(jira.Input{
//	    Project: "PLATFORM",
//	    Since:   core.CursorFor("jira"),
//	})
func CursorFor(source string) *time.Time {
	id := cursorMarkerID.Add(1)
	marker := &CursorMarker{id: id, source: source}
	cursorMarkerRegistry.Store(id, marker)

	t := time.Unix(0, int64(id))
	return &t
}

// Output creates a magic marker that resolves to a previous node's output.
// The framework replaces this with the actual value at execution time.
//
// Example:
//
//	gcp.CreateSubnet(gcp.SubnetInput{
//	    VPC: core.Output("vpc.name"),
//	})
func Output(path string) string {
	return outputMarkerPrefix + path + outputMarkerSuffix
}

// CursorString creates a string marker for cursor reference (for string fields).
func CursorString(source string) string {
	return cursorMarkerPrefix + source + cursorMarkerSuffix
}

// InputData creates a magic marker that resolves to a value from the flow's initial input.
// Webhook-triggered flows store payload data in FlowInput; this marker provides
// access from activity input structs.
//
// Example:
//
//	bitbucket.ParseWebhook(bitbucket.ParseWebhookInput{
//	    RawPayload: core.InputData("webhook_payload"),
//	})
func InputData(key string) string {
	return inputMarkerPrefix + key + inputMarkerSuffix
}

const (
	dataRefMarkerBackend = "__marker__"
)

// OutputRef creates a DataRef marker that resolves to a previous node's DataRef output.
// The framework replaces this with the actual DataRef at execution time.
//
// Example:
//
//	ollama.BatchEmbed(ollama.BatchEmbedInput{
//	    DocumentsRef: core.OutputRef("jira_docs"),
//	})
func OutputRef(nodeKey string) DataRef {
	return DataRef{
		StorageKey: nodeKey,
		Backend:    dataRefMarkerBackend,
	}
}

// IsOutputRefMarker returns true if the DataRef is a marker for output resolution.
func IsOutputRefMarker(ref DataRef) bool {
	return ref.Backend == dataRefMarkerBackend
}

// WindowOutput creates a DataRef marker that resolves to the current windowed batch's output.
// In windowed execution, the framework stores each batch's output under the "__window__" key.
func WindowOutput() DataRef {
	return OutputRef("__window__")
}

// resolve processes an input struct and replaces magic markers with actual values.
func resolve[I any](input I, state *FlowState) (I, error) {
	val := reflect.ValueOf(&input).Elem()
	if err := resolveValue(val, state); err != nil {
		return input, err
	}
	return input, nil
}

// resolveValue recursively resolves magic markers in a reflect.Value.
func resolveValue(val reflect.Value, state *FlowState) error {
	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		if val.Type() == reflect.TypeOf((*time.Time)(nil)) {
			return resolveCursorPointer(val, state)
		}
		return resolveValue(val.Elem(), state)

	case reflect.Struct:
		if val.Type() == reflect.TypeOf(time.Time{}) {
			return resolveCursorValue(val, state)
		}
		if val.Type() == reflect.TypeOf(DataRef{}) {
			return resolveDataRefValue(val, state)
		}
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.CanSet() {
				if err := resolveValue(field, state); err != nil {
					return err
				}
			}
		}

	case reflect.String:
		if val.CanSet() {
			resolved, err := resolveStringMarkers(val.String(), state)
			if err != nil {
				return err
			}
			val.SetString(resolved)
		}

	case reflect.Slice:
		for i := 0; i < val.Len(); i++ {
			if err := resolveValue(val.Index(i), state); err != nil {
				return err
			}
		}

	case reflect.Map:
		if val.IsNil() {
			return nil
		}
		iter := val.MapRange()
		updates := make(map[reflect.Value]reflect.Value)
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()
			if v.Kind() == reflect.String {
				resolved, err := resolveStringMarkers(v.String(), state)
				if err != nil {
					return err
				}
				if resolved != v.String() {
					updates[k] = reflect.ValueOf(resolved)
				}
			}
		}
		for k, v := range updates {
			val.SetMapIndex(k, v)
		}
	}

	return nil
}

// resolveCursorPointer resolves a *time.Time cursor marker.
func resolveCursorPointer(val reflect.Value, state *FlowState) error {
	if val.IsNil() {
		return nil
	}

	timePtr := val.Interface().(*time.Time)
	if timePtr == nil {
		return nil
	}

	markerID := uint64(timePtr.UnixNano())
	markerVal, ok := cursorMarkerRegistry.Load(markerID)
	if !ok {
		return nil
	}

	marker := markerVal.(*CursorMarker)
	cursor := state.GetCursor(marker.source)

	if cursor.Position == "" {
		val.Set(reflect.Zero(val.Type()))
		return nil
	}

	t := cursor.TimeOr(time.Time{})
	if t.IsZero() {
		val.Set(reflect.Zero(val.Type()))
		return nil
	}

	val.Set(reflect.ValueOf(&t))
	return nil
}

// resolveDataRefValue resolves a DataRef marker to the actual DataRef from FlowState.
func resolveDataRefValue(val reflect.Value, state *FlowState) error {
	if !val.CanSet() {
		return nil
	}

	ref := val.Interface().(DataRef)
	if !IsOutputRefMarker(ref) {
		return nil
	}

	nodeKey := ref.StorageKey
	result := state.GetResult(nodeKey)
	if result == nil {
		return fmt.Errorf("no result for node %q", nodeKey)
	}

	actualRef, err := extractDataRef(result)
	if err != nil {
		return fmt.Errorf("extract DataRef from %q: %w", nodeKey, err)
	}

	val.Set(reflect.ValueOf(actualRef))
	return nil
}

// extractDataRef extracts a DataRef from a result value.
// It handles both direct DataRef values and structs with a Ref field.
func extractDataRef(result interface{}) (DataRef, error) {
	if ref, ok := result.(DataRef); ok {
		return ref, nil
	}

	val := reflect.ValueOf(result)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return DataRef{}, fmt.Errorf("expected DataRef or struct with Ref field, got %T", result)
	}

	refField := val.FieldByName("Ref")
	if !refField.IsValid() {
		return DataRef{}, fmt.Errorf("struct %T has no Ref field", result)
	}

	ref, ok := refField.Interface().(DataRef)
	if !ok {
		return DataRef{}, fmt.Errorf("Ref field is not DataRef, got %T", refField.Interface())
	}

	return ref, nil
}

// resolveCursorValue resolves a time.Time cursor marker (non-pointer).
func resolveCursorValue(val reflect.Value, state *FlowState) error {
	if !val.CanSet() {
		return nil
	}

	timeVal := val.Interface().(time.Time)
	markerID := uint64(timeVal.UnixNano())
	markerVal, ok := cursorMarkerRegistry.Load(markerID)
	if !ok {
		return nil
	}

	marker := markerVal.(*CursorMarker)
	cursor := state.GetCursor(marker.source)

	if cursor.Position == "" {
		val.Set(reflect.Zero(val.Type()))
		return nil
	}

	t := cursor.TimeOr(time.Time{})
	val.Set(reflect.ValueOf(t))
	return nil
}

// ResolveStringRef resolves a string that may contain output/input/cursor markers.
func ResolveStringRef(s string, state *FlowState) (string, error) {
	return resolveStringMarkers(s, state)
}

// resolveStringMarkers resolves output, cursor, and input string markers.
func resolveStringMarkers(s string, state *FlowState) (string, error) {
	if strings.HasPrefix(s, outputMarkerPrefix) && strings.HasSuffix(s, outputMarkerSuffix) {
		path := s[len(outputMarkerPrefix) : len(s)-len(outputMarkerSuffix)]
		resolved, err := resolveOutputPath(path, state)
		if err != nil {
			return "", fmt.Errorf("resolve output %q: %w", path, err)
		}
		return resolved, nil
	}

	if strings.HasPrefix(s, cursorMarkerPrefix) && strings.HasSuffix(s, cursorMarkerSuffix) {
		source := s[len(cursorMarkerPrefix) : len(s)-len(cursorMarkerSuffix)]
		cursor := state.GetCursor(source)
		return cursor.Position, nil
	}

	if strings.HasPrefix(s, inputMarkerPrefix) && strings.HasSuffix(s, inputMarkerSuffix) {
		key := s[len(inputMarkerPrefix) : len(s)-len(inputMarkerSuffix)]
		data, ok := state.GetInputData(key)
		if !ok {
			return "", fmt.Errorf("no input data for key %q", key)
		}
		return string(data), nil
	}

	return s, nil
}

// resolveOutputPath resolves a path like "vpc" or "vpc.name" to the actual value.
func resolveOutputPath(path string, state *FlowState) (string, error) {
	parts := strings.SplitN(path, ".", 2)
	nodeKey := parts[0]

	result := state.GetResult(nodeKey)
	if result == nil {
		return "", fmt.Errorf("no result for node %q", nodeKey)
	}

	if len(parts) == 1 {
		return formatValue(result), nil
	}

	fieldPath := parts[1]
	val := reflect.ValueOf(result)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return "", fmt.Errorf("cannot access field %q on non-struct type", fieldPath)
	}

	field := val.FieldByName(fieldPath)
	if !field.IsValid() {
		field = val.FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, fieldPath)
		})
	}

	if !field.IsValid() {
		return "", fmt.Errorf("field %q not found in %T", fieldPath, result)
	}

	return formatValue(field.Interface()), nil
}

// formatValue converts a value to string for output resolution.
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ResolveCursorRef looks up a cursor value from the flow state.
func ResolveCursorRef(source string, state *FlowState) *time.Time {
	cursor := state.GetCursor(source)
	if cursor.Position == "" {
		return nil
	}
	t := cursor.TimeOr(time.Time{})
	if t.IsZero() {
		return nil
	}
	return &t
}

// ResolveOutputRef looks up a previous node's output from the flow state.
func ResolveOutputRef(path string, state *FlowState) (string, bool) {
	result := state.GetResult(path)
	if result == nil {
		return "", false
	}
	if s, ok := result.(string); ok {
		return s, true
	}
	return "", false
}
