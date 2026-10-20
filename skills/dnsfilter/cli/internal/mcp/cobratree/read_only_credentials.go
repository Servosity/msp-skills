// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials" entry.

package cobratree

import (
	"strings"

	"dnsfilter-pp-cli/internal/mcp/redact"
	"github.com/spf13/cobra"
)

// ReadOnlyCredentialRefusal returns a non-empty reason when method+path must
// not run while the MCP read-only switch is on: every non-GET, and every path
// that reaches a credential endpoint (redact.CredentialPathSegments: the
// api-keys reads, the network secret_key endpoints, the roaming client
// uninstall PIN, and the PSA redirect link, which carries a one-time token
// and is a GET that creates an integration). Callers pass the registered
// template AND, separately, the resolved request path: a caller-supplied
// placeholder value (id="uninstall_pin") only shows up in the latter.
func ReadOnlyCredentialRefusal(method, path string) string {
	if redact.IsCredentialRequestPath(path) {
		return "it returns or manages a credential"
	}
	if !strings.EqualFold(method, "GET") {
		return "it is a " + strings.ToUpper(method) + " request"
	}
	return ""
}

// commandCallsCredentialPath reports whether a Cobra command declares (via the
// generated pp:method/pp:path annotations) an API call ReadOnlyCredentialRefusal
// refuses. Endpoint commands are never mirrored today; this keeps a future
// mirror of one (or a hand-written command carrying the annotations) off the
// read-only surface too.
func commandCallsCredentialPath(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	path := cmd.Annotations["pp:path"]
	if path == "" {
		return false
	}
	method := cmd.Annotations["pp:method"]
	if method == "" {
		method = "GET"
	}
	return ReadOnlyCredentialRefusal(method, path) != ""
}

// credentialResourceArg returns the first argument value of a mirrored call
// that names a credential endpoint (api-keys, uninstall_pin, secret-key,
// redirect_link, in any spelling IsCredentialRequestPath sees through), so
// `tail api-keys` or `why_blocked --host uninstall_pin` cannot make the child
// CLI request one under the read-only switch.
// Every string value is checked (flags, structured positionals, and each
// token of the raw args field), comma lists included.
func credentialResourceArg(args map[string]any) string {
	var check func(v any) string
	check = func(v any) string {
		switch tv := v.(type) {
		case string:
			// Each token is read as a request path the child CLI might build
			// from it (a tail resource, a --host roaming-client id that
			// becomes /v1/user_agents/<id>), with the same normalization as
			// the resolved-path check: case, '-' for '_', percent-encoding,
			// slashes, a format suffix.
			for _, tok := range SplitShellArgs(strings.ReplaceAll(tv, ",", " ")) {
				if redact.IsCredentialRequestPath("/" + tok) {
					return tok
				}
			}
		case []any:
			for _, item := range tv {
				if hit := check(item); hit != "" {
					return hit
				}
			}
		}
		return ""
	}
	for _, v := range args {
		if hit := check(v); hit != "" {
			return hit
		}
	}
	return ""
}
