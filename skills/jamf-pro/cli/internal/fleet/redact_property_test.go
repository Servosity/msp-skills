// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"strings"
	"testing"
)

// TestChildEnvAllowlistProperty: every credential-bearing or JAMF_PRO_*
// name, in every casing (Windows names are case-insensitive), is kept out of
// a child's environment; listed names pass in any casing (the runner adds the
// instance's own secret entry itself).
func TestChildEnvAllowlistProperty(t *testing.T) {
	casings := func(name string) []string {
		mixed := []rune(strings.ToLower(name))
		for i := range mixed {
			if i%2 == 0 {
				mixed[i] = []rune(strings.ToUpper(string(mixed[i])))[0]
			}
		}
		return []string{name, strings.ToLower(name), strings.ToUpper(name), string(mixed)}
	}
	denied := []string{"JAMF_PRO_FLEET_SECRETS", "JAMF_PRO_SECRET_GLOBEX", "JAMF_PRO_CLIENT_SECRET", "JAMF_PRO_CLIENT_ID", "JAMF_PRO_BASE_URL", "JAMF_PRO_TOKEN_URL",
		"JAMF_PRO_SERVER", "JAMF_PRO_CONFIG", "JAMF_PRO_HOME", "JAMF_PRO_IDENTITY", "PP_JAMF_PRO_IDENTITY", "AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "OP_SESSION_x"}
	for _, name := range denied {
		for _, n := range casings(name) {
			env := []string{n + "=leak", "PATH=/bin"}
			if got := ChildEnv(env); len(got) != 1 || got[0] != "PATH=/bin" {
				t.Errorf("ChildEnv kept %s: %v", n, got)
			}
			if strings.HasPrefix(strings.ToUpper(name), "JAMF_PRO_") {
				if got := Scrub(env); len(got) != 1 {
					t.Errorf("Scrub kept %s: %v", n, got)
				}
			}
		}
	}
	for _, name := range []string{"PATH", "SystemRoot", "TEMP", "HOME", "LC_ALL", "XDG_CONFIG_HOME", "JAMF_PRO_NO_LEARN"} {
		for _, n := range casings(name) {
			if got := ChildEnv([]string{n + "=v"}); len(got) != 1 {
				t.Errorf("ChildEnv dropped allowed %s", n)
			}
		}
	}
}

// TestChildErrorDenyByDefault: a child's error keeps our own message and
// the HTTP status, never a quoted upstream body, and a line carrying a
// credential is withheld whole.
func TestChildErrorDenyByDefault(t *testing.T) {
	got := childError("Error: GET /api/v1/scripts returned HTTP 500: {\"echo\":\"client_secret=abc123\",\"note\":\"BODYMARK\"}\n", nil)
	if strings.Contains(got, "BODYMARK") || strings.Contains(got, "abc123") || !strings.Contains(got, "HTTP 500") {
		t.Fatalf("child error = %q", got)
	}
	if got := childError(`Error: OAuth client_credentials mint: HTTP 401: {"error":"invalid_client"}`, nil); !strings.HasSuffix(got, `HTTP 401: {"error":"invalid_client"}`) {
		t.Fatalf("the short token error code must stay: %q", got)
	}
	if got := childError(`Error: OAuth client_credentials mint: HTTP 401: {"error":"abcdefghijkl"}`, nil); strings.Contains(got, "abcdefghijkl") || !strings.Contains(got, "HTTP 401") {
		t.Fatalf("only a standard OAuth code may follow the status: %q", got)
	}
	// An unreachable server names the failure kind, not "withheld" (Go quotes
	// the token URL, which the credential check would otherwise flag).
	if got := childError(`Error: calling token endpoint: Post "https://acme.jamfcloud.com/api/v1/oauth/token": could not connect to the server`, nil); got != "could not connect to the server" {
		t.Fatalf("network failure kind lost: %q", got)
	}
	if got := childError("Error: retry with token=abc123 please", nil); strings.Contains(got, "abc123") {
		t.Fatalf("a credential in our own text must be withheld: %q", got)
	}
}
