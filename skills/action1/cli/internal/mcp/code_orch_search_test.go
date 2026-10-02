// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestCodeOrchSearchFindsRemoteAssistance verifies that agents can discover the
// remote-session tools using the request and readiness terms in their guidance.
func TestCodeOrchSearchFindsRemoteAssistance(t *testing.T) {
	const start = "endpoints.managed-id-remote-sessions-post"
	const status = "endpoints.managed-id-remote-sessions-session-id-get"
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{query: "assistance", want: []string{start}},
		{query: "connected", want: []string{start, status}},
		{query: "browser", want: []string{start, status}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{
				"query": tc.query,
			}}}
			result, err := handleCodeOrchSearch(context.Background(), req)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if result == nil || result.IsError || len(result.Content) != 1 {
				t.Fatalf("expected a successful search response, got %#v", result)
			}
			content, ok := result.Content[0].(mcplib.TextContent)
			if !ok {
				t.Fatalf("expected text content, got %T", result.Content[0])
			}
			var response struct {
				Results []struct {
					EndpointID string `json:"endpoint_id"`
				} `json:"results"`
			}
			if err := json.Unmarshal([]byte(content.Text), &response); err != nil {
				t.Fatalf("decode search response: %v", err)
			}
			found := make(map[string]bool)
			for _, endpoint := range response.Results {
				found[endpoint.EndpointID] = true
			}
			for _, id := range tc.want {
				if !found[id] {
					t.Errorf("search %q did not return %s: %s", tc.query, id, content.Text)
				}
			}
		})
	}
}

// TestCodeOrchSearchFindsInstallerConfiguration guards the agent-facing discovery
// path for reading and updating an existing software installer.
func TestCodeOrchSearchFindsInstallerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		query    string
		want     []string
		guidance map[string][]string
	}{
		{query: "installer", want: []string{
			"software-repository.packages-all-package-id-get",
			"software-repository.versions.packages-all-package-id-id-patch",
		}, guidance: map[string][]string{
			"software-repository.packages-all-package-id-get": {"fields: versions"},
			"software-repository.versions.packages-all-package-id-id-patch": {
				"params", "approval_status", "approve_updates", "EULA_accepted", "accept_eula",
			},
		}},
		{query: "displayed version", want: []string{
			"software-repository.versions.packages-all-package-id-id-get",
			"software-repository.versions.packages-all-package-id-id-patch",
		}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{
				"query": tc.query,
			}}}
			result, err := handleCodeOrchSearch(context.Background(), req)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if result == nil || result.IsError || len(result.Content) != 1 {
				t.Fatalf("expected a successful search response, got %#v", result)
			}
			content, ok := result.Content[0].(mcplib.TextContent)
			if !ok {
				t.Fatalf("expected text content, got %T", result.Content[0])
			}
			var response struct {
				Results []struct {
					EndpointID string `json:"endpoint_id"`
					Summary    string `json:"summary"`
				} `json:"results"`
			}
			if err := json.Unmarshal([]byte(content.Text), &response); err != nil {
				t.Fatalf("decode search response: %v", err)
			}
			found := make(map[string]string)
			for _, endpoint := range response.Results {
				found[endpoint.EndpointID] = endpoint.Summary
			}
			for _, id := range tc.want {
				if _, ok := found[id]; !ok {
					t.Errorf("search %q did not return %s: %s", tc.query, id, content.Text)
				}
			}
			for id, terms := range tc.guidance {
				for _, term := range terms {
					if !strings.Contains(found[id], term) {
						t.Errorf("search summary for %s omits required guidance %q: %s", id, term, found[id])
					}
				}
			}
		})
	}
}
