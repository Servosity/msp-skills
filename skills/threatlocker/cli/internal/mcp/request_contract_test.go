// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"context"
	"encoding/json"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"testing"
)

func requestContractSearch(t *testing.T, query string) []map[string]any {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": query, "limit": float64(1000)}
	result, err := handleCodeOrchSearch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("search failed: %#v", result)
	}
	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(mcplib.TextContent).Text), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Results
}

func requestContractEndpoint(t *testing.T, query, id string) map[string]any {
	t.Helper()
	for _, ep := range requestContractSearch(t, query) {
		if ep["endpoint_id"] == id {
			return ep
		}
	}
	t.Fatalf("%s missing from discovery for %q", id, query)
	return nil
}

func TestRequestContractCreate(t *testing.T) {
	ep := requestContractEndpoint(t, "applications.create", "applications.create")
	schema, ok := ep["params_schema"].(map[string]any)
	if !ok {
		t.Fatal("applications.create discovery lacks params_schema")
	}
	props, _ := schema["properties"].(map[string]any)
	name, _ := props["name"].(map[string]any)
	if name["type"] != "string" || name["x-location"] != "body" {
		t.Fatalf("name contract = %#v", name)
	}
	required, _ := schema["required"].([]any)
	for _, key := range required {
		if key == "name" {
			return
		}
	}
	t.Fatal("name must be required")
}

func TestRequestContractSearchField(t *testing.T) {
	requestContractEndpoint(t, "name", "applications.create")
}

func TestRequestContractSearchShortExact(t *testing.T) {
	// This connector declares application-id/applicationId, not a literal id.
	// An exact short token must not match one component of a longer input name.
	if got := requestContractSearch(t, "id"); len(got) != 0 {
		t.Fatalf("id matched longer names: %#v", got)
	}
}

func TestRequestContractSearchRejectsSubstringNoise(t *testing.T) {
	if got := requestContractSearch(t, "zzzzapplicationszzzz"); len(got) != 0 {
		t.Fatalf("substring-only query returned %d irrelevant results: %#v", len(got), got)
	}
}

func TestRequestContractShortTokenDoesNotMatchDescriptions(t *testing.T) {
	// "on" must not match application, description, location or organization.
	if got := requestContractSearch(t, "on"); len(got) != 0 {
		t.Fatalf("short-token substring noise: %#v", got)
	}
}

func TestRequestContractAllEndpointBindings(t *testing.T) {
	for _, registered := range codeOrchEndpoints {
		t.Run(registered.ID, func(t *testing.T) {
			ep := requestContractEndpoint(t, registered.ID, registered.ID)
			schema, ok := ep["params_schema"].(map[string]any)
			if !ok || schema["type"] != "object" {
				t.Fatalf("missing object params_schema: %#v", schema)
			}
			props, _ := schema["properties"].(map[string]any)
			check := func(name, location string) {
				t.Helper()
				field, ok := props[name].(map[string]any)
				// A body field may own the same wire key as a path input. In
				// that case discovery must give the path a distinct public name.
				if location == "path" && (!ok || field["x-location"] != location) {
					for _, value := range props {
						candidate, _ := value.(map[string]any)
						if candidate["x-wire-name"] == name && candidate["x-location"] == location {
							field, ok = candidate, true
							break
						}
					}
				}
				if !ok || field["x-location"] != location {
					t.Errorf("declared %s %q lacks matching discovery: %#v", location, name, field)
				}
			}
			for _, name := range registered.Positional {
				check(name, "path")
			}
			for _, binding := range registered.QueryParams {
				check(binding.PublicName, "query")
			}
			for _, binding := range registered.TemplateParams {
				check(binding.PublicName, "template")
			}
		})
	}
}
