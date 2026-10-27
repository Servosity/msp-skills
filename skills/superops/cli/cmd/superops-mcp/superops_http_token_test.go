// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for superops_http_token.go
// (superops-mcp-http-token-unguessable).

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHTTPCallerTokenMustBeUnguessable(t *testing.T) {
	for _, tc := range []struct {
		token string
		ok    bool
	}{
		{"${user_config.superops_mcp_http_token}", false},
		{"$(openssl rand -hex 32)", false},
		{"$SUPEROPS_MCP_HTTP_TOKEN_VALUE_FROM_THE_DOCS_X", false},
		{"shared-secret", false},
		{strings.Repeat("a", 31), false},
		{strings.Repeat("ab", 16) + " " + strings.Repeat("cd", 16), false},
		{strings.Repeat("0f", 32), true},
		{strings.Repeat("a", 32), true},
		{"5f0d6c1e-2b7a-4c9e-8d3f-1a2b3c4d5e6f", true},
		{"q8Zr+u/Vb3Kp1Xw9Lm2Nc4Td6Fg7Hj0Sa5Ye1Ri=", true},
	} {
		if err := requireUnguessableHTTPCallerToken(tc.token); (err == nil) != tc.ok {
			t.Errorf("requireUnguessableHTTPCallerToken(%q) = %v, want ok=%v", tc.token, err, tc.ok)
		}
	}
}

// The round-1 input: HTTP mode with the manifest's unresolved placeholder as
// its token. The server runs as a child process (main() with HTTP flags) and
// must exit refusing, never serve.
func TestHTTPModeRefusesAGuessableToken(t *testing.T) {
	if os.Getenv("SUPEROPS_MCP_TOKEN_HELPER") == "1" {
		os.Args = []string{"superops-mcp", "--transport", "http", "--addr", "127.0.0.1:0"}
		main()
		return
	}
	for _, token := range []string{"${user_config.superops_mcp_http_token}", "shared-secret"} {
		home := t.TempDir()
		cmd := exec.Command(os.Args[0], "-test.run=^TestHTTPModeRefusesAGuessableToken$")
		cmd.Env = append(os.Environ(), "SUPEROPS_MCP_TOKEN_HELPER=1", httpTokenEnvVar+"="+token,
			"HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+home, "XDG_DATA_HOME="+home,
			"XDG_STATE_HOME="+home, "XDG_CACHE_HOME="+home, "SUPEROPS_HOME="+home, "PP_MCP_TRANSPORT=")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(stderr.String(), "not a usable bearer token") {
				t.Errorf("token %q: exit %v, want the refusal:\n%s", token, err, stderr.String())
			}
			if strings.Contains(stderr.String(), token) {
				t.Errorf("token %q is shown:\n%s", token, stderr.String())
			}
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Errorf("token %q: HTTP mode started serving:\n%s", token, stderr.String())
		}
	}
}
