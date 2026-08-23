// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"testing"
)

// recordingPostClient captures the last body handed to a POST query so the
// tests below can assert the exact wire shape.
type recordingPostClient struct{ lastBody any }

func (c *recordingPostClient) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

func (c *recordingPostClient) PostWithParams(_ context.Context, _ string, _ map[string]string, body any) (json.RawMessage, int, error) {
	c.lastBody = body
	return json.RawMessage(`[]`), 200, nil
}

func (c *recordingPostClient) PostQueryWithParams(_ context.Context, _ string, _ map[string]string, body any) (json.RawMessage, int, error) {
	c.lastBody = body
	return json.RawMessage(`[]`), 200, nil
}

func fetchBody(t *testing.T, resource string, params map[string]string, cursor string) string {
	t.Helper()
	idWalk, ok := syncResourceIDWalkConfig(resource)
	if !ok {
		t.Fatalf("%s declares no id-walk config", resource)
	}
	c := &recordingPostClient{}
	if _, err := syncFetch(context.Background(), c, resource, "/"+resource+"/query", params, idWalk, cursor); err != nil {
		t.Fatalf("syncFetch(%s, cursor=%q): %v", resource, cursor, err)
	}
	encoded, err := json.Marshal(c.lastBody)
	if err != nil {
		t.Fatalf("marshaling captured body: %v", err)
	}
	return string(encoded)
}

// The Autotask query endpoint rejects a filterless body with HTTP 500
// "Value cannot be null. Parameter name: filters" (issue #257). The first
// page must carry the id-walk's own condition, not an empty array — an empty
// condition list matches zero records and turns the failure silent.
func TestSyncPostFirstPageSeedsIDWalkFilter(t *testing.T) {
	got := fetchBody(t, "companies", map[string]string{"maxRecords": "100"}, "")
	want := `{"filter":[{"field":"id","op":"gte","value":0}],"maxRecords":100}`
	if got != want {
		t.Errorf("first-page body = %s, want %s", got, want)
	}
}

func TestSyncPostLaterPagesWalkFromCursor(t *testing.T) {
	got := fetchBody(t, "companies", map[string]string{"maxRecords": "100"}, "42")
	want := `{"filter":[{"field":"id","op":"gt","value":42}],"maxRecords":100}`
	if got != want {
		t.Errorf("second-page body = %s, want %s", got, want)
	}
}

// A caller-supplied filter must survive as JSON (not the literal string
// "[...]"), and must not be overwritten by the first-page seed.
func TestSyncPostCoercesCallerSuppliedArrayFilter(t *testing.T) {
	params := map[string]string{
		"maxRecords": "100",
		"filter":     `[{"field":"companyName","op":"beginsWith","value":"A"}]`,
	}
	got := fetchBody(t, "companies", params, "")
	want := `{"filter":[{"field":"companyName","op":"beginsWith","value":"A"}],"maxRecords":100}`
	if got != want {
		t.Errorf("caller-filter body = %s, want %s", got, want)
	}
}

// A supplied-but-empty filter is rejected rather than silently replaced by the
// seed: an empty condition list matches no records, so substituting the seed
// would turn the caller's explicit request into a whole-tenant sync.
func TestSyncPostRejectsSuppliedEmptyFilter(t *testing.T) {
	for _, value := range []string{"[]", "null", "", "   ", `"[]"`} {
		idWalk, _ := syncResourceIDWalkConfig("companies")
		c := &recordingPostClient{}
		params := map[string]string{"maxRecords": "100", "filter": value}
		if _, err := syncFetch(context.Background(), c, "companies", "/companies/query", params, idWalk, ""); err == nil {
			t.Errorf("filter=%q: want an error, got body %v", value, c.lastBody)
		}
	}
}

func TestCoerceSyncBodyParamDecodesArraysAndCSV(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		typ   string
		want  string
	}{
		{"json array", `[{"field":"id","op":"gte","value":0}]`, "array", `[{"field":"id","op":"gte","value":0}]`},
		{"empty json array", `[]`, "array", `[]`},
		{"csv array", "id,companyName", "string_csv_array", `["id","companyName"]`},
		{"unparseable array falls back to string", "not json", "array", `"not json"`},
		{"int untouched", "100", "int", `100`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(coerceSyncBodyParam(tc.value, tc.typ))
			if err != nil {
				t.Fatalf("marshaling coerced value: %v", err)
			}
			if string(encoded) != tc.want {
				t.Errorf("coerceSyncBodyParam(%q, %q) = %s, want %s", tc.value, tc.typ, encoded, tc.want)
			}
		})
	}
}

// Every POST sync resource walks by id and declares an array `filter`, so the
// seed must apply to all of them, not just companies.
func TestEveryPostSyncResourceSeedsAFilter(t *testing.T) {
	for _, resource := range []string{"companies", "contacts", "tickets", "projects", "time-entries"} {
		got := fetchBody(t, resource, map[string]string{"maxRecords": "100"}, "")
		var decoded map[string]any
		if err := json.Unmarshal([]byte(got), &decoded); err != nil {
			t.Fatalf("%s: %v", resource, err)
		}
		filters, ok := decoded["filter"].([]any)
		if !ok || len(filters) == 0 {
			t.Errorf("%s first-page body has no filter conditions: %s", resource, got)
		}
	}
}

// A filter that is not a JSON array must be rejected, never treated as absent.
// Treating it as absent would let the first-page seed silently replace the
// caller's restriction and sync the whole tenant.
func TestSyncPostRejectsNonArrayFilter(t *testing.T) {
	for _, value := range []string{
		`{"field":"companyName","op":"beginsWith","value":"A"}`,
		`5`,
		`true`,
		`"not json`,
	} {
		idWalk, ok := syncResourceIDWalkConfig("companies")
		if !ok {
			t.Fatal("companies declares no id-walk config")
		}
		params := map[string]string{"maxRecords": "100", "filter": value}
		c := &recordingPostClient{}
		_, err := syncFetch(context.Background(), c, "companies", "/companies/query", params, idWalk, "")
		if err == nil {
			t.Errorf("filter=%s: want an error, got body %v", value, c.lastBody)
		}
	}
}

// A field-restricted sync must still request the walk's key column, or the
// walk cannot advance past page 1.
func TestSyncPostKeepsIDFieldInIncludeFields(t *testing.T) {
	got := fetchBody(t, "companies", map[string]string{"maxRecords": "100", "includeFields": "companyName,phone"}, "")
	want := `{"filter":[{"field":"id","op":"gte","value":0}],"includeFields":["companyName","phone","id"],"maxRecords":100}`
	if got != want {
		t.Errorf("restricted-field body = %s, want %s", got, want)
	}
}

func TestSyncPostDoesNotDuplicateIDField(t *testing.T) {
	got := fetchBody(t, "companies", map[string]string{"maxRecords": "100", "includeFields": "id,companyName"}, "")
	want := `{"filter":[{"field":"id","op":"gte","value":0}],"includeFields":["id","companyName"],"maxRecords":100}`
	if got != want {
		t.Errorf("already-keyed body = %s, want %s", got, want)
	}
}
