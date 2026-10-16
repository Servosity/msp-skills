// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"bitdefender-pp-cli/internal/cliutil"
)

// Every listed method must exist in the registry and be a read; every
// registered get/find method that is not listed must be one of the
// credential-bearing reads the list deliberately leaves out.
func TestGravityZoneReadOnlyMethodsMatchRegistry(t *testing.T) {
	excluded := map[string]bool{
		"push#getPushEventSettings":          true,
		"licensing#getLicenseInfo":           true,
		"companies#getCompanyDetailsByUser":  true,
		"packages#getPackageDetails":         true,
		"packages#getInstallationLinks":      true,
		"integrations#getIntegrationDetails": true,
	}
	registered := map[string]bool{}
	for _, ep := range codeOrchEndpoints {
		_, rest, _ := strings.Cut(ep.Path, "/jsonrpc/")
		registered[rest] = true
		_, method, _ := strings.Cut(rest, "#")
		read := strings.HasPrefix(method, "get") || strings.HasPrefix(method, "find")
		switch {
		case gravityZoneReadOnlyAllows(ep.Path) && !read:
			t.Errorf("%s is allowed under read-only but is not a get/find method", ep.Path)
		case read && !gravityZoneReadOnlyAllows(ep.Path) && !excluded[rest]:
			t.Errorf("read method %s is neither allowed nor deliberately excluded", ep.Path)
		case excluded[rest] && gravityZoneReadOnlyAllows(ep.Path):
			t.Errorf("credential-bearing read %s is allowed under read-only", ep.Path)
		}
	}
	for m := range gravityZoneReadOnlyMethods {
		if !registered[m] {
			t.Errorf("allowlisted method %s is not in the endpoint registry", m)
		}
	}
	for m := range excluded {
		if !registered[m] {
			t.Errorf("excluded method %s is not in the endpoint registry", m)
		}
	}
	for _, p := range []string{"", "network#getEndpointsList", "/jsonrpc/network#getEndpointsList", "/v1.0/jsonrpc/network#getEndpointsList#x"} {
		if gravityZoneReadOnlyAllows(p) {
			t.Errorf("malformed template %q was allowed", p)
		}
	}
}

// Under the switch, a listed read reaches the client (and fails on missing
// config, not on the switch); a write and an excluded read are refused.
func TestCodeOrchExecuteReadOnlyRunsGravityZoneReads(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	t.Setenv("BITDEFENDER_API_KEY", "")
	call := func(id string) string {
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": id},
		}})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if res == nil || len(res.Content) == 0 {
			return ""
		}
		text, _ := res.Content[0].(mcplib.TextContent)
		return text.Text
	}
	if got := call("endpoints.list"); strings.Contains(got, "refusing") {
		t.Fatalf("endpoints.list refused under read-only: %s", got)
	}
	for _, id := range []string{"isolation.isolate", "endpoints.delete", "push.get", "license.get"} {
		if got := call(id); !strings.Contains(got, "refusing") {
			t.Fatalf("%s was not refused under read-only: %s", id, got)
		}
	}
}
