// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

// Every startup refusal reaches the operator: stderr passes the credential kit,
// and credkit.Exit drains it before the server exits.
func TestStartupErrorsAreShownBeforeExit(t *testing.T) {
	bin := mcpBinary(t)
	testenv.Isolate(t)
	cases := []struct {
		name string
		args []string
		env  []string
		want string
	}{
		{"http without a token", []string{"--transport", "http"}, []string{"NETSAPIENS_MCP_HTTP_TOKEN="}, "NETSAPIENS_MCP_HTTP_TOKEN"},
		{"unknown transport", []string{"--transport", "carrier-pigeon"}, nil, "unknown --transport"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Env = append(os.Environ(), tc.env...)
			var errb bytes.Buffer
			cmd.Stderr = &errb
			if err := cmd.Run(); err == nil {
				t.Fatal("server started; want a refusal")
			}
			if !strings.Contains(errb.String(), tc.want) {
				t.Fatalf("stderr = %q; want it to name %q", errb.String(), tc.want)
			}
		})
	}
}
