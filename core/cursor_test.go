package core

import (
	"testing"
	"time"
)

func TestOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "simple node reference",
			path: "vpc",
			want: "{{output:vpc}}",
		},
		{
			name: "field reference",
			path: "vpc.name",
			want: "{{output:vpc.name}}",
		},
		{
			name: "nested field reference",
			path: "cluster.config.endpoint",
			want: "{{output:cluster.config.endpoint}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// when
			got := Output(tt.path)

			// then
			if got != tt.want {
				t.Errorf("Output(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCursorFor(t *testing.T) {
	t.Parallel()

	// given
	source := "jira"

	// when
	cursor := CursorFor(source)

	// then
	if cursor == nil {
		t.Fatal("CursorFor returned nil")
	}

	markerID := uint64(cursor.UnixNano())
	markerVal, ok := cursorMarkerRegistry.Load(markerID)
	if !ok {
		t.Error("cursor marker not found in registry")
	}

	marker := markerVal.(*CursorMarker)
	if marker.source != source {
		t.Errorf("marker source = %q, want %q", marker.source, source)
	}
}

func TestCursorString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "jira cursor",
			source: "jira",
			want:   "{{cursor:jira}}",
		},
		{
			name:   "confluence cursor",
			source: "confluence",
			want:   "{{cursor:confluence}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// when
			got := CursorString(tt.source)

			// then
			if got != tt.want {
				t.Errorf("CursorString(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestResolveOutputMarker(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: map[string]any{
			"vpc": "my-vpc-id",
		},
	}

	type Input struct {
		VPCID string
	}

	input := Input{
		VPCID: Output("vpc"),
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.VPCID != "my-vpc-id" {
		t.Errorf("VPCID = %q, want %q", resolved.VPCID, "my-vpc-id")
	}
}

func TestResolveOutputMarkerWithField(t *testing.T) {
	t.Parallel()

	// given
	type VPCResult struct {
		ID   string
		Name string
		CIDR string
	}

	state := &FlowState{
		results: map[string]any{
			"vpc": VPCResult{
				ID:   "vpc-123",
				Name: "my-vpc",
				CIDR: "10.0.0.0/16",
			},
		},
	}

	type Input struct {
		VPCName string
		VPCCIDR string
	}

	input := Input{
		VPCName: Output("vpc.Name"),
		VPCCIDR: Output("vpc.CIDR"),
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.VPCName != "my-vpc" {
		t.Errorf("VPCName = %q, want %q", resolved.VPCName, "my-vpc")
	}

	if resolved.VPCCIDR != "10.0.0.0/16" {
		t.Errorf("VPCCIDR = %q, want %q", resolved.VPCCIDR, "10.0.0.0/16")
	}
}

func TestResolveCursorPointerMarker(t *testing.T) {
	t.Parallel()

	// given
	cursorTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	state := &FlowState{
		cursors: map[string]Cursor{
			"jira": {
				Source:   "jira",
				Position: cursorTime.Format(time.RFC3339),
			},
		},
	}

	type Input struct {
		Since *time.Time
	}

	input := Input{
		Since: CursorFor("jira"),
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Since == nil {
		t.Fatal("Since is nil after resolution")
	}

	if !resolved.Since.Equal(cursorTime) {
		t.Errorf("Since = %v, want %v", resolved.Since, cursorTime)
	}
}

func TestResolveCursorPointerMarkerWithNoCursor(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		cursors: make(map[string]Cursor),
	}

	type Input struct {
		Since *time.Time
	}

	input := Input{
		Since: CursorFor("jira"),
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Since != nil {
		t.Errorf("Since should be nil when no cursor exists, got %v", resolved.Since)
	}
}

func TestResolveCursorStringMarker(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		cursors: map[string]Cursor{
			"jira": {
				Source:   "jira",
				Position: "2024-01-15T10:30:00Z",
			},
		},
	}

	type Input struct {
		LastSync string
	}

	input := Input{
		LastSync: CursorString("jira"),
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.LastSync != "2024-01-15T10:30:00Z" {
		t.Errorf("LastSync = %q, want %q", resolved.LastSync, "2024-01-15T10:30:00Z")
	}
}

func TestResolveNestedStruct(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: map[string]any{
			"config": "production",
		},
	}

	type Config struct {
		Env string
	}

	type Input struct {
		Name   string
		Config Config
	}

	input := Input{
		Name: "test",
		Config: Config{
			Env: Output("config"),
		},
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Config.Env != "production" {
		t.Errorf("Config.Env = %q, want %q", resolved.Config.Env, "production")
	}
}

func TestResolveSlice(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: map[string]any{
			"region1": "us-west-2",
			"region2": "us-east-1",
		},
	}

	type Input struct {
		Regions []string
	}

	input := Input{
		Regions: []string{Output("region1"), Output("region2")},
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(resolved.Regions) != 2 {
		t.Fatalf("got %d regions, want 2", len(resolved.Regions))
	}

	if resolved.Regions[0] != "us-west-2" {
		t.Errorf("Regions[0] = %q, want %q", resolved.Regions[0], "us-west-2")
	}

	if resolved.Regions[1] != "us-east-1" {
		t.Errorf("Regions[1] = %q, want %q", resolved.Regions[1], "us-east-1")
	}
}

func TestResolveMap(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: map[string]any{
			"env": "production",
		},
	}

	type Input struct {
		Labels map[string]string
	}

	input := Input{
		Labels: map[string]string{
			"environment": Output("env"),
			"team":        "platform",
		},
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Labels["environment"] != "production" {
		t.Errorf("Labels[environment] = %q, want %q", resolved.Labels["environment"], "production")
	}

	if resolved.Labels["team"] != "platform" {
		t.Errorf("Labels[team] = %q, want %q", resolved.Labels["team"], "platform")
	}
}

func TestResolveOutputMissingNode(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: make(map[string]any),
	}

	type Input struct {
		Value string
	}

	input := Input{
		Value: Output("nonexistent"),
	}

	// when
	_, err := resolve(input, state)

	// then
	if err == nil {
		t.Error("expected error for missing node reference")
	}
}

func TestResolveNoMarkers(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: make(map[string]any),
	}

	type Input struct {
		Name    string
		Count   int
		Enabled bool
	}

	input := Input{
		Name:    "test",
		Count:   42,
		Enabled: true,
	}

	// when
	resolved, err := resolve(input, state)

	// then
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Name != "test" {
		t.Errorf("Name = %q, want %q", resolved.Name, "test")
	}

	if resolved.Count != 42 {
		t.Errorf("Count = %d, want %d", resolved.Count, 42)
	}

	if !resolved.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestResolveCursorRef(t *testing.T) {
	t.Parallel()

	// given
	cursorTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	state := &FlowState{
		cursors: map[string]Cursor{
			"jira": {
				Source:   "jira",
				Position: cursorTime.Format(time.RFC3339),
			},
		},
	}

	// when
	result := ResolveCursorRef("jira", state)

	// then
	if result == nil {
		t.Fatal("ResolveCursorRef returned nil")
	}

	if !result.Equal(cursorTime) {
		t.Errorf("got %v, want %v", result, cursorTime)
	}
}

func TestResolveCursorRefNotFound(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		cursors: make(map[string]Cursor),
	}

	// when
	result := ResolveCursorRef("nonexistent", state)

	// then
	if result != nil {
		t.Errorf("expected nil for nonexistent cursor, got %v", result)
	}
}

func TestResolveOutputRef(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: map[string]any{
			"vpc": "my-vpc-id",
		},
	}

	// when
	result, ok := ResolveOutputRef("vpc", state)

	// then
	if !ok {
		t.Error("ResolveOutputRef returned false")
	}

	if result != "my-vpc-id" {
		t.Errorf("got %q, want %q", result, "my-vpc-id")
	}
}

func TestResolveOutputRefNotFound(t *testing.T) {
	t.Parallel()

	// given
	state := &FlowState{
		results: make(map[string]any),
	}

	// when
	_, ok := ResolveOutputRef("nonexistent", state)

	// then
	if ok {
		t.Error("expected false for nonexistent output")
	}
}
