// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Codex review r1 class "documented field type misread" (datto-rmm handfix
// fleet-reads-every-documented-shape): the fleet analytics decode synced and
// live Datto records into Go structs and skip a record that does not decode.
// ActivityLog.site and ActivityLog.user are objects in Datto's own spec
// (SiteBasicDto, UserDto) but were read as strings, so every real activity
// vanished from `fleet device-timeline`. Every struct a fleet analytic reads a
// Datto record into must decode the record built from the vendor schema
// (every documented property, of its documented type, nested objects and
// arrays filled).
func TestFleetStructsDecodeEveryDocumentedShape(t *testing.T) {
	schemas := loadVendorSchemas(t)
	cases := []struct {
		schema string
		into   func() any
	}{
		{"Device", func() any { return &fleetDevice{} }},
		{"Device", func() any { return &udfDevice{} }},
		{"Alert", func() any { return &fleetAlert{} }},
		{"Site", func() any { return &fleetSite{} }},
		{"ActivityLog", func() any { return &fleetActivityLog{} }},
		{"Software", func() any { return &fleetSoftware{} }},
		{"JobResults", func() any { return &jobResultsRecord{} }},
	}
	for _, c := range cases {
		for variant := 0; variant < 3; variant++ {
			doc := vendorExample(schemas, map[string]any{"$ref": "#/components/schemas/" + c.schema}, variant, 0)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, c.into()); err != nil {
				t.Errorf("%T does not decode a documented %s record (variant %d): %v\n%s", c.into(), c.schema, variant, err, raw)
			}
		}
	}
}

// The failing input from the review: a documented activity (site and user as
// objects) appears in the device's timeline with its site and user named.
func TestDeviceTimelineKeepsDocumentedActivities(t *testing.T) {
	raw := []byte(`{"id":"log-1","hostname":"PC1","date":1793232000.25,"action":"deployment","category":"job","entity":"DEVICE","site":{"id":1,"name":"Acme"},"user":{"id":2,"userName":"tech","firstName":"Tess","lastName":"Tech"}}`)
	var l fleetActivityLog
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("a documented ActivityLog does not decode: %v", err)
	}
	events := buildTimeline(nil, []fleetActivityLog{l}, "PC1", 0, 0, time.Unix(1793232000, 0).Add(time.Hour))
	if len(events) != 1 {
		t.Fatalf("the documented activity is missing from the timeline: %+v", events)
	}
	if events[0].Site != "Acme" || !strings.Contains(events[0].Summary, "tech") {
		t.Fatalf("the activity lost its site or user: %+v", events[0])
	}
}

func loadVendorSchemas(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if err != nil {
		// The published tree withholds the vendor spec (press/publication.json);
		// the factory's library copy keeps it, where this test runs.
		t.Skipf("spec.json not beside the module: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Components.Schemas) == 0 {
		t.Fatal("the vendor spec has no schemas")
	}
	return spec.Components.Schemas
}

// vendorExample builds a value of the schema: every property present, enums
// and oneOf branches rotated by variant, arrays with one element.
func vendorExample(schemas map[string]any, s map[string]any, variant, depth int) any {
	if depth > 8 {
		return nil
	}
	if ref, ok := s["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		next, _ := schemas[name].(map[string]any)
		return vendorExample(schemas, next, variant, depth+1)
	}
	for _, k := range []string{"oneOf", "anyOf"} {
		if alts, ok := s[k].([]any); ok && len(alts) > 0 {
			alt, _ := alts[variant%len(alts)].(map[string]any)
			return vendorExample(schemas, alt, variant, depth+1)
		}
	}
	if all, ok := s["allOf"].([]any); ok {
		merged := map[string]any{}
		for _, a := range all {
			if part, ok := vendorExample(schemas, a.(map[string]any), variant, depth+1).(map[string]any); ok {
				for k, v := range part {
					merged[k] = v
				}
			}
		}
		return merged
	}
	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		if s["type"] != "array" {
			return enum[variant%len(enum)]
		}
	}
	switch s["type"] {
	case "string":
		return []string{"2026-01-02T03:04:05Z", "text", "1793232000"}[variant%3]
	case "integer":
		return 7 + variant
	case "number":
		return 1793232000.714
	case "boolean":
		return variant%2 == 0
	case "array":
		items, _ := s["items"].(map[string]any)
		if items == nil {
			return []any{}
		}
		return []any{vendorExample(schemas, items, variant, depth+1)}
	}
	props, _ := s["properties"].(map[string]any)
	out := map[string]any{}
	for k, v := range props {
		ps, _ := v.(map[string]any)
		out[k] = vendorExample(schemas, ps, variant, depth+1)
	}
	return out
}
