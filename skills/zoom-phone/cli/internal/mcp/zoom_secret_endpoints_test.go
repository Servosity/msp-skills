// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"zoom-phone-pp-cli/internal/cliutil"
)

func TestZoomSecretEndpointsBlockedInReadOnlyMode(t *testing.T) {
	for id := range zoomSecretEndpointIDs {
		ep := findCodeOrchEndpoint(id)
		if ep == nil || ep.Method != "GET" {
			t.Fatalf("%s is not a GET endpoint in the code-orch registry; update zoomSecretEndpointIDs", id)
		}
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	if !zoomCodeOrchSecretBlocked("devices.get-adevice") {
		t.Fatal("secret endpoint allowed in read-only mode")
	}
	if zoomCodeOrchSecretBlocked("devices.list-phone") {
		t.Fatal("ordinary read blocked")
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if zoomCodeOrchSecretBlocked("devices.get-adevice") {
		t.Fatal("secret endpoint blocked outside read-only mode")
	}
}
