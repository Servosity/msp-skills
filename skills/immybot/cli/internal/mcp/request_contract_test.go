// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func requestContractCall(args map[string]any) mcplib.CallToolRequest {
	var req mcplib.CallToolRequest
	req.Params.Arguments = args
	return req
}

func requestContractResultJSON(t *testing.T, result *mcplib.CallToolResult, err error) map[string]any {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.IsError || len(result.Content) == 0 {
		t.Fatalf("unexpected MCP result: %#v", result)
	}
	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("expected text metadata, got %T", result.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("metadata JSON: %v: %s", err, text.Text)
	}
	return out
}

func requestContractSearch(t *testing.T, query, id string) map[string]any {
	t.Helper()
	result, err := handleCodeOrchSearch(context.Background(), requestContractCall(map[string]any{"query": query, "limit": float64(10)}))
	payload := requestContractResultJSON(t, result, err)
	results, ok := payload["results"].([]any)
	if !ok {
		t.Fatalf("search has no results array: %#v", payload)
	}
	for _, raw := range results {
		item := raw.(map[string]any)
		if item["endpoint_id"] == id {
			return item
		}
	}
	t.Fatalf("search %q did not find %s: %#v", query, id, results)
	return nil
}

func requestContractProperty(t *testing.T, schema map[string]any, wireName string) map[string]any {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing JSON Schema properties: %#v", schema)
	}
	for name, raw := range props {
		p, ok := raw.(map[string]any)
		if ok && (name == wireName || p["x-wire-name"] == wireName) {
			return p
		}
	}
	t.Fatalf("missing property %q: %#v", wireName, props)
	return nil
}

func requestContractSchema(t *testing.T, metadata map[string]any) map[string]any {
	t.Helper()
	schema, ok := metadata["params_schema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Fatalf("endpoint metadata must expose an object params_schema: %#v", metadata)
	}
	return schema
}

// Issue #351: discovery must explain the same named endpoint that execute sends.
func TestRequestContractSearchDescribesNamedWrite(t *testing.T) {
	const endpoint = "brandings.create"
	base := requestContractSearch(t, "brandings create", endpoint)
	withField := requestContractSearch(t, "brandings create"+" "+"description", endpoint)
	if withField["score"].(float64) <= base["score"].(float64) {
		t.Errorf("request field did not contribute to discovery score: base=%v field=%v", base["score"], withField["score"])
	}
	schema := requestContractSchema(t, withField)
	requestContractRequired(t, schema, "description")
	fields := map[string]string{"description": "string"}
	for name, typ := range fields {
		p := requestContractProperty(t, schema, name)
		if p["type"] != typ || p["x-location"] != "body" {
			t.Errorf("%s: want type=%s location=body, got %#v", name, typ, p)
		}
	}

}

func requestContractRequired(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	required, _ := schema["required"].([]any)
	for _, name := range names {
		found := false
		for _, value := range required {
			if value == name {
				found = true
			}
		}
		if !found {
			t.Errorf("required field %q missing from %v", name, required)
		}
	}
}

func TestRequestContractShortTermsDoNotMatchEverything(t *testing.T) {
	result, err := handleCodeOrchSearch(context.Background(), requestContractCall(map[string]any{"query": "is the and"}))
	payload := requestContractResultJSON(t, result, err)
	if payload["count"] != float64(0) {
		t.Fatalf("short terms must not create search noise: %#v", payload)
	}
}

func TestRequestContractGetMetadataStaysReadOnly(t *testing.T) {
	result, err := handleCodeOrchGet(context.Background(), requestContractCall(map[string]any{"endpoint_id": "brandings.get-by-id"}))
	metadata := requestContractResultJSON(t, result, err)
	p := requestContractProperty(t, requestContractSchema(t, metadata), "id")
	if p["x-location"] != "path" {
		t.Fatalf("wrong GET parameter location: %#v", p)
	}
	result, err = handleCodeOrchGet(context.Background(), requestContractCall(map[string]any{"endpoint_id": "brandings.create"}))
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("GET metadata must retain its method guard: result=%#v err=%v", result, err)
	}
}

func requestContractConfig(t *testing.T, baseURL string) {
	t.Helper()
	dir := t.TempDir()
	const prefix = "IMMYBOT"
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, prefix+"_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HOME", dir)
	t.Setenv(prefix+"_HOME", dir)
	t.Setenv(prefix+"_NO_CONFIG_WRITE", "true")
	configPath := filepath.Join(dir, "config.toml")
	configText := fmt.Sprintf("base_url = %q\naccess_token = %q\ntoken_expiry = 2099-01-01T00:00:00Z\n", baseURL, "contract-test-token")

	if err := os.WriteFile(configPath, []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(prefix+"_CONFIG", configPath)
	t.Setenv(prefix+"_BASE_URL", baseURL)
}

// This drives the real handler and client against a local API fixture. It
// exercises serialization and authentication without writing to a tenant.
func TestRequestContractExecuteNamedWriteOnWire(t *testing.T) {
	const bodyJSON = `{"description":"Fixture branding","backgroundColor":"#ffffff","ignoreYear":false}`
	var expected map[string]any
	if err := json.Unmarshal([]byte(bodyJSON), &expected); err != nil {
		t.Fatal(err)
	}
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/brandings" {
			t.Errorf("request route: %s %s", r.Method, r.URL)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("body fields leaked into query: %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer contract-test-token" {
			t.Errorf("authentication lost: header=%q", got)
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var actual map[string]any
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Errorf("body must be JSON object, got %s (%v)", raw, err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("wire body differs: got %s want %s", raw, bodyJSON)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	var params map[string]any
	_ = json.Unmarshal([]byte(bodyJSON), &params)

	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": "brandings.create", "params": params}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("execute failed: result=%#v err=%v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("execute did not reach the local fixture")
	}
}

func TestRequestContractExecuteAdvertisedGETBindings(t *testing.T) {
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/brandings/42" {
			t.Errorf("GET route: %s %s", r.Method, r.URL)
		}
		expectedQuery := map[string][]string{}
		if !reflect.DeepEqual(map[string][]string(r.URL.Query()), expectedQuery) {
			t.Errorf("query alias not resolved: got %v want %v", r.URL.Query(), expectedQuery)
		}
		raw, _ := io.ReadAll(r.Body)
		if len(raw) != 0 {
			t.Errorf("GET sent a body: %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	params := map[string]any{"id": "42"}
	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": "brandings.get-by-id", "params": params}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("GET execute failed: result=%#v err=%v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("GET did not reach the local fixture")
	}
}

func TestRequestContractExecuteNamedArrayBody(t *testing.T) {
	const endpoint = "computers.create-restore"
	metadata := requestContractSearch(t, "computers create restore", endpoint)
	p := requestContractProperty(t, requestContractSchema(t, metadata), "body")
	if p["type"] != "array" || p["x-location"] != "body" {
		t.Errorf("array body schema: %#v", p)
	}
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/computers/restore" {
			t.Errorf("array endpoint route: %s %s", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		var actual, expected any
		_ = json.Unmarshal([]byte(`[42,43]`), &expected)
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("array body changed: got %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	var body []any
	_ = json.Unmarshal([]byte(`[42,43]`), &body)
	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": endpoint, "params": map[string]any{"body": body}}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("array execute failed: %#v %v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("array request did not reach fixture")
	}
}

// A path identifier and an identically named JSON field must be independent.
func TestRequestContractPathAndBodyMayShareWireName(t *testing.T) {
	const endpoint = "brandings.update-by-id"
	metadata := requestContractSearch(t, "brandings update by id", endpoint)
	schema := requestContractSchema(t, metadata)
	props := schema["properties"].(map[string]any)
	pathField := props["path_id"].(map[string]any)
	bodyField := props["id"].(map[string]any)
	if pathField["x-location"] != "path" || pathField["x-wire-name"] != "id" || bodyField["x-location"] != "body" {
		t.Fatalf("path/body locations: %#v", props)
	}
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/brandings/42" {
			t.Errorf("path/body route: %s %s", r.Method, r.URL)
		}
		var actual map[string]any
		if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
			t.Error(err)
		}
		expected := map[string]any{"description": "Fixture branding", "id": float64(99)}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("path consumed body fields: got %#v want %#v", actual, expected)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	params := map[string]any{"description": "Fixture branding", "path_id": "42", "id": float64(99)}
	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": endpoint, "params": params}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("execute: %#v %v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("collision fixture was not called")
	}
}
