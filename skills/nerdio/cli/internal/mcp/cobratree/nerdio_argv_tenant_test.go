// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/cliutil"
)

// nerdio handfix tenant-secrets-shown-never-stored, child argv: a LAPS password
// the MCP server showed its caller (nerdio_read devices.laps) is not held, so
// before this the caller could hand it back as a tool argument to a mirrored
// command that writes a file (teach --notes), and the child CLI, which never
// saw the read, stored it. The child argv check refuses a shown tenant secret
// in any form, as it refuses a held credential.
func TestChildArgvNeverRepeatsAShownTenantSecret(t *testing.T) {
	const laps = "LapsArgv-8d21f9c4Q"
	cliutil.ShowTenantSecrets([]byte(`{"password":"` + laps + `","accountName":"admin"}`))
	for _, argv := range [][]string{
		{"teach", "--query=find device", "--notes=" + laps},
		{"teach", "--query=find device", "--notes=" + base64.StdEncoding.EncodeToString([]byte(laps))},
	} {
		_, err := RunCLICommand(context.Background(), "/nonexistent/nerdio-cli", argv)
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("RunCLICommand(%v) with a shown tenant secret: want the refusal, got %v", argv, err)
		}
		if err != nil && strings.Contains(err.Error(), laps) {
			t.Errorf("the refusal repeats the secret: %v", err)
		}
	}
	// An ordinary argument still reaches the child (here: the exec of a
	// missing binary fails, which is not the refusal).
	_, err := RunCLICommand(context.Background(), "/nonexistent/nerdio-cli", []string{"teach", "--notes=device admin rotated"})
	if err != nil && strings.Contains(err.Error(), "repeats a credential") {
		t.Errorf("an ordinary argument was refused: %v", err)
	}
}
