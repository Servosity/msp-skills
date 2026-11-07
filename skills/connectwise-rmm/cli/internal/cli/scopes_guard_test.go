// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/connectwise-rmm/handfixes.json
// under the "asio-token-json-regional" entry (every Asio token request passes
// the press's outbound guard).

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScopesRefusesPlaintextTokenURL: scopes mints with the client secret, so a
// plain-http token URL on a non-loopback host must be refused before any dial,
// exactly as auth login and the auto-mint refuse it.
func TestScopesRefusesPlaintextTokenURL(t *testing.T) {
	t.Setenv("CONNECTWISE_RMM_HOME", t.TempDir())
	t.Setenv("CONNECTWISE_RMM_BASE_URL", "http://plaintext-probe.invalid")
	t.Setenv("CONNECTWISE_RMM_TOKEN_URL", "http://plaintext-probe.invalid/v1/token")
	t.Setenv("CONNECTWISE_RMM_CLIENT_ID", "stub-id-1234")
	t.Setenv("CONNECTWISE_RMM_CLIENT_SECRET", "stub-secret-1234")

	var out bytes.Buffer
	cmd := RootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scopes", "--scopes", "platform.devices.read", "--json"})
	err := cmd.Execute()
	text := out.String()
	if err != nil {
		text += err.Error()
	}
	if !strings.Contains(text, "plaintext") || strings.Contains(text, "no such host") {
		t.Fatalf("scopes did not refuse the plaintext token URL before dialing: %s", text)
	}
}

// TestEveryAsioTokenRequestIsGuarded: the class check. Every production file
// that builds an Asio token request (it carries the client secret) also runs
// the outbound guard on that request's JSON body.
func TestEveryAsioTokenRequestIsGuarded(t *testing.T) {
	files, _ := filepath.Glob("../*/*.go")
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "NewAsioTokenRequest(") || strings.Contains(src, "func NewAsioTokenRequest(") {
			continue
		}
		seen++
		if strings.Count(src, "NewAsioTokenRequest(") != strings.Count(src, "AsioTokenBody(req)") {
			t.Errorf("%s builds an Asio token request without guarding it (GuardTokenRequest/guardOutbound with AsioTokenBody)", f)
		}
	}
	if seen < 3 {
		t.Fatalf("found %d Asio token request sites, want the auto-mint, auth login and scopes", seen)
	}
}
