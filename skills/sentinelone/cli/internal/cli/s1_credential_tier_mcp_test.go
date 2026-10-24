// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the CLI-side credential-tier refusal
// (s1_credential_tier_mcp.go) for a CLI the MCP server spawned.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/cliutil/testenv"
)

const (
	tierRangerCommunity = "SNMP-COMMUNITY-SECRET-555"
	tierRangerSSHKey    = "SSHKEY-SECRET-444"
	tierRangerAuthKey   = "SNMPV3-AUTHKEY-222"
	tierUninstallPw     = "UNINSTALL-PW-SECRET-909"
)

// captureStdout runs fn with os.Stdout redirected (tail writes its NDJSON to
// os.Stdout directly) and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

// TestTailAndExportRefuseCredentialTierUnderMCP: a CLI the MCP server
// spawned (SENTINELONE_MCP_CHILD=1) refuses `tail ranger` and `export ranger`,
// and an export whose id smuggles the uninstall-password path into an
// ordinary account read, before any request; the operator's own terminal
// (no marker) still tails ranger.
func TestTailAndExportRefuseCredentialTierUnderMCP(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "uninstall-password") {
			_, _ = w.Write([]byte(`{"data":{"password":"` + tierUninstallPw + `"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"c1","community":"` + tierRangerCommunity + `","sshKey":"` + tierRangerSSHKey + `","authKey":"` + tierRangerAuthKey + `"}],"pagination":{"nextCursor":null}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "cli-tier-test-token")
	take := func() []string {
		mu.Lock()
		defer mu.Unlock()
		h := hits
		hits = nil
		return h
	}

	t.Setenv(cliutil.MCPChildEnvVar, "1")
	for _, args := range [][]string{
		{"tail", "ranger", "--follow=false"},
		{"tail", "--resource", "ranger", "--follow=false"},
		{"export", "ranger"},
		{"export", "ranger", "--format", "json"},
		{"export", "accounts", "123/uninstall-password/view"},
	} {
		var out string
		var err error
		stdout := captureStdout(t, func() { out, err = runSecretCLI(t, args...) })
		all := out + stdout
		if err != nil {
			all += " " + err.Error()
		}
		if err == nil || !strings.Contains(all, "credential-tier") {
			t.Errorf("%v under the MCP marker was not refused as credential-tier: err=%v\n%s", args, err, all)
		}
		for _, s := range []string{tierRangerCommunity, tierRangerSSHKey, tierRangerAuthKey, tierUninstallPw} {
			if strings.Contains(all, s) {
				t.Errorf("%v under the MCP marker printed %s", args, s)
			}
		}
		if h := take(); len(h) > 0 {
			t.Errorf("%v under the MCP marker sent %v before refusing", args, h)
		}
	}

	// The operator's own terminal: no marker, the read works as before.
	t.Setenv(cliutil.MCPChildEnvVar, "")
	stdout := captureStdout(t, func() {
		if out, err := runSecretCLI(t, "tail", "ranger", "--follow=false"); err != nil {
			t.Errorf("operator tail ranger failed: %v\n%s", err, out)
		}
	})
	if h := take(); len(h) != 1 || h[0] != "/ranger/cred-groups/details" {
		t.Errorf("operator tail ranger requests = %v, want one /ranger/cred-groups/details", h)
	}
	if !strings.Contains(stdout, tierRangerCommunity) {
		t.Errorf("operator tail ranger did not print the rows: %s", stdout)
	}
}
