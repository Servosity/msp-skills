// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestInferColumns(t *testing.T) {
	rows := []json.RawMessage{
		json.RawMessage(`{"Name":"Tod","Open Count":12,"Updated":"2026-05-01T10:00:00","Active":true,"Score":"7.5","Empty":null}`),
		json.RawMessage(`{"Name":"Ana","Open Count":9,"Updated":"2026-05-02","Active":false,"Score":"8.1","Empty":null}`),
	}
	cols := inferColumns(rows)
	byName := map[string]columnInfo{}
	for _, c := range cols {
		byName[c.Name] = c
	}
	tests := []struct {
		col  string
		want string
	}{
		{"Name", "text"},
		{"Open Count", "number"},
		{"Updated", "date"},
		{"Active", "boolean"},
		{"Score", "number"}, // number-shaped string
		{"Empty", "null"},
	}
	for _, tt := range tests {
		c, ok := byName[tt.col]
		if !ok {
			t.Fatalf("column %q missing from inference: %+v", tt.col, cols)
		}
		if c.Type != tt.want {
			t.Fatalf("column %q type = %q, want %q", tt.col, c.Type, tt.want)
		}
		if c.Sampled != 2 {
			t.Fatalf("column %q sampled = %d, want 2", tt.col, c.Sampled)
		}
	}
	if byName["Empty"].NonNull != 0 {
		t.Fatalf("null column non_null = %d, want 0", byName["Empty"].NonNull)
	}
	if byName["Name"].NonNull != 2 {
		t.Fatalf("Name non_null = %d, want 2", byName["Name"].NonNull)
	}
	if byName["Open Count"].WhereHint == "" {
		t.Fatal("numeric column should carry a where_hint")
	}
}

func TestJSONScalarType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`"2026-05-01"`, "date"},
		{`"2026-05-01 10:00:00"`, "date"},
		{`"12.5"`, "number"},
		{`"hello"`, "text"},
		{`12.5`, "number"},
		{`true`, "boolean"},
		{`null`, "null"},
		{`{"a":1}`, "object"},
		{`[1,2]`, "array"},
	}
	for _, tt := range tests {
		if got := jsonScalarType(json.RawMessage(tt.in)); got != tt.want {
			t.Fatalf("jsonScalarType(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDominantTypeAllNull(t *testing.T) {
	if got := dominantType(map[string]int{"null": 3}); got != "null" {
		t.Fatalf("all-null column type = %q, want null", got)
	}
	if got := dominantType(map[string]int{"number": 2, "text": 2}); got != "number" {
		t.Fatalf("tie should resolve by priority order, got %q", got)
	}
}
