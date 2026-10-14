// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json secret-resources-never-persisted (context tool).

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestContextDoesNotAdvertiseSecretResourcesAsSyncable: sync refuses the
// never-stored resources, so the context tool must not call them syncable.
func TestContextDoesNotAdvertiseSecretResourcesAsSyncable(t *testing.T) {
	s := server.NewMCPServer("connectsecure", "test")
	RegisterTools(s)
	result, err := handleContext(s)(context.Background(), mcplib.CallToolRequest{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("handleContext: %v", err)
	}
	var payload struct {
		Resources []struct {
			Name       string `json:"name"`
			Syncable   bool   `json:"syncable"`
			Searchable bool   `json:"searchable"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(mcpTextContent(t, result)), &payload); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, r := range payload.Resources {
		switch r.Name {
		case "credentials", "scheduler", "integration_credentials", "integration_rules", "attack_surface_results", "custom_domains":
			seen++
			if r.Syncable || r.Searchable {
				t.Errorf("%s advertised syncable=%v searchable=%v", r.Name, r.Syncable, r.Searchable)
			}
		case "companies":
			if !r.Syncable {
				t.Error("companies lost syncable")
			}
		}
	}
	if seen != 6 {
		t.Fatalf("found %d of 6 secret resources in context", seen)
	}
}
