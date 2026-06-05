// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCSVColumnsFromRow(t *testing.T) {
	row := json.RawMessage(`{"b":1,"a":"x","c":null}`)
	got := csvColumnsFromRow(row)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("csvColumnsFromRow = %v, want %v (sorted)", got, want)
	}
}

func TestCSVRecordForRow(t *testing.T) {
	cols := []string{"a", "b", "c", "missing"}
	row := json.RawMessage(`{"a":"plain","b":12.5,"c":null,"extra":"dropped"}`)
	got := csvRecordForRow(row, cols)
	want := []string{"plain", "12.5", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("csvRecordForRow = %v, want %v", got, want)
	}
}

func TestCSVRecordForRowNestedJSON(t *testing.T) {
	cols := []string{"obj"}
	row := json.RawMessage(`{"obj":{"k":1}}`)
	got := csvRecordForRow(row, cols)
	if got[0] != `{"k":1}` {
		t.Fatalf("nested JSON cell = %q, want verbatim JSON", got[0])
	}
}
